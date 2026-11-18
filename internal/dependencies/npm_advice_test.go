package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func npmAdviceClient(t *testing.T, mode string) *Client {
	t.Helper()
	owners := 0
	return &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("credential leaked")
		}
		var body any
		switch req.URL.Host {
		case "api.deps.dev":
			if strings.Contains(req.URL.Path, "/projects/") {
				stars := 1000
				if mode == "low reputation" {
					stars = 20
				}
				body = map[string]any{"projectKey": map[string]string{"id": "github.com/org/repo"}, "starsCount": stars}
			} else if strings.Contains(req.URL.Path, "/versions/") {
				if mode == "metadata vulnerability" {
					return response(`{}`, 404), nil
				}
				version := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
				v := Version{Key: VersionKey{System: "NPM", Name: "pkg", Version: version}, PublishedAt: checked.AddDate(-1, 0, 0), Attestations: []Attestation{{Type: "https://slsa.dev/provenance/v1", Verified: true, SourceRepository: "https://github.com/org/repo", Commit: strings.Repeat("a", 40)}}}
				if mode == "unverified" {
					v.Attestations[0].Verified = false
				}
				body = v
			} else {
				versions := []Version{}
				for _, version := range []string{"3.0.0", "2.0.0", "1.0.0"} {
					v := Version{Key: VersionKey{Version: version}, PublishedAt: checked.AddDate(-1, 0, 0)}
					if mode == "young catalog" && version == "3.0.0" {
						v.PublishedAt = checked
					}
					versions = append(versions, v)
				}
				body = map[string]any{"packageKey": map[string]string{"name": "pkg"}, "versions": versions}
			}
		case "api.osv.dev":
			var q struct {
				Version string `json:"version"`
			}
			if json.NewDecoder(req.Body).Decode(&q) != nil {
				t.Fatal("OSV query")
			}
			if mode == "osv failure" {
				return response(`{}`, 403), nil
			}
			if mode == "metadata vulnerability" || mode == "vulnerability" && q.Version == "2.0.0" {
				return response(`{"vulns":[{"id":"GHSA-fixture"}]}`, 200), nil
			}
			body = map[string]any{}
		case "registry.npmjs.org":
			if req.URL.Path == "/pkg" {
				owners++
				if owners > 1 {
					t.Fatal("owner roster repeated")
				}
				roster := []map[string]string{{"name": "one"}, {"name": "two"}}
				if mode == "single owner" {
					roster = roster[:1]
				}
				if mode == "unknown owners" {
					return response(`{}`, 403), nil
				}
				body = map[string]any{"name": "pkg", "maintainers": roster}
			} else {
				version := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
				body = map[string]any{"name": "pkg", "version": version, "description": "Useful package", "dependencies": map[string]string{"child": "^1.0.0"}, "engines": map[string]string{"node": ">=24"}}
			}
		case "api.scorecard.dev":
			return response(`{}`, 404), nil
		default:
			t.Fatal("unexpected deep/privileged/artifact request", req.URL)
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestNPMAdviceWithoutProject(t *testing.T) {
	for _, tt := range []struct{ mode, decision, version string }{
		{"normal", Allow, "3.0.0"}, {"unknown owners", Allow, "3.0.0"}, {"young catalog", Allow, "2.0.0"},
		{"unverified", Unknown, ""}, {"single owner", Deny, ""}, {"low reputation", Deny, ""}, {"osv failure", Unknown, ""}, {"metadata vulnerability", Deny, ""},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			r := npmAdviceClient(t, tt.mode).AssessPackage(context.Background(), "npm", "pkg", nil, PackageAdvicePolicy(14), checked)
			if r.Decision != tt.decision || r.Scope != "package" || r.Coverage != "direct-package-only" || r.Details.Project != nil {
				t.Fatal(r)
			}
			if tt.version != "" && (r.Recommendation == nil || r.Recommendation.Version != tt.version || r.Recommendation.Compatibility != "package-only") {
				t.Fatal(r)
			}
			if tt.mode == "unverified" && (r.CandidateVersion == nil || r.Recommendation != nil) {
				t.Fatal(r)
			}
			if tt.mode == "metadata vulnerability" && !slices.Contains(r.Details.Candidates[0].Reasons, "KNOWN_VULNERABILITY") {
				t.Fatal(r)
			}
			if slices.Contains(r.Evidence.Missing, "PROJECT_RESOLUTION") || slices.Contains(r.Evidence.Missing, "ACTIVE_MAINTAINERS") {
				t.Fatal("strict proof leaked", r)
			}
			if r.Decision == Allow {
				if _, err := PrepareInstall(t.TempDir(), r); err == nil {
					t.Fatal("advice authorized legacy artifact planner")
				}
			}
		})
	}
	v := "2.0.0"
	r := npmAdviceClient(t, "vulnerability").AssessPackage(context.Background(), "npm", "pkg", &v, PackageAdvicePolicy(14), checked)
	if r.Decision != Deny || r.Recommendation == nil || r.Recommendation.Version != "3.0.0" {
		t.Fatal(r)
	}
}
