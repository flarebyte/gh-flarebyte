// purpose: Bind per-wheel core metadata to PyPI's hashed Simple API record.
// responsibilities: Retrieve bounded metadata sidecars without downloading or executing distributions.
package dependencies

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"time"
)

const maxPyPIMetadata = 256 << 10

var pypiSimpleVersion = regexp.MustCompile(`^1\.[0-9]{1,6}$`)

func (c *Client) checkPyPIMetadata(ctx context.Context, project *ProjectSnapshot, node ProjectNode, f *Finding, sources *[]Source) {
	var index struct {
		Meta struct {
			Version string `json:"api-version"`
		} `json:"meta"`
		Name  string `json:"name"`
		Files []struct {
			Filename string            `json:"filename"`
			URL      string            `json:"url"`
			Hashes   map[string]string `json:"hashes"`
			Core     json.RawMessage   `json:"core-metadata"`
			Legacy   json.RawMessage   `json:"dist-info-metadata"`
		} `json:"files"`
	}
	endpoint := "https://pypi.org/simple/" + url.PathEscape(node.Name) + "/"
	if c.readAccept(ctx, "pypi-simple", endpoint, nil, &index, sources, "application/vnd.pypi.simple.v1+json") != nil {
		return
	}
	if !pypiSimpleVersion.MatchString(index.Meta.Version) || !equivalentName("pypi", index.Name, node.Name) || len(index.Files) > 10000 {
		return
	}
	expected := project.pypi[node.Path]
	matches, selected := 0, 0
	for i, file := range index.Files {
		if file.URL == node.Resolved {
			matches++
			selected = i
		}
	}
	if matches != 1 {
		return
	}
	file := index.Files[selected]
	if file.Filename != expected.Filename || !cargoChecksum.MatchString(file.Hashes["sha256"]) {
		return
	}
	if "sha256-"+file.Hashes["sha256"] != node.Integrity {
		f.Reasons = unique(append(f.Reasons, "ARTIFACT_MISMATCH"))
		return
	}
	core := file.Core
	if len(core) == 0 {
		core = file.Legacy
	}
	var hashes map[string]string
	if json.Unmarshal(core, &hashes) != nil || !cargoChecksum.MatchString(hashes["sha256"]) {
		return
	}
	if len(file.Core) > 0 && len(file.Legacy) > 0 {
		var legacy map[string]string
		if json.Unmarshal(file.Legacy, &legacy) != nil || legacy["sha256"] != hashes["sha256"] {
			return
		}
	}
	b, err := c.readPyPIMetadata(ctx, node.Resolved, sources)
	if err != nil {
		return
	}
	hash := sha256.Sum256(b)
	if hex.EncodeToString(hash[:]) != hashes["sha256"] {
		f.Reasons = unique(append(f.Reasons, "METADATA_HASH_MISMATCH"))
		return
	}
	metadata, ok := parsePyPIMetadata(b)
	if !ok {
		return
	}
	f.Distribution = &DistributionMetadata{Basis: "pypi-core-metadata-sha256", SHA256: hashes["sha256"], DependencyCount: len(metadata.Dependencies), Status: "incomplete"}
	if !equivalentName("pypi", metadata.Name, node.Name) || metadata.Version != node.Version {
		f.Reasons = unique(append(f.Reasons, "IDENTITY_CONFLICT"))
		f.Distribution.Status = "conflict"
		return
	}
	runtime := Finding{}
	checkPythonCompatibility(expected.ManifestPython, expected.LockPython, metadata.Python, &runtime)
	f.Reasons = unique(append(f.Reasons, runtime.Reasons...))
	f.Missing = unique(append(f.Missing, runtime.Missing...))
	if f.Runtime == nil || f.Runtime.Status == "unknown" || runtime.Runtime.Status == "incompatible" {
		f.Runtime = runtime.Runtime
	}
	complete := true
	for _, requirement := range metadata.Dependencies {
		name, constraint, supported := pythonDistributionRequirement(requirement)
		if !supported {
			complete = false
			continue
		}
		var locked string
		for _, item := range project.Nodes {
			if item.Name == name {
				locked = item.Version
				break
			}
		}
		if locked == "" {
			f.Reasons = unique(append(f.Reasons, "PYPI_LOCK_DEPENDENCY_MISSING"))
			continue
		}
		matches, supported := pythonDistributionMatches(constraint, locked)
		if !supported {
			complete = false
			continue
		}
		if !matches {
			f.Reasons = unique(append(f.Reasons, "DEPENDENCY_METADATA_MISMATCH"))
		}
	}
	if complete {
		f.Missing = slices.DeleteFunc(f.Missing, func(s string) bool { return s == "PYPI_DISTRIBUTION_METADATA" })
		f.Distribution.Status = "verified"
	}
	if slices.Contains(f.Reasons, "PYPI_LOCK_DEPENDENCY_MISSING") || slices.Contains(f.Reasons, "DEPENDENCY_METADATA_MISMATCH") {
		f.Distribution.Status = "conflict"
	}
}

func (c *Client) readPyPIMetadata(ctx context.Context, wheelURL string, sources *[]Source) ([]byte, error) {
	if _, ok := pypiWheelURL(wheelURL); !ok {
		return nil, fmt.Errorf("invalid metadata origin")
	}
	endpoint := wheelURL + ".metadata"
	source := Source{Provider: "pypi-core-metadata", URL: endpoint, Status: "unavailable"}
	defer func() { source.RetrievedAt = time.Now().UTC(); *sources = append(*sources, source) }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("User-Agent", "gh-flarebyte-dep/1")
	resp, attempts, err := c.request(req)
	source.Attempts = attempts
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	source.Status = fmt.Sprintf("http_%d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return nil, &LookupError{resp.StatusCode, source.Provider}
	}
	limit := min(int64(maxPyPIMetadata), remainingBytes(ctx))
	if limit <= 0 {
		source.Status = "budget_exhausted"
		return nil, errBudget
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if budgetErr := consumeBytes(ctx, int64(len(b))); budgetErr != nil {
		source.Status = "budget_exhausted"
		return nil, budgetErr
	}
	if err != nil || int64(len(b)) > limit {
		source.Status = "invalid_response"
		return nil, fmt.Errorf("metadata response exceeds limit or cannot be read")
	}
	source.Status = "ok"
	return b, nil
}
