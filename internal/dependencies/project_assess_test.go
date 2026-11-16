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

func projectClient(t *testing.T, dir, mode string) *Client {
	t.Helper()
	base := fixtureClient(t, "normal")
	return &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "registry.npmjs.org":
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			name, version := parts[0], parts[len(parts)-1]
			release := map[string]any{"name": name, "version": version, "description": "Package description", "dist": map[string]string{"tarball": "https://registry.npmjs.org/" + name + "/-/" + name + "-" + version + ".tgz", "integrity": testIntegrity()}}
			if name == "pkg" && mode != "metadata" {
				release["dependencies"] = map[string]string{"child": "1.0.0"}
			}
			if mode == "badartifact" {
				release["dist"].(map[string]string)["integrity"] = "sha512-invalid"
			}
			if mode == "runtime" {
				release["engines"] = map[string]string{"node": ">=18"}
			}
			if mode == "bundled" {
				release["bundledDependencies"] = []string{"hidden"}
			}
			if mode == "shrinkwrap" {
				release["_hasShrinkwrap"] = true
			}
			b, err := json.Marshal(release)
			if err != nil {
				t.Fatal(err)
			}
			return response(string(b), 200), nil
		case "api.osv.dev":
			var q struct {
				Package struct {
					Name string `json:"name"`
				} `json:"package"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				t.Fatal(err)
			}
			if mode == "mutation" {
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "vulnerable" && q.Package.Name == "child" {
				return response(`{"vulns":[{"id":"GHSA-transitive"}]}`, 200), nil
			}
			return response(`{}`, 200), nil
		case "api.deps.dev":
			if strings.Contains(r.URL.Path, "/packages/child/versions/") {
				if mode == "lookupfailure" {
					return response(`{}`, 503), nil
				}
				return response(`{"versionKey":{"system":"NPM","name":"child","version":"1.0.0"},"publishedAt":"2025-01-01T00:00:00Z"}`, 200), nil
			}
		}
		return base.HTTP.Do(r)
	})}
}

func projectWithChild(t *testing.T) string {
	t.Helper()
	m, l := projectDocuments()
	packages := l["packages"].(map[string]any)
	packages["node_modules/pkg"].(map[string]any)["dependencies"] = map[string]string{"child": "1.0.0"}
	packages["node_modules/child"] = map[string]any{"version": "1.0.0", "resolved": "https://registry.npmjs.org/child/-/child-1.0.0.tgz", "integrity": testIntegrity()}
	return writeProjectDocuments(t, m, l)
}

func TestAssessLocalProject(t *testing.T) {
	for _, tt := range []struct{ mode, reason, missing string }{
		{"normal", "", "TRANSITIVE_EVIDENCE"},
		{"vulnerable", "KNOWN_VULNERABILITY", ""},
		{"badartifact", "ARTIFACT_MISMATCH", ""},
		{"metadata", "DEPENDENCY_METADATA_MISMATCH", ""},
		{"lookupfailure", "", "TRANSITIVE_EVIDENCE"},
		{"runtime", "", "RUNTIME_COMPATIBILITY"},
		{"bundled", "", "TRANSITIVE_EVIDENCE"},
		{"shrinkwrap", "", "TRANSITIVE_EVIDENCE"},
		{"mutation", "", "PROJECT_CHANGED_DURING_ASSESSMENT"},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			dir := projectWithChild(t)
			v := "1.0.0"
			r := projectClient(t, dir, tt.mode).AssessLocal(context.Background(), dir, "npm", "pkg", &v, DefaultPolicy(14), checked)
			if r.Decision == Allow || r.Recommendation != nil {
				t.Fatal("unverified authority approved")
			}
			if tt.reason != "" && !slices.Contains(r.Reasons, tt.reason) {
				t.Fatalf("%+v", r)
			}
			if tt.missing != "" && !slices.Contains(r.Evidence.Missing, tt.missing) {
				t.Fatalf("%+v", r)
			}
			if tt.mode == "mutation" {
				if r.Scope != "package" || r.Details.Project.Complete {
					t.Fatal("changed plan approved")
				}
			} else if r.Scope != "project-resolution" {
				t.Fatal(r.Scope)
			}
			if tt.mode == "vulnerable" {
				if !slices.Contains(r.Evidence.Advisories, "GHSA-transitive") {
					t.Fatal(r.Evidence)
				}
				found := false
				for _, node := range r.Details.Project.Findings {
					found = found || node.Path == "node_modules/child" && slices.Contains(node.Finding.Advisories, "GHSA-transitive")
				}
				if !found {
					t.Fatal("missing dependency path")
				}
			}
		})
	}
	dir := projectWithChild(t)
	r := projectClient(t, dir, "normal").AssessLocal(context.Background(), dir, "npm", "pkg", nil, DefaultPolicy(14), checked)
	if len(r.Details.Project.Findings) != 2 {
		t.Fatal("locked version omitted from candidate budget", r.Details)
	}
	r = projectClient(t, dir, "normal").AssessLocal(context.Background(), t.TempDir(), "npm", "pkg", nil, DefaultPolicy(14), checked)
	if !slices.Contains(r.Evidence.Missing, "PROJECT_MANIFEST") {
		t.Fatal(r.Evidence)
	}
}

func TestProjectAssessmentBudgets(t *testing.T) {
	c := fixtureClient(t, "versionfailure")
	project := &ProjectSnapshot{Complete: true, Missing: []string{}, Nodes: []ProjectNode{}}
	for i := 0; i < maxAssessedNodes+1; i++ {
		project.Nodes = append(project.Nodes, ProjectNode{Name: "pkg", Version: "1.0.0", Path: "node_modules/pkg"})
	}
	r := NewReport("npm", "pkg", nil, DefaultPolicy(14), checked)
	c.assessProject(context.Background(), project, DefaultPolicy(14), checked, &r, map[string]TrustEvidence{})
	if len(project.Findings) != maxAssessedNodes || !slices.Contains(project.Missing, "PROJECT_ASSESSMENT_LIMIT") {
		t.Fatal(project)
	}
	f := Finding{Reasons: []string{}, Missing: []string{}, Advisories: []string{}}
	mergeProjectFindings(&f, project)
	if !slices.Contains(f.Missing, "PROJECT_ASSESSMENT_INCOMPLETE") {
		t.Fatal(f)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	project.Findings = nil
	c.assessProject(ctx, project, DefaultPolicy(14), checked, &r, map[string]TrustEvidence{})
	if len(project.Findings) > 0 {
		t.Fatal("canceled graph assessed")
	}
	for _, v := range []any{nil, false, []any{}} {
		if hasBundled(v) {
			t.Fatal(v)
		}
	}
	for _, v := range []any{true, []any{"pkg"}, "pkg", map[string]string{"pkg": "1"}} {
		if !hasBundled(v) {
			t.Fatal(v)
		}
	}
}

func TestAssessLeafAddition(t *testing.T) {
	m, l := projectDocuments()
	delete(m, "dependencies")
	packages := l["packages"].(map[string]any)
	delete(packages[""].(map[string]any), "dependencies")
	delete(packages, "node_modules/pkg")
	dir := writeProjectDocuments(t, m, l)
	v := "1.0.0"
	r := projectClient(t, dir, "metadata").AssessLocal(context.Background(), dir, "npm", "pkg", &v, DefaultPolicy(14), checked)
	if r.Scope != "project-resolution" || r.Decision != Unknown || len(r.Details.Project.Plans) == 0 || r.Details.Project.Plans[0].Kind != "add-leaf" || !r.Details.Project.unchanged(dir) {
		t.Fatalf("%+v", r)
	}
	if slices.Contains(r.Evidence.Missing, "PROJECT_RESOLUTION") {
		t.Fatal("supported leaf plan left unresolved")
	}
}
