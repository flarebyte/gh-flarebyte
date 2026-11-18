package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func swiftAdviceClient(t *testing.T, mode string) *Client {
	t.Helper()
	return &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("unexpected credentials")
		}
		var body any
		switch {
		case req.URL.Host == "api.osv.dev":
			var q struct {
				Package struct{ Name, Ecosystem string }
				Version string
			}
			if json.NewDecoder(req.Body).Decode(&q) != nil || q.Package.Name != "github.com/org/repo" || q.Package.Ecosystem != "SwiftURL" {
				t.Fatal(q)
			}
			if mode == "osv failure" {
				return response(`{}`, 403), nil
			}
			body = map[string]any{}
			if mode == "vulnerable" && q.Version == "1.0.0" {
				body = map[string]any{"vulns": []any{map[string]string{"id": "GHSA-swift-fixture"}}}
			}
		case req.URL.Host == "api.scorecard.dev":
			return response(`{}`, 404), nil
		case req.URL.Host == "api.github.com" && req.URL.Path == "/repos/org/repo/releases":
			if req.URL.Query().Get("per_page") != "100" {
				t.Fatal(req.URL)
			}
			if mode == "rate limit" {
				return response(`{}`, 403), nil
			}
			flag := false
			releases := []swiftRelease{}
			for _, tag := range []string{"v2.0.0", "1.0.0", "3.0.0-beta.1"} {
				release := swiftRelease{Tag: tag, Draft: &flag, Prerelease: &flag, PublishedAt: checked.AddDate(-1, 0, 0)}
				if mode == "young" && tag == "v2.0.0" {
					release.PublishedAt = checked
				}
				if mode == "missing date" {
					release.PublishedAt = time.Time{}
				}
				if mode == "schema" {
					release.Draft = nil
				}
				releases = append(releases, release)
			}
			switch mode {
			case "tags only":
				releases = nil
			case "duplicate":
				releases = append(releases, swiftRelease{Tag: "2.0.0", Draft: &flag, Prerelease: &flag})
			case "unknown tag":
				releases = append(releases, swiftRelease{Tag: "release-next", Draft: &flag, Prerelease: &flag})
			case "pagination", "bounded":
				if mode == "bounded" || req.URL.Query().Get("page") == "1" {
					releases = nil
					for i := 0; i < 100; i++ {
						releases = append(releases, swiftRelease{Tag: "draft", Draft: func() *bool { v := true; return &v }(), Prerelease: &flag})
					}
				}
			}
			if mode == "duplicate json" {
				return response(`[{"tag_name":"1.0.0","tag_name":"2.0.0","draft":false,"prerelease":false}]`, 200), nil
			}
			body = releases
			if releases == nil {
				body = []swiftRelease{}
			}
		case req.URL.Host == "api.github.com" && req.URL.Path == "/repos/org/repo":
			stars := 1000
			if mode == "low stars" {
				stars = 20
			}
			body = map[string]any{"full_name": "org/repo", "stargazers_count": stars, "description": "A useful Swift library"}
			if mode == "repo mismatch" {
				body = map[string]any{"full_name": "other/repo", "stargazers_count": 1000}
			}
		default:
			t.Fatal("unexpected deps.dev, tag, clone or SDK request", req.URL)
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestSwiftPackageAdvice(t *testing.T) {
	for _, tt := range []struct{ mode, decision, version string }{
		{"normal", Allow, "2.0.0"}, {"young", Allow, "1.0.0"}, {"pagination", Allow, "2.0.0"},
		{"low stars", Deny, ""}, {"repo mismatch", Unknown, ""}, {"osv failure", Unknown, ""},
		{"missing date", Unknown, ""}, {"schema", Unknown, ""}, {"tags only", Unknown, ""},
		{"duplicate", Unknown, ""}, {"duplicate json", Unknown, ""}, {"unknown tag", Unknown, ""}, {"bounded", Unknown, ""}, {"rate limit", Unknown, ""},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			r := swiftAdviceClient(t, tt.mode).AssessPackage(context.Background(), "swift", "https://github.com/org/repo", nil, PackageAdvicePolicy(14), checked)
			if r.Decision != tt.decision || r.Coverage != "direct-package-only" || r.Details.Project != nil {
				t.Fatal(r)
			}
			if tt.version != "" && (r.Recommendation == nil || r.Recommendation.Version != tt.version || r.Description == nil || r.Evidence.IdentityBasis != "github-repository-url" || !slices.Contains(r.Warnings, "MAINTAINER_COUNT_UNKNOWN")) {
				t.Fatal(r)
			}
			if tt.version == "" && r.Recommendation != nil {
				t.Fatal(r)
			}
		})
	}
	v := "1.0.0"
	r := swiftAdviceClient(t, "vulnerable").AssessPackage(context.Background(), "swift", "https://github.com/org/repo", &v, PackageAdvicePolicy(14), checked)
	if r.Decision != Deny || r.Recommendation == nil || r.Recommendation.Version != "2.0.0" || len(r.Evidence.Advisories) == 0 {
		t.Fatal(r)
	}
}

func TestSwiftCoordinates(t *testing.T) {
	name, v, err := Parse("swift", "https://github.com/Org/Repo.git@v1.2.3")
	if err != nil || name != "https://github.com/org/repo" || v == nil || *v != "1.2.3" {
		t.Fatal(name, v, err)
	}
	for _, arg := range []string{"https://evil.test/org/repo", "https://github.com/org/.git", "https://github.com/org/repo?x=1", "https://github.com/org/repo/../../evil", "https://github.com/org/repo@1.0.0\n", "https://github.com/org/repo@$(evil)"} {
		if _, _, err := Parse("swift", arg); err == nil {
			t.Fatal(arg)
		}
	}
	if _, ok := stableVersion("swift", "1.2"); ok {
		t.Fatal("partial stable version")
	}
	if _, ok := stableVersion("swift", "v1.2.3"); ok {
		t.Fatal("catalog must strip only one v prefix")
	}
	if !strings.HasPrefix(name, "https://github.com/") {
		t.Fatal(name)
	}
}
