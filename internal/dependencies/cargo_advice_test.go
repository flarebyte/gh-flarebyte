package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func cargoAdviceClient(t *testing.T, mode string) *Client {
	t.Helper()
	owners := 0
	return &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
		if response, ok := adviceRepositoryResponse(t, req); ok {
			return response, nil
		}
		var body any
		switch {
		case req.URL.Host == "api.osv.dev":
			queryVersion := adviceQueryVersion(t, req, "crates.io", "pkg")
			if mode == "osv failure" {
				return response(`{}`, 403), nil
			}
			if mode == "metadata vulnerability" || mode == "vulnerability" && queryVersion == "2.0.0" {
				return response(`{"vulns":[{"id":"GHSA-cargo-fixture"}]}`, 200), nil
			}
			body = map[string]any{}
		case req.URL.Host == "api.deps.dev" && strings.Contains(req.URL.Path, "/versions/"):
			if mode == "metadata vulnerability" {
				return response(`{}`, 404), nil
			}
			version := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
			v := Version{Key: VersionKey{System: "CARGO", Name: "pkg", Version: version}, PublishedAt: checked.AddDate(-1, 0, 0), Attestations: []Attestation{{Type: "https://slsa.dev/provenance/v1", Verified: true, SourceRepository: "https://github.com/org/repo", Commit: strings.Repeat("a", 40)}}}
			if mode == "unverified" {
				v.Attestations = nil
			}
			body = v
		case req.URL.Host == "api.deps.dev":
			versions := []Version{}
			for _, version := range []string{"3.0.0", "2.0.0", "4.0.0-rc1"} {
				v := Version{Key: VersionKey{Version: version}, PublishedAt: checked.AddDate(-1, 0, 0)}
				if mode == "young" && version == "3.0.0" {
					v.PublishedAt = checked
				}
				versions = append(versions, v)
			}
			body = map[string]any{"packageKey": map[string]string{"name": "pkg"}, "versions": versions}
		case req.URL.Host == "crates.io" && req.URL.Path == "/api/v1/crates/pkg":
			body = map[string]any{"crate": map[string]string{"description": "Useful Rust library"}}
		case req.URL.Host == "crates.io" && req.URL.Path == "/api/v1/crates/pkg/owners":
			owners++
			if owners > 1 {
				t.Fatal("owner roster fetched repeatedly")
			}
			if mode == "unknown owners" {
				return response(`{}`, 403), nil
			}
			users := []cargoOwner{{ID: 1, Login: "one", Kind: "user"}, {ID: 2, Login: "two", Kind: "user"}}
			switch mode {
			case "single owner":
				users = users[:1]
			case "team":
				users[1] = cargoOwner{ID: 3, Login: "github:org:team", Kind: "team"}
			case "team only":
				users = []cargoOwner{{ID: 3, Login: "github:org:team", Kind: "team"}}
			case "duplicate owner":
				users[1] = users[0]
			case "ambiguous login":
				users[1].Login = "ONE"
			}
			body = map[string]any{"users": users}
		case req.URL.Host == "crates.io" && strings.HasPrefix(req.URL.Path, "/api/v1/crates/pkg/"):
			version := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
			if _, ok := stableVersion("cargo", version); !ok {
				t.Fatal("unexpected publisher/history lookup", req.URL)
			}
			if mode == "unavailable" {
				return response(`{}`, 404), nil
			}
			file := map[string]any{"crate": "pkg", "num": version, "description": "Useful Rust library", "yanked": mode == "yanked"}
			if mode == "missing yanked" {
				delete(file, "yanked")
			}
			body = map[string]any{"version": file}
		default:
			t.Fatal("unexpected archive/SDK/feature/privileged request", req.URL)
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestCargoPackageAdvice(t *testing.T) {
	for _, tt := range []struct{ mode, decision, version string }{
		{"normal", Allow, "3.0.0"}, {"young", Allow, "2.0.0"}, {"team", Allow, "3.0.0"}, {"team only", Allow, "3.0.0"},
		{"unknown owners", Allow, "3.0.0"}, {"duplicate owner", Allow, "3.0.0"}, {"ambiguous login", Allow, "3.0.0"},
		{"single owner", Deny, ""}, {"unverified", Unknown, ""}, {"yanked", Deny, ""},
		{"unavailable", Deny, ""}, {"missing yanked", Unknown, ""}, {"osv failure", Unknown, ""}, {"metadata vulnerability", Deny, ""},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			r := cargoAdviceClient(t, tt.mode).AssessPackage(context.Background(), "cargo", "pkg", nil, PackageAdvicePolicy(14), checked)
			if r.Decision != tt.decision || r.Scope != "package" || r.Coverage != "direct-package-only" || r.Details.Project != nil {
				t.Fatal(r)
			}
			if tt.version != "" && (r.Recommendation == nil || r.Recommendation.Version != tt.version || r.Description == nil) {
				t.Fatal(r)
			}
			if tt.mode != "normal" && tt.mode != "young" && tt.version != "" && !slices.Contains(r.Warnings, "MAINTAINER_COUNT_UNKNOWN") {
				t.Fatal(r.Warnings)
			}
			if slices.Contains(r.Evidence.Missing, "PROJECT_RESOLUTION") || slices.Contains(r.Evidence.Missing, "ACTIVE_MAINTAINERS") {
				t.Fatal(r)
			}
		})
	}
	v := "2.0.0"
	r := cargoAdviceClient(t, "vulnerability").AssessPackage(context.Background(), "cargo", "pkg", &v, PackageAdvicePolicy(14), checked)
	if r.Decision != Deny || r.Recommendation == nil || r.Recommendation.Version != "3.0.0" {
		t.Fatal(r)
	}
}
