package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func mavenAdviceClient(t *testing.T, mode string) *Client {
	t.Helper()
	return &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("unexpected credentials")
		}
		var body any
		switch {
		case req.URL.Host == "api.osv.dev":
			queryVersion := adviceQueryVersion(t, req, "Maven", "org.example:lib")
			return adviceOSVFixture(mode, queryVersion, "2.0.0", "GHSA-maven-fixture"), nil
		case req.URL.Host == "api.scorecard.dev":
			return response(`{}`, 404), nil
		case req.URL.Host == "api.deps.dev" && strings.Contains(req.URL.Path, "/projects/"):
			stars := 1000
			if mode == "low stars" {
				stars = 20
			}
			body = map[string]any{"projectKey": map[string]string{"id": "github.com/org/repo"}, "starsCount": stars, "description": "Useful JVM library"}
		case req.URL.Host == "api.deps.dev" && strings.Contains(req.URL.Path, "/versions/"):
			if mode == "metadata vulnerability" {
				return response(`{}`, 404), nil
			}
			version := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
			v := Version{Key: VersionKey{System: "MAVEN", Name: "org.example:lib", Version: version}, PublishedAt: checked.AddDate(-1, 0, 0), Deprecated: mode == "deprecated", Attestations: []Attestation{{Type: "https://slsa.dev/provenance/v1", Verified: true, SourceRepository: "https://github.com/org/repo", Commit: strings.Repeat("a", 40)}}}
			if mode == "unverified" {
				v.Attestations = nil
			}
			body = v
		case req.URL.Host == "api.deps.dev":
			versions := []Version{}
			names := []string{"3.0.0", "2.0.0", "4.0.0-RC1", "4.0.0-SNAPSHOT"}
			if mode == "qualifier" {
				names = append(names, "3.0.Final")
			}
			if mode == "only qualifiers" {
				names = []string{"3.0.Final"}
			}
			for _, name := range names {
				v := Version{Key: VersionKey{Version: name}, PublishedAt: checked.AddDate(-1, 0, 0)}
				if mode == "young" && name == "3.0.0" {
					v.PublishedAt = checked
				}
				versions = append(versions, v)
			}
			body = map[string]any{"packageKey": map[string]string{"name": "org.example:lib"}, "versions": versions}
		default:
			t.Fatal("unexpected resolver/registry/build request", req.URL)
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestMavenPackageAdvice(t *testing.T) {
	for _, tt := range []struct{ mode, decision, version string }{
		{"normal", Allow, "3.0.0"}, {"young", Allow, "2.0.0"},
		{"qualifier", Unknown, ""}, {"only qualifiers", Unknown, ""}, {"unverified", Unknown, ""},
		{"low stars", Deny, ""}, {"deprecated", Deny, ""}, {"osv failure", Unknown, ""}, {"metadata vulnerability", Deny, ""},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			r := mavenAdviceClient(t, tt.mode).AssessPackage(context.Background(), "maven", "org.example:lib", nil, PackageAdvicePolicy(14), checked)
			if r.Decision != tt.decision || r.Scope != "package" || r.Coverage != "direct-package-only" || r.Details.Project != nil {
				t.Fatal(r)
			}
			if tt.version != "" && (r.Recommendation == nil || r.Recommendation.Version != tt.version || r.Description == nil || !slices.Contains(r.Warnings, "MAVEN_LIFECYCLE_COVERAGE_LIMITED")) {
				t.Fatal(r)
			}
			if tt.mode == "only qualifiers" && !slices.Contains(r.Evidence.Missing, "VERSION_ORDERING") {
				t.Fatal(r)
			}
			if tt.mode == "qualifier" && r.Recommendation != nil {
				t.Fatal("unsupported ordering permitted recommendation")
			}
			if slices.Contains(r.Evidence.Missing, "PROJECT_RESOLUTION") || slices.Contains(r.Evidence.Missing, "ACTIVE_MAINTAINERS") {
				t.Fatal(r)
			}
		})
	}
	v := "2.0.0"
	r := mavenAdviceClient(t, "vulnerability").AssessPackage(context.Background(), "maven", "org.example:lib", &v, PackageAdvicePolicy(14), checked)
	if r.Decision != Deny || r.Recommendation == nil || r.Recommendation.Version != "3.0.0" {
		t.Fatal(r)
	}
}

func TestMavenNumericVersionSubset(t *testing.T) {
	for _, v := range []string{"1", "1.2", "1.2.3", "1.2.3.4"} {
		if _, ok := stableVersion("maven", v); !ok {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"01.2", "1.2.3.4.5", "1.0.Final", "1.0-jre", "1..2", "1.0-SNAPSHOT", "99999999999999999999999999.1"} {
		if _, ok := stableVersion("maven", v); ok {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"1.0-RC1", "1.0.alpha1", "1.0-beta.2", "1.0-M1", "1.0-SNAPSHOT"} {
		if !knownPrerelease("maven", v) {
			t.Fatal(v)
		}
	}
	if !newer("maven", "1.10", "1.9") || knownPrerelease("maven", "1.0.Final") {
		t.Fatal("incorrect Maven classification")
	}
}

func adviceQueryVersion(t *testing.T, req *http.Request, ecosystem, name string) string {
	t.Helper()
	var q struct {
		Package struct{ Name, Ecosystem string }
		Version string
	}
	if err := json.NewDecoder(req.Body).Decode(&q); err != nil || q.Package.Name != name || q.Package.Ecosystem != ecosystem {
		t.Fatalf("unexpected OSV query: %+v (%v)", q, err)
	}
	return q.Version
}

func adviceRepositoryResponse(t *testing.T, req *http.Request) (*http.Response, bool) {
	t.Helper()
	if req.Header.Get("Authorization") != "" {
		t.Fatal("unexpected credentials")
	}
	if req.URL.Host == "api.scorecard.dev" {
		return response(`{}`, 404), true
	}
	if req.URL.Host == "api.deps.dev" && strings.Contains(req.URL.Path, "/projects/") {
		return response(`{"projectKey":{"id":"github.com/org/repo"},"starsCount":1000}`, 200), true
	}
	return nil, false
}

func adviceOSVFixture(mode, version, vulnerableVersion, id string) *http.Response {
	if mode == "osv failure" {
		return response(`{}`, 403)
	}
	if mode == "metadata vulnerability" || mode == "vulnerability" && version == vulnerableVersion {
		b, _ := json.Marshal(map[string]any{"vulns": []any{map[string]string{"id": id}}})
		return response(string(b), 200)
	}
	return response(`{}`, 200)
}
