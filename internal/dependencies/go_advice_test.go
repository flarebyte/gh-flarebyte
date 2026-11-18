package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func goAdviceClient(t *testing.T, module, mode string) *Client {
	t.Helper()
	return &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("unexpected credentials")
		}
		var body any
		switch {
		case req.URL.Host == "api.osv.dev":
			var query struct {
				Package struct{ Name, Ecosystem string }
				Version string
			}
			if json.NewDecoder(req.Body).Decode(&query) != nil || query.Package.Ecosystem != "Go" || query.Package.Name != module {
				t.Fatal(query)
			}
			if mode == "osv failure" {
				return response(`{}`, 403), nil
			}
			if mode == "metadata vulnerability" || mode == "vulnerability" && query.Version == "v2.2.0" {
				return response(`{"vulns":[{"id":"GHSA-go-fixture"}]}`, 200), nil
			}
			body = map[string]any{}
		case req.URL.Host == "api.scorecard.dev":
			return response(`{}`, 404), nil
		case req.URL.Host == "api.deps.dev" && strings.Contains(req.URL.Path, "/projects/"):
			stars := 1000
			if mode == "low stars" {
				stars = 20
			}
			body = map[string]any{"projectKey": map[string]string{"id": "github.com/org/repo"}, "starsCount": stars, "description": "Useful Go module"}
		case req.URL.Host == "api.deps.dev" && strings.Contains(req.URL.Path, "/versions/"):
			if mode == "metadata vulnerability" {
				return response(`{}`, 404), nil
			}
			version := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
			v := Version{Key: VersionKey{System: "GO", Name: module, Version: version}, PublishedAt: checked.AddDate(-1, 0, 0), Deprecated: mode == "deprecated"}
			b, _ := json.Marshal(v)
			var data map[string]any
			if json.Unmarshal(b, &data) != nil {
				t.Fatal("fixture")
			}
			repo := "github.com/org/repo"
			if mode == "repo conflict" {
				repo = "github.com/other/repo"
			}
			data["relatedProjects"] = []any{map[string]any{"projectKey": map[string]string{"id": repo}, "relationType": "SOURCE_REPO"}}
			body = data
		case req.URL.Host == "api.deps.dev":
			versions := []Version{}
			for _, version := range []string{"v2.3.0", "v2.2.0", "v0.0.0-20240101000000-aaaaaaaaaaaa"} {
				v := Version{Key: VersionKey{Version: version}, PublishedAt: checked.AddDate(-1, 0, 0)}
				if mode == "young" && version == "v2.3.0" {
					v.PublishedAt = checked
				}
				versions = append(versions, v)
			}
			body = map[string]any{"packageKey": map[string]string{"name": module}, "versions": versions}
		default:
			t.Fatal("unexpected registry/vanity/resolution/native request", req.URL)
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestGoPackageAdvice(t *testing.T) {
	for _, tt := range []struct{ mode, module, decision, version string }{
		{"normal", "github.com/org/repo/v2", Allow, "v2.3.0"},
		{"young", "github.com/org/repo/v2", Allow, "v2.2.0"},
		{"vanity", "example.com/module/v2", Unknown, ""},
		{"repo conflict", "github.com/org/repo/v2", Deny, ""},
		{"low stars", "github.com/org/repo/v2", Deny, ""},
		{"deprecated", "github.com/org/repo/v2", Deny, ""},
		{"osv failure", "github.com/org/repo/v2", Unknown, ""},
		{"metadata vulnerability", "github.com/org/repo/v2", Deny, ""},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			r := goAdviceClient(t, tt.module, tt.mode).AssessPackage(context.Background(), "go", tt.module, nil, PackageAdvicePolicy(14), checked)
			if r.Decision != tt.decision || r.Scope != "package" || r.Coverage != "direct-package-only" || r.Details.Project != nil {
				t.Fatal(r)
			}
			if tt.version != "" && (r.Recommendation == nil || r.Recommendation.Version != tt.version || r.Evidence.IdentityBasis != "github-module-path" || r.Description == nil) {
				t.Fatal(r)
			}
			if slices.Contains(r.Evidence.Missing, "PROJECT_RESOLUTION") || slices.Contains(r.Evidence.Missing, "ACTIVE_MAINTAINERS") {
				t.Fatal(r)
			}
			if !slices.Contains(r.Warnings, "GO_RETRACTIONS_UNCHECKED") {
				t.Fatal(r.Warnings)
			}
			if tt.mode == "vanity" && (r.Recommendation != nil || r.CandidateVersion == nil) {
				t.Fatal(r)
			}
		})
	}
	module := "github.com/org/repo/v2"
	v := "v2.2.0"
	r := goAdviceClient(t, module, "vulnerability").AssessPackage(context.Background(), "go", module, &v, PackageAdvicePolicy(14), checked)
	if r.Decision != Deny || r.Recommendation == nil || r.Recommendation.Version != "v2.3.0" {
		t.Fatal(r)
	}
	v = "v0.0.0-20240101000000-aaaaaaaaaaaa"
	r = goAdviceClient(t, module, "normal").AssessPackage(context.Background(), "go", module, &v, PackageAdvicePolicy(14), checked)
	if r.Decision != Unknown || r.Recommendation != nil || slices.Contains(r.Reasons, "PRERELEASE") {
		t.Fatal(r)
	}
}

func TestGoAdviceRepositoryBinding(t *testing.T) {
	for _, tt := range []struct{ module, repo, identity, want, basis string }{
		{"github.com/org/repo/sub/v2", "", "unknown", "verified", "github-module-path"},
		{"github.com/org/repo.git/sub", "github.com/org/repo", "unverified", "verified", "github-module-path"},
		{"github.com/org/repo", "github.com/org/repo", "unverified", "verified", "github-module-path"},
		{"github.com/org/repo", "github.com/other/repo", "verified", "conflict", "github-module-path"},
		{"github.com/org/repo", "", "conflict", "conflict", ""},
		{"golang.org/x/net", "github.com/golang/net", "unverified", "unverified", ""},
		{"golang.org/x/net", "github.com/golang/net", "verified", "verified", "verified-provenance"},
	} {
		_, identity, basis := goAdviceRepository(tt.module, tt.repo, tt.identity)
		if identity != tt.want || basis != tt.basis {
			t.Fatal(tt, identity, basis)
		}
	}
}
