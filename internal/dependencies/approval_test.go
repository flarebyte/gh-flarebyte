package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestVerifiedLeafRecommendation(t *testing.T) {
	m, l := projectDocuments()
	delete(m, "dependencies")
	packages := l["packages"].(map[string]any)
	delete(packages[""].(map[string]any), "dependencies")
	delete(packages, "node_modules/pkg")
	dir := writeProjectDocuments(t, m, l)
	base := projectClient(t, dir, "metadata")
	authority := authorityClient(t, "normal")
	c := &Client{GitHubToken: authority.GitHubToken, HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.github.com" {
			return authority.HTTP.Do(r)
		}
		if r.URL.Host == "api.scorecard.dev" {
			b, err := json.Marshal(goodCard())
			if err != nil {
				t.Fatal(err)
			}
			return response(string(b), 200), nil
		}
		if r.URL.Host == "api.deps.dev" && strings.Contains(r.URL.Path, "/versions/") {
			version := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			v := Version{Key: VersionKey{System: "NPM", Name: "pkg", Version: version}, PublishedAt: checked.AddDate(-1, 0, 0), Attestations: []Attestation{{Type: "https://slsa.dev/provenance/v1", Verified: true, SourceRepository: "https://github.com/org/repo", Commit: strings.Repeat("a", 40)}}}
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			return response(string(b), 200), nil
		}
		return base.HTTP.Do(r)
	})}
	r := c.AssessLocal(context.Background(), dir, "npm", "pkg", nil, DefaultPolicy(14), checked)
	if r.Decision != Allow || r.Recommendation == nil || r.Recommendation.Version != "3.0.0" || r.Scope != "project-resolution" || r.Evidence.Identity != "verified" || r.Evidence.Maintainers != "multiple" || len(r.Evidence.Missing) > 0 || len(r.Alternatives) != 2 {
		t.Fatalf("%+v evidence=%+v candidates=%+v", r, r.Evidence, r.Details.Candidates)
	}
	if !r.Details.Project.unchanged(dir) {
		t.Fatal("approval mutated project")
	}
}
