package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
)

const legacyRequestsVersion = "2.23.0-py2.7"

func pythonAvailabilityClient(t *testing.T, mode string, versions []string) (*Client, *int) {
	t.Helper()
	probes := 0
	c := &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("public catalog sent credentials")
		}
		var payload any
		switch req.URL.Host {
		case "api.deps.dev":
			if !strings.Contains(req.URL.Path, "/versions/") {
				p := Package{}
				p.PackageKey.Name = "requests"
				for _, v := range versions {
					p.Versions = append(p.Versions, Version{Key: VersionKey{Version: v}})
				}
				payload = p
			} else {
				parts := strings.Split(req.URL.Path, "/")
				version := parts[len(parts)-1]
				payload = Version{Key: VersionKey{System: "PYPI", Name: "requests", Version: version}, PublishedAt: checked.AddDate(-1, 0, 0)}
			}
		case "api.osv.dev":
			payload = map[string]any{}
		case "pypi.org":
			parts := strings.Split(req.URL.Path, "/")
			version := parts[len(parts)-2]
			if _, ok := stableVersion("pypi", version); !ok && version != "requests" {
				probes++
				switch mode {
				case "unavailable":
					return response(`{"message":"Not Found"}`, 404), nil
				case "forbidden":
					return response(`{}`, 403), nil
				case "unavailable service":
					return response(`{}`, 503), nil
				case "malformed":
					return response(`{"info":`, 200), nil
				}
			}
			name := "requests"
			if mode == "wrong identity" && version == legacyRequestsVersion {
				name = "other"
			}
			file := map[string]any{"filename": "requests.whl", "yanked": mode == "yanked"}
			if mode == "missing filename" {
				delete(file, "filename")
			}
			files := []any{file}
			if mode == "no files" {
				files = []any{}
			}
			payload = map[string]any{"info": map[string]string{"name": name, "version": version, "summary": "HTTP library"}, "urls": files}
		default:
			t.Fatalf("unexpected provider %s", req.URL)
		}
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
	return c, &probes
}

func TestPythonLegacyCatalogAvailability(t *testing.T) {
	data, err := os.ReadFile("testdata/pypi-requests-legacy-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Versions []string `json:"versions"`
	}
	if err := json.Unmarshal(data, &captured); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"unavailable", "yanked", "present", "forbidden", "unavailable service", "malformed", "wrong identity", "no files", "missing filename"} {
		t.Run(mode, func(t *testing.T) {
			c, calls := pythonAvailabilityClient(t, mode, captured.Versions)
			r := c.Assess(context.Background(), "pypi", "requests", nil, DefaultPolicy(14), checked)
			excluded := mode == "unavailable" || mode == "yanked"
			if slices.Contains(r.Details.RecommendationMissing, "VERSION_ORDERING") == excluded {
				t.Fatal(mode, r.Details.RecommendationMissing)
			}
			if (len(r.Details.CatalogExclusions) == 1) != excluded {
				t.Fatal(r.Details.CatalogExclusions)
			}
			if excluded && (r.Details.CatalogExclusions[0].Version != legacyRequestsVersion || r.Details.CatalogExclusions[0].Reason != "WITHDRAWN_RELEASE") {
				t.Fatal(r.Details.CatalogExclusions)
			}
			maxCalls := 1
			if mode == "unavailable service" {
				maxCalls = 2
			}
			if *calls != maxCalls {
				t.Fatal("unexpected preflight work", *calls)
			}
			if r.Decision == Allow || r.Recommendation != nil {
				t.Fatal("catalog availability bypassed trust/resolution gaps")
			}
			for _, f := range r.Details.Candidates {
				if f.Version == legacyRequestsVersion {
					t.Fatal("unorderable catalog entry became a candidate")
				}
			}
		})
	}
}

func TestPythonLegacyExactCheckRemainsDenied(t *testing.T) {
	c, _ := pythonAvailabilityClient(t, "unavailable", []string{"2.34.2", legacyRequestsVersion})
	version := legacyRequestsVersion
	r := c.Assess(context.Background(), "pypi", "requests", &version, DefaultPolicy(14), checked)
	if r.Decision != Deny || !slices.Contains(r.Reasons, "WITHDRAWN_RELEASE") || r.Requested == nil || *r.Requested != version {
		t.Fatal(r)
	}
	if r.Details.Candidates[0].Version != version || !slices.Contains(r.Details.Candidates[0].Missing, "STABLE_VERSION_SEMANTICS") {
		t.Fatal("exact identity was normalized or hidden", r.Details.Candidates)
	}
}

func TestPythonCatalogProbeBudgetAndIdentity(t *testing.T) {
	versions := []string{"2.34.2", legacyRequestsVersion, legacyRequestsVersion, "99-unknown1", "99-unknown2", "99-unknown3", "99-unknown4"}
	c, calls := pythonAvailabilityClient(t, "unavailable", versions)
	r := c.Assess(context.Background(), "pypi", "requests", nil, DefaultPolicy(14), checked)
	if *calls != maxPythonCatalogProbes || len(r.Details.CatalogExclusions) != maxPythonCatalogProbes || !slices.Contains(r.Details.RecommendationMissing, "VERSION_ORDERING") {
		t.Fatal("probe bound or unknown ordering lost", *calls, r.Details)
	}
	for _, version := range []string{"../bad", "1@other", "1\nunsafe"} {
		sources := []Source{}
		before := *calls
		if c.pythonCatalogUnavailable(context.Background(), "requests", version, &sources) || *calls != before || len(sources) != 0 {
			t.Fatal("unsafe coordinate queried")
		}
	}
	sources := []Source{}
	if !c.pythonCatalogUnavailable(context.Background(), "requests", legacyRequestsVersion, &sources) || len(sources) != 1 || sources[0].Package != "pypi:requests" || sources[0].Version != legacyRequestsVersion {
		t.Fatal("preflight evidence not attributed", sources)
	}
}
