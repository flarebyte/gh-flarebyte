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

func pubAdviceClient(t *testing.T, mode string) *Client {
	t.Helper()
	catalogCalls, publisherCalls := 0, 0
	return &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("unexpected credentials")
		}
		var body any
		switch {
		case req.URL.Host == "pub.dev" && req.URL.Path == "/api/packages/pkg":
			catalogCalls++
			if catalogCalls != 1 {
				t.Fatal("catalog fetched repeatedly")
			}
			if req.Header.Get("Accept") != "application/vnd.pub.v2+json" {
				t.Fatal(req.Header)
			}
			if mode == "missing" {
				return response(`{}`, 404), nil
			}
			if mode == "null flag" {
				return response(`{"name":"pkg","versions":[{"version":"1.0.0","retracted":null}]}`, 200), nil
			}
			if mode == "duplicate json" {
				return response(`{"name":"pkg","name":"other"}`, 200), nil
			}
			releases := []pubRelease{}
			for _, v := range []string{"3.0.0", "2.0.0", "4.0.0-rc.1"} {
				release := pubRelease{Version: v, Published: checked.AddDate(-1, 0, 0)}
				release.Pubspec.Name = "pkg"
				release.Pubspec.Version = v
				release.Pubspec.Description = "A useful Dart library"
				release.Pubspec.Repository = "https://github.com/org/repo"
				switch mode {
				case "young":
					if v == "3.0.0" {
						release.Published = checked
					}
				case "missing date":
					release.Published = time.Time{}
				case "future date":
					release.Published = checked.AddDate(0, 0, 1)
				case "retracted":
					release.Retracted = true
				case "release mismatch":
					release.Pubspec.Name = "other"
				case "no repository":
					release.Pubspec.Repository = ""
				case "evil repository":
					release.Pubspec.Repository = "https://evil.test/repo"
				}
				releases = append(releases, release)
			}
			if mode == "duplicate version" {
				releases = append(releases, releases[0])
			}
			if mode == "unknown version" {
				r := releases[0]
				r.Version = "release-next"
				r.Pubspec.Version = r.Version
				releases = append(releases, r)
			}
			body = map[string]any{"name": "pkg", "versions": releases, "isDiscontinued": mode == "discontinued"}
		case req.URL.Host == "pub.dev" && strings.HasSuffix(req.URL.Path, "/publisher"):
			publisherCalls++
			if publisherCalls != 1 {
				t.Fatal("publisher fetched repeatedly")
			}
			if mode == "publisher failure" {
				return response(`{}`, 403), nil
			}
			body = map[string]any{"publisherId": "example.org"}
			if mode == "no publisher" {
				body = map[string]any{"publisherId": nil}
			}
		case req.URL.Host == "api.osv.dev":
			var q struct {
				Package struct{ Name, Ecosystem string }
				Version string
			}
			if json.NewDecoder(req.Body).Decode(&q) != nil || q.Package.Name != "pkg" || q.Package.Ecosystem != "Pub" {
				t.Fatal(q)
			}
			if mode == "osv failure" {
				return response(`{}`, 403), nil
			}
			body = map[string]any{}
			if mode == "vulnerable" && q.Version == "2.0.0" {
				body = map[string]any{"vulns": []any{map[string]string{"id": "GHSA-pub-fixture"}}}
			}
		case req.URL.Host == "api.github.com" && req.URL.Path == "/repos/org/repo":
			body = map[string]any{"full_name": "org/repo", "stargazers_count": 1000}
		case req.URL.Host == "api.scorecard.dev":
			return response(`{}`, 404), nil
		default:
			t.Fatal("unexpected deps.dev/archive/SDK/source lookup", req.URL)
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestPubPackageAdvice(t *testing.T) {
	for _, tt := range []struct{ mode, decision, candidate string }{
		{"normal", Unknown, "3.0.0"}, {"young", Unknown, "2.0.0"}, {"no repository", Unknown, "3.0.0"},
		{"evil repository", Unknown, "3.0.0"}, {"no publisher", Unknown, "3.0.0"}, {"publisher failure", Unknown, "3.0.0"},
		{"retracted", Deny, ""}, {"discontinued", Deny, ""}, {"osv failure", Unknown, ""},
		{"missing date", Unknown, ""}, {"future date", Unknown, ""}, {"missing", Unknown, ""},
		{"null flag", Unknown, ""}, {"duplicate version", Unknown, ""}, {"release mismatch", Unknown, ""}, {"duplicate json", Unknown, ""},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			r := pubAdviceClient(t, tt.mode).AssessPackage(context.Background(), "pub", "pkg", nil, PackageAdvicePolicy(14), checked)
			if r.Decision != tt.decision || r.Recommendation != nil || r.Scope != "package" || r.Coverage != "direct-package-only" || r.Details.Project != nil {
				t.Fatal(r)
			}
			if tt.candidate != "" && (r.CandidateVersion == nil || *r.CandidateVersion != tt.candidate || r.Description == nil || !slices.Contains(r.Evidence.Missing, "PACKAGE_IDENTITY") || !slices.Contains(r.Warnings, "MAINTAINER_COUNT_UNKNOWN")) {
				t.Fatal(r)
			}
			if tt.candidate == "" && r.CandidateVersion != nil {
				t.Fatal(r)
			}
		})
	}
	v := "2.0.0"
	r := pubAdviceClient(t, "vulnerable").AssessPackage(context.Background(), "pub", "pkg", &v, PackageAdvicePolicy(14), checked)
	if r.Decision != Deny || len(r.Evidence.Advisories) == 0 || r.CandidateVersion == nil || *r.CandidateVersion != "3.0.0" {
		t.Fatal(r)
	}
	r = pubAdviceClient(t, "unknown version").AssessPackage(context.Background(), "pub", "pkg", nil, PackageAdvicePolicy(14), checked)
	if r.Recommendation != nil || !slices.Contains(r.Details.RecommendationMissing, "VERSION_ORDERING") {
		t.Fatal(r)
	}
}

func TestPubCoordinatesAndVersions(t *testing.T) {
	for _, name := range []string{"Pkg", "bad-name", "bad.name", "_hidden", "pkg:1.0.0", "../pkg"} {
		if _, _, err := Parse("pub", name); err == nil {
			t.Fatal(name)
		}
	}
	for _, version := range []string{"1", "1.2", "1.2.3.4", "01.2.3", "v1.2.3"} {
		if _, ok := stableVersion("pub", version); ok {
			t.Fatal(version)
		}
	}
	if _, ok := stableVersion("pub", "1.2.3+build.1"); ok {
		t.Fatal("unsupported Dart build metadata ordering")
	}
	if !knownPrerelease("pub", "1.2.3-rc.1") {
		t.Fatal("prerelease")
	}
}
