// purpose: Read bounded public dependency evidence without trusting package-provided endpoints.
// responsibilities: Query fixed HTTPS providers, enforce response limits and preserve evidence provenance.
// architecture notes: A small HTTP interface permits offline contract tests; no provider credentials are needed.
package dependencies

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBody = 4 << 20

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
type Client struct {
	HTTP HTTPDoer
	// GitHubToken supplies existing credentials lazily. Tokens are never recorded
	// in reports and are sent only to the fixed GitHub API origin.
	GitHubToken func(context.Context) (string, error)
}

func NewClient() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxResponseHeaderBytes = 64 << 10
	transport.ResponseHeaderTimeout = 8 * time.Second
	return &Client{HTTP: &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return fmt.Errorf("provider redirect refused") }}}
}

type LookupError struct {
	Status   int
	Provider string
}

func (e *LookupError) Error() string {
	return fmt.Sprintf("%s lookup failed (HTTP %d)", e.Provider, e.Status)
}

func (c *Client) read(ctx context.Context, provider, endpoint string, body any, out any, sources *[]Source) error {
	return c.readAccept(ctx, provider, endpoint, body, out, sources, "application/json")
}

func (c *Client) readAccept(ctx context.Context, provider, endpoint string, body any, out any, sources *[]Source, accept string) error {
	var input io.Reader
	method := http.MethodGet
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		input = bytes.NewReader(b)
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, input)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "gh-flarebyte-dep/1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	source := Source{Provider: provider, URL: endpoint, RetrievedAt: time.Now().UTC(), Status: "unavailable"}
	defer func() { source.RetrievedAt = time.Now().UTC(); *sources = append(*sources, source) }()
	if provider == "github" {
		if req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != "api.github.com" || c.GitHubToken == nil {
			source.Status = "authority_unavailable"
			return fmt.Errorf("github authority lookup unavailable")
		}
		token, err := c.GitHubToken(ctx)
		if err != nil || token == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
			source.Status = "credentials_unavailable"
			return fmt.Errorf("github credentials unavailable")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	}
	resp, attempts, err := c.request(req)
	source.Attempts = attempts
	if err != nil {
		if errors.Is(err, errBudget) {
			source.Status = "budget_exhausted"
		}
		return fmt.Errorf("%s lookup unavailable", provider)
	}
	defer resp.Body.Close()
	source.Status = fmt.Sprintf("http_%d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return &LookupError{resp.StatusCode, provider}
	}
	limit := remainingBytes(ctx)
	if limit <= 0 {
		source.Status = "budget_exhausted"
		return errBudget
	}
	if limit > maxBody {
		limit = maxBody
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		source.Status = "read_failed"
		return fmt.Errorf("%s response could not be read", provider)
	}
	if err := consumeBytes(ctx, int64(len(b))); err != nil {
		source.Status = "budget_exhausted"
		return err
	}
	if len(b) > maxBody {
		source.Status = "response_too_large"
		return fmt.Errorf("%s response exceeds size limit", provider)
	}
	if trimmed := bytes.TrimSpace(b); len(trimmed) == 0 || (trimmed[0] != '{' && !((provider == "github" || provider == "github-releases") && trimmed[0] == '[')) {
		source.Status = "invalid_response"
		return fmt.Errorf("%s invalid response object", provider)
	}
	var decodeErr error
	if provider == "github-releases" {
		decodeErr = decodeGitHubArray(b, out)
	} else if provider == "github" {
		decodeErr = json.Unmarshal(b, out)
	} else {
		decodeErr = uniqueJSON(b, out)
	}
	if decodeErr != nil {
		source.Status = "invalid_response"
		return fmt.Errorf("%s invalid response", provider)
	}
	source.Status = "ok"
	return nil
}

type VersionKey struct {
	System  string `json:"system"`
	Name    string `json:"name"`
	Version string `json:"version"`
}
type Package struct {
	PackageKey struct {
		Name string `json:"name"`
	} `json:"packageKey"`
	Versions []Version `json:"versions"`
}
type Version struct {
	Key          VersionKey    `json:"versionKey"`
	PublishedAt  time.Time     `json:"publishedAt"`
	Deprecated   bool          `json:"isDeprecated"`
	Attestations []Attestation `json:"attestations"`
	Provenances  []Attestation `json:"slsaProvenances"`
	Advisories   []struct {
		ID string `json:"id"`
	} `json:"advisoryKeys"`
	Projects []struct {
		Key struct {
			ID string `json:"id"`
		} `json:"projectKey"`
		Provenance string `json:"relationProvenance"`
		Type       string `json:"relationType"`
	} `json:"relatedProjects"`
}

type Attestation struct {
	Type             string `json:"type"`
	Verified         bool   `json:"verified"`
	SourceRepository string `json:"sourceRepository"`
	Commit           string `json:"commit"`
}
type Check struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}
type Scorecard struct {
	Date time.Time `json:"date"`
	Repo struct {
		Name   string `json:"name"`
		Commit string `json:"commit"`
	} `json:"repo"`
	Engine struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	} `json:"scorecard"`
	Score  float64 `json:"score"`
	Checks []Check `json:"checks"`
}
type Project struct {
	Key struct {
		ID string `json:"id"`
	} `json:"projectKey"`
	Stars       *int   `json:"starsCount"`
	Description string `json:"description"`
	Scorecard   struct {
		Date       time.Time `json:"date"`
		Repository struct {
			Name   string `json:"name"`
			Commit string `json:"commit"`
		} `json:"repository"`
		Engine struct {
			Version string `json:"version"`
			Commit  string `json:"commit"`
		} `json:"scorecard"`
		Score  float64 `json:"overallScore"`
		Checks []Check `json:"checks"`
	} `json:"scorecard"`
}
type Advisory struct {
	ID        string     `json:"id"`
	Aliases   []string   `json:"aliases"`
	Withdrawn *time.Time `json:"withdrawn"`
	Modified  time.Time  `json:"modified"`
	Summary   string     `json:"summary"`
	Affected  []struct {
		Package struct {
			Name      string `json:"name"`
			Ecosystem string `json:"ecosystem"`
		} `json:"package"`
		Ranges []struct {
			Type   string              `json:"type"`
			Repo   string              `json:"repo,omitempty"`
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
}

func (c *Client) Package(ctx context.Context, system, name string, sources *[]Source) (Package, error) {
	var p Package
	err := c.read(ctx, "deps.dev", "https://api.deps.dev/v3/systems/"+url.PathEscape(system)+"/packages/"+url.PathEscape(name), nil, &p, sources)
	return p, err
}
func (c *Client) Version(ctx context.Context, system, name, version string, sources *[]Source) (Version, error) {
	var v Version
	err := c.read(ctx, "deps.dev", "https://api.deps.dev/v3/systems/"+url.PathEscape(system)+"/packages/"+url.PathEscape(name)+"/versions/"+url.PathEscape(version), nil, &v, sources)
	return v, err
}
func (c *Client) Project(ctx context.Context, repo string, sources *[]Source) (Project, error) {
	var p Project
	err := c.read(ctx, "deps.dev", "https://api.deps.dev/v3/projects/"+url.PathEscape(repo), nil, &p, sources)
	return p, err
}
func (c *Client) Scorecard(ctx context.Context, repo string, sources *[]Source) (Scorecard, error) {
	var s Scorecard
	if Repository(repo) == "" {
		return s, fmt.Errorf("invalid repository identity")
	}
	err := c.read(ctx, "scorecard", "https://api.scorecard.dev/projects/"+repo, nil, &s, sources)
	return s, err
}

// OSV follows every returned page within a bounded budget. A partial result is
// always accompanied by an error, never mistaken for an empty successful scan.
func (c *Client) OSV(ctx context.Context, ecosystem, name, version string, sources *[]Source) ([]Advisory, error) {
	items := []Advisory{}
	token := ""
	seen := map[string]bool{}
	for page := 0; page < 8; page++ {
		var response struct {
			Vulns []Advisory `json:"vulns"`
			Next  string     `json:"next_page_token"`
		}
		query := map[string]any{"package": map[string]string{"ecosystem": ecosystem, "name": name}, "version": version}
		if token != "" {
			query["page_token"] = token
		}
		before := len(*sources)
		err := c.read(ctx, "osv", "https://api.osv.dev/v1/query", query, &response, sources)
		if len(*sources) > before {
			(*sources)[len(*sources)-1].Package = ecosystem + ":" + name
			(*sources)[len(*sources)-1].Version = version
		}
		if err != nil {
			return items, err
		}
		items = append(items, response.Vulns...)
		if response.Next == "" {
			return items, nil
		}
		if seen[response.Next] {
			return items, fmt.Errorf("osv pagination repeated")
		}
		token = response.Next
		seen[token] = true
	}
	return items, fmt.Errorf("osv pagination limit reached")
}
