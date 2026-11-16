package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func cargoClient(t *testing.T, dir, mode string) *Client {
	t.Helper()
	authority := authorityClient(t, "normal")
	return &Client{GitHubToken: authority.GitHubToken, HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		var payload any
		switch r.URL.Host {
		case "api.github.com":
			return authority.HTTP.Do(r)
		case "api.scorecard.dev":
			payload = goodCard()
		case "api.osv.dev":
			var q struct {
				Package struct {
					Name      string `json:"name"`
					Ecosystem string `json:"ecosystem"`
				} `json:"package"`
				Version string `json:"version"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				t.Fatal(err)
			}
			if q.Package.Ecosystem != "crates.io" || q.Version != "1.0.0" {
				t.Fatal("wrong OSV identity", q)
			}
			if mode == "mutation" {
				if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname='edit'"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if (mode == "vulnerable" || mode == "lookup vulnerable") && q.Package.Name == "child" {
				return response(`{"vulns":[{"id":"RUSTSEC-2026-0001"}]}`, 200), nil
			}
			return response(`{}`, 200), nil
		case "api.deps.dev":
			if strings.Contains(r.URL.Path, "/projects/") {
				payload = map[string]any{"projectKey": map[string]string{"id": "github.com/org/repo"}, "starsCount": 4200}
				break
			}
			if strings.Contains(r.URL.Path, "/versions/") {
				parts := strings.Split(r.URL.Path, "/")
				name := parts[len(parts)-3]
				if (mode == "lookup failure" || mode == "lookup vulnerable") && name == "child" {
					return response(`{}`, 404), nil
				}
				v := Version{Key: VersionKey{System: "CARGO", Name: name, Version: "1.0.0"}, PublishedAt: checked.AddDate(-1, 0, 0), Attestations: []Attestation{{Type: "https://slsa.dev/provenance/v1", Verified: true, SourceRepository: "https://github.com/org/repo", Commit: strings.Repeat("a", 40)}}}
				if mode == "unverified" {
					v.Attestations = nil
				}
				payload = v
				break
			}
			payload = map[string]any{"packageKey": map[string]string{"name": "pkg"}, "versions": []any{map[string]any{"versionKey": map[string]string{"version": "1.0.0"}}}}
		case "crates.io":
			parts := strings.Split(r.URL.Path, "/")
			name := parts[4]
			if strings.HasSuffix(r.URL.Path, "/dependencies") {
				if name == "child" {
					payload = map[string]any{"dependencies": []any{}}
					break
				}
				if mode == "dependency failure" {
					return response(`{}`, 404), nil
				}
				if mode == "dependency schema" {
					return response(`{}`, 200), nil
				}
				deps := []any{map[string]any{"crate_id": "child", "req": "^1.0.0", "kind": "normal", "optional": false, "default_features": true, "features": []string{}}}
				if mode == "missing edge" {
					deps = []any{}
				}
				if mode == "wrong requirement" {
					deps[0].(map[string]any)["req"] = "^2.0.0"
				}
				if mode == "unknown range" {
					deps[0].(map[string]any)["req"] = "1 || 2"
				}
				if mode == "unknown kind" {
					deps[0].(map[string]any)["kind"] = "future"
				}
				if mode == "missing optional flag" {
					delete(deps[0].(map[string]any), "optional")
				}
				if mode == "optional absent" {
					deps = append(deps, map[string]any{"crate_id": "other", "req": "^1.0.0", "kind": "normal", "optional": true, "default_features": true, "features": []string{}})
				}
				if mode == "mandatory absent" {
					deps = append(deps, map[string]any{"crate_id": "other", "req": "^1.0.0", "kind": "build", "optional": false, "default_features": true, "features": []string{}})
				}
				if mode == "dev absent" {
					deps = append(deps, map[string]any{"crate_id": "other", "req": "^1.0.0", "kind": "dev", "optional": false, "default_features": true, "features": []string{}})
				}
				payload = map[string]any{"dependencies": deps}
				break
			}
			if mode == "artifact failure" {
				return response(`{}`, 404), nil
			}
			if mode == "artifact schema" {
				return response(`{}`, 200), nil
			}
			version := map[string]any{"crate": name, "num": "1.0.0", "yanked": false, "checksum": strings.Repeat("a", 64), "rust_version": nil, "features": map[string][]string{}}
			if mode == "checksum" {
				version["checksum"] = strings.Repeat("b", 64)
			}
			if mode == "checksum schema" {
				delete(version, "checksum")
			}
			if mode == "lifecycle schema" {
				delete(version, "yanked")
			}
			if mode == "identity" {
				version["num"] = "2.0.0"
			}
			if mode == "runtime" {
				version["rust_version"] = "1.85"
			}
			if mode == "yanked" && name == "child" {
				version["yanked"] = true
			}
			payload = map[string]any{"version": version}
		default:
			t.Fatalf("unexpected provider %s", r.URL)
		}
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestAssessCargoProject(t *testing.T) {
	for _, test := range []struct{ mode, decision, reason, missing string }{
		{"normal", Allow, "", ""}, {"dev absent", Allow, "", ""},
		{"vulnerable", Deny, "KNOWN_VULNERABILITY", ""}, {"checksum", Deny, "ARTIFACT_MISMATCH", ""},
		{"lookup vulnerable", Deny, "KNOWN_VULNERABILITY", ""},
		{"identity", Deny, "IDENTITY_CONFLICT", ""}, {"missing edge", Deny, "DEPENDENCY_METADATA_MISMATCH", ""},
		{"wrong requirement", Deny, "DEPENDENCY_METADATA_MISMATCH", ""}, {"mandatory absent", Deny, "DEPENDENCY_METADATA_MISMATCH", ""},
		{"yanked", Deny, "WITHDRAWN_RELEASE", ""}, {"runtime", Unknown, "", "TRANSITIVE_EVIDENCE"},
		{"unverified", Unknown, "", "PACKAGE_IDENTITY"}, {"lookup failure", Unknown, "", "TRANSITIVE_EVIDENCE"},
		{"optional absent", Unknown, "", "TRANSITIVE_EVIDENCE"}, {"unknown range", Unknown, "", "TRANSITIVE_EVIDENCE"},
		{"unknown kind", Unknown, "", "TRANSITIVE_EVIDENCE"}, {"missing optional flag", Unknown, "", "TRANSITIVE_EVIDENCE"},
		{"checksum schema", Unknown, "", "TRANSITIVE_EVIDENCE"}, {"dependency schema", Unknown, "", "TRANSITIVE_EVIDENCE"},
		{"lifecycle schema", Unknown, "", "TRANSITIVE_EVIDENCE"},
		{"dependency failure", Unknown, "", "TRANSITIVE_EVIDENCE"}, {"artifact failure", Deny, "WITHDRAWN_RELEASE", ""},
		{"artifact schema", Unknown, "", "TRANSITIVE_EVIDENCE"},
		{"mutation", Unknown, "", "PROJECT_CHANGED_DURING_ASSESSMENT"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			m, l := cargoDocuments()
			dir := writeCargoDocuments(t, m, l)
			v := "1.0.0"
			r := cargoClient(t, dir, test.mode).AssessLocal(context.Background(), dir, "cargo", "pkg", &v, DefaultPolicy(14), checked)
			if r.Decision != test.decision || test.reason != "" && !slices.Contains(r.Reasons, test.reason) || test.missing != "" && !slices.Contains(r.Evidence.Missing, test.missing) {
				t.Fatalf("unexpected verdict: %+v", r)
			}
			if test.mode == "mutation" {
				if r.Scope != "package" || r.Recommendation != nil {
					t.Fatal("changed project approved")
				}
				return
			}
			if r.Scope != "project-resolution" || len(r.Details.Project.Findings) != 2 || !r.Details.Project.unchanged(dir) {
				t.Fatal(r.Scope, r.Details.Project)
			}
			if test.decision == Allow {
				if r.Recommendation == nil || r.Recommendation.Version != "1.0.0" {
					t.Fatal(r.Recommendation)
				}
				if _, err := PrepareInstall(dir, r); err == nil {
					t.Fatal("Cargo native install enabled")
				}
			} else if r.Recommendation != nil {
				t.Fatal("unsafe recommendation", r.Recommendation)
			}
			if test.mode == "vulnerable" {
				found := false
				for _, f := range r.Details.Project.Findings {
					found = found || f.Path == "Cargo.lock#child@1.0.0" && slices.Contains(f.Finding.Advisories, "RUSTSEC-2026-0001")
				}
				if !found {
					t.Fatal("missing dependency identity")
				}
			}
			for _, f := range r.Details.Project.Findings {
				if f.Finding.Evidence != nil && !slices.Equal(f.Finding.Missing, f.Finding.Evidence.Missing) {
					t.Fatal("inconsistent explained evidence")
				}
			}
		})
	}
}

func TestCargoFeaturesRemainUnknown(t *testing.T) {
	for _, manifest := range []string{
		"[features]\ndefault=['missing']\n",
		"[features]\ncustom=['pkg/derive']\n",
	} {
		m, l := cargoDocuments()
		dir := writeCargoDocuments(t, m+manifest, l)
		v := "1.0.0"
		r := cargoClient(t, dir, "normal").AssessLocal(context.Background(), dir, "cargo", "pkg", &v, DefaultPolicy(14), checked)
		if r.Decision != Unknown || !slices.Contains(r.Evidence.Missing, "TRANSITIVE_EVIDENCE") {
			t.Fatal("unresolved feature activation approved", r)
		}
	}
}
