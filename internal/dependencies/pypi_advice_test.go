package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func pypiAdviceClient(t *testing.T, mode string) *Client {
	t.Helper()
	return &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
		if response, ok := adviceRepositoryResponse(t, req); ok {
			return response, nil
		}
		var body any
		switch {
		case req.URL.Host == "api.osv.dev":
			queryVersion := adviceQueryVersion(t, req, "PyPI", "my-pkg")
			if mode == "osv failure" {
				return response(`{}`, 403), nil
			}
			if mode == "metadata vulnerability" || mode == "vulnerability" && queryVersion == "1!2.0" {
				return response(`{"vulns":[{"id":"GHSA-pypi-fixture"}]}`, 200), nil
			}
			body = map[string]any{}
		case req.URL.Host == "api.deps.dev" && strings.Contains(req.URL.Path, "/versions/"):
			if mode == "metadata vulnerability" {
				return response(`{}`, 404), nil
			}
			version := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
			v := Version{Key: VersionKey{System: "PYPI", Name: "my-pkg", Version: version}, PublishedAt: checked.AddDate(-1, 0, 0), Attestations: []Attestation{{Type: "https://docs.pypi.org/attestations/publish/v1", Verified: true, SourceRepository: "https://github.com/org/repo", Commit: strings.Repeat("a", 40)}}}
			if mode == "unverified" {
				v.Attestations = nil
			}
			body = v
		case req.URL.Host == "api.deps.dev":
			versions := []Version{}
			for _, version := range []string{"1!3.0.post1", "1!2.0", "2!1.0rc1"} {
				v := Version{Key: VersionKey{Version: version}, PublishedAt: checked.AddDate(-1, 0, 0)}
				if mode == "young" && version == "1!3.0.post1" {
					v.PublishedAt = checked
				}
				versions = append(versions, v)
			}
			body = map[string]any{"packageKey": map[string]string{"name": "my-pkg"}, "versions": versions}
		case req.URL.Host == "pypi.org" && req.URL.Path == "/pypi/my-pkg/json":
			roles := []any{map[string]string{"role": "Owner", "user": "one"}, map[string]string{"role": "Maintainer", "user": "two"}}
			var organization any
			if mode == "single owner" || mode == "organization" {
				roles = roles[:1]
			}
			if mode == "organization" {
				organization = "org"
			}
			body = map[string]any{"info": map[string]string{"name": "My_Pkg"}, "ownership": map[string]any{"roles": roles, "organization": organization}}
			if mode == "unknown roles" {
				body = map[string]any{"info": map[string]string{"name": "My_Pkg"}}
			}
		case req.URL.Host == "pypi.org" && strings.HasPrefix(req.URL.Path, "/pypi/my-pkg/"):
			if mode == "unavailable" {
				return response(`{}`, 404), nil
			}
			if mode == "registry failure" {
				return response(`{}`, 403), nil
			}
			parts := strings.Split(req.URL.Path, "/")
			version := parts[len(parts)-2]
			file := map[string]any{"filename": "fixture.whl", "yanked": mode == "yanked" || mode == "partly yanked"}
			if mode == "missing yanked" {
				delete(file, "yanked")
			}
			files := []any{file}
			if mode == "partly yanked" {
				files = append(files, map[string]any{"filename": "fixture.tar.gz", "yanked": false})
			}
			body = map[string]any{"info": map[string]string{"name": "My_Pkg", "version": version, "summary": "Useful library", "requires_python": ">=999"}, "urls": files}
		default:
			t.Fatal("unexpected wheel/metadata/runtime/privileged request", req.URL)
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestPyPIPackageAdvice(t *testing.T) {
	for _, tt := range []struct{ mode, decision, version string }{
		{"normal", Allow, "1!3.0.post1"}, {"young", Allow, "1!2.0"}, {"partly yanked", Allow, "1!3.0.post1"},
		{"unknown roles", Allow, "1!3.0.post1"}, {"organization", Allow, "1!3.0.post1"},
		{"single owner", Deny, ""}, {"unverified", Unknown, ""}, {"yanked", Deny, ""},
		{"unavailable", Deny, ""}, {"registry failure", Unknown, ""}, {"missing yanked", Unknown, ""}, {"osv failure", Unknown, ""}, {"metadata vulnerability", Deny, ""},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			r := pypiAdviceClient(t, tt.mode).AssessPackage(context.Background(), "pypi", "my-pkg", nil, PackageAdvicePolicy(14), checked)
			if r.Decision != tt.decision || r.Scope != "package" || r.Coverage != "direct-package-only" || r.Details.Project != nil {
				t.Fatal(r)
			}
			if tt.version != "" && (r.Recommendation == nil || r.Recommendation.Version != tt.version || r.Description == nil) {
				t.Fatal(r)
			}
			if (tt.mode == "unknown roles" || tt.mode == "organization") && !slices.Contains(r.Warnings, "MAINTAINER_COUNT_UNKNOWN") {
				t.Fatal(r.Warnings)
			}
			if slices.Contains(r.Evidence.Missing, "PROJECT_RESOLUTION") || slices.Contains(r.Evidence.Missing, "ACTIVE_MAINTAINERS") {
				t.Fatal(r)
			}
			if tt.mode == "unverified" && (r.Recommendation != nil || r.CandidateVersion == nil) {
				t.Fatal(r)
			}
		})
	}
	v := "1!2.0"
	r := pypiAdviceClient(t, "vulnerability").AssessPackage(context.Background(), "pypi", "my-pkg", &v, PackageAdvicePolicy(14), checked)
	if r.Decision != Deny || r.Recommendation == nil || r.Recommendation.Version != "1!3.0.post1" {
		t.Fatal(r)
	}
}
