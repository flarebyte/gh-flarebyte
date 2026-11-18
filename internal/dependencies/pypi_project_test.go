package dependencies

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func pythonInventoryDocuments() (string, string) {
	m := "[project]\nname='local'\nversion='1.0.0'\ndependencies=['pkg==1.0.0']\n"
	l := "lock-version='1.0'\ncreated-by='fixture'\nextras=[]\ndependency-groups=[]\n"
	for _, name := range []string{"pkg", "child"} {
		l += fmt.Sprintf("[[packages]]\nname='%s'\nversion='1.0.0'\nindex='https://pypi.org/simple/'\n[[packages.wheels]]\nname='%s-1.0.0-py3-none-any.whl'\nurl='https://files.pythonhosted.org/packages/ab/%s-1.0.0-py3-none-any.whl'\nsize=123\nhashes={sha256='%s'}\n", name, name, name, strings.Repeat("a", 64))
	}
	return m, l
}
func writePythonInventory(t *testing.T, m, l string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"pyproject.toml": m, "pylock.toml": l} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReadPythonInventory(t *testing.T) {
	m, l := pythonInventoryDocuments()
	dir := writePythonInventory(t, m, l)
	p := ReadProject(dir, "pypi")
	if !p.InventoryComplete || p.Complete || len(p.Nodes) != 2 || p.Resolution != "locked-inventory" || p.binds("pkg", "1.0.0") || !p.unchanged(dir) || !slices.Contains(p.Missing, "PYPI_RESOLUTION_UNVERIFIED") {
		t.Fatal(p)
	}
	if err := os.WriteFile(filepath.Join(dir, "pylock.toml"), []byte(l+"\n# changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if p.unchanged(dir) {
		t.Fatal("changed inventory ignored")
	}
	for _, test := range []struct {
		name string
		m, l string
		gap  string
	}{
		{"range", strings.Replace(m, "pkg==1.0.0", "pkg>=1", 1), l, "PROJECT_PYTHON_REQUIREMENTS"},
		{"marker", strings.Replace(m, "pkg==1.0.0", "pkg==1.0.0; python_version > 3", 1), l, "PROJECT_PYTHON_REQUIREMENTS"},
		{"dynamic", m + "dynamic=['dependencies']\n", l, "PROJECT_PYTHON_CONTEXT"},
		{"groups", m + "[dependency-groups]\ndev=[]\n", l, "PROJECT_PYTHON_CONTEXT"},
		{"tool overrides", m + "[tool.uv]\n", l, "PROJECT_PYTHON_CONTEXT"},
		{"lock version", m, strings.Replace(l, "lock-version='1.0'", "lock-version='2.0'", 1), "PROJECT_LOCK_FORMAT"},
		{"stale", strings.Replace(m, "pkg==1.0.0", "pkg==2.0.0", 1), l, "PROJECT_LOCK_STALE"},
		{"prerelease", m, strings.Replace(l, "version='1.0.0'", "version='1.0.0rc1'", 1), "PROJECT_VERSION_SEMANTICS"},
		{"foreign index", m, strings.Replace(l, "https://pypi.org/simple/", "https://evil.test/simple/", 1), "PROJECT_NONREGISTRY_DEPENDENCY"},
		{"foreign wheel", m, strings.Replace(l, "files.pythonhosted.org", "evil.test", 1), "PROJECT_ARTIFACT_IDENTITY"},
		{"bad hash", m, strings.Replace(l, strings.Repeat("a", 64), "bad", 1), "PROJECT_ARTIFACT_IDENTITY"},
		{"local wheel", m, strings.Replace(l, "size=123", "path='local.whl'\nsize=123", 1), "PROJECT_NONREGISTRY_DEPENDENCY"},
		{"wrong filename", m, strings.Replace(l, "name='pkg-1.0.0-py3-none-any.whl'", "name='other.whl'", 1), "PROJECT_ARTIFACT_IDENTITY"},
		{"bad size", m, strings.Replace(l, "size=123", "size=-1", 1), "PROJECT_ARTIFACT_IDENTITY"},
		{"sdist", m, strings.Replace(l, "index='https://pypi.org/simple/'", "sdist={url='https://evil.test/pkg.tar.gz'}", 1), "PROJECT_NONREGISTRY_DEPENDENCY"},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := ReadProject(writePythonInventory(t, test.m, test.l), "pypi")
			if p.InventoryComplete || p.Complete || !slices.Contains(p.Missing, test.gap) {
				t.Fatal(p)
			}
		})
	}
	for _, raw := range []string{"http://files.pythonhosted.org/packages/pkg.whl", "https://files.pythonhosted.org/packages/../pkg.whl", "https://files.pythonhosted.org/packages/pkg.whl?token=secret", "https://files.pythonhosted.org/packages/pkg.tar.gz", "https://user@files.pythonhosted.org/packages/pkg.whl"} {
		if _, ok := pypiWheelURL(raw); ok {
			t.Fatal("unsafe artifact accepted", raw)
		}
	}
}

func pythonInventoryClient(t *testing.T, dir, mode string) *Client {
	t.Helper()
	return &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("credential sent")
		}
		var body any
		switch req.URL.Host {
		case "api.deps.dev":
			if mode == "root lookup vulnerable" && !strings.Contains(req.URL.Path, "/versions/") {
				return response(`{}`, 404), nil
			}
			if strings.Contains(req.URL.Path, "/versions/") {
				parts := strings.Split(req.URL.Path, "/")
				name := parts[len(parts)-3]
				if mode == "lookup vulnerable" && name == "child" {
					return response(`{}`, 404), nil
				}
				body = Version{Key: VersionKey{System: "PYPI", Name: name, Version: "1.0.0"}, PublishedAt: checked.AddDate(-1, 0, 0)}
			} else {
				body = map[string]any{"packageKey": map[string]string{"name": "pkg"}, "versions": []any{map[string]any{"versionKey": map[string]string{"version": "1.0.0"}}}}
			}
		case "api.osv.dev":
			var q struct {
				Package struct {
					Name      string `json:"name"`
					Ecosystem string `json:"ecosystem"`
				} `json:"package"`
				Version string `json:"version"`
			}
			if json.NewDecoder(req.Body).Decode(&q) != nil || q.Package.Ecosystem != "PyPI" || q.Version != "1.0.0" {
				t.Fatal("bad OSV identity", q)
			}
			if mode == "mutation" {
				if err := os.WriteFile(filepath.Join(dir, "pylock.toml"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if (mode == "vulnerable" || mode == "lookup vulnerable" || mode == "root lookup vulnerable") && q.Package.Name == "child" {
				return response(`{"vulns":[{"id":"GHSA-inventory-child"}]}`, 200), nil
			}
			body = map[string]any{}
		case "pypi.org":
			parts := strings.Split(req.URL.Path, "/")
			name := parts[2]
			if mode == "unavailable" && name == "child" {
				return response(`{}`, 404), nil
			}
			if mode == "lookup" && name == "child" {
				return response(`{}`, 403), nil
			}
			info := map[string]any{"name": name, "version": "1.0.0", "summary": "Synthetic library"}
			file := map[string]any{"filename": name + "-1.0.0-py3-none-any.whl", "url": "https://files.pythonhosted.org/packages/ab/" + name + "-1.0.0-py3-none-any.whl", "digests": map[string]string{"sha256": strings.Repeat("a", 64)}, "yanked": false, "size": 123}
			if mode == "python runtime" {
				file["requires_python"] = ">=3.10"
			}
			files := []any{file}
			if name == "child" {
				switch mode {
				case "hash":
					file["digests"] = map[string]string{"sha256": strings.Repeat("b", 64)}
				case "identity":
					info["name"] = "other"
				case "yanked":
					file["yanked"] = true
				case "missing artifact":
					files = []any{}
				case "size":
					file["size"] = 321
				case "missing yanked":
					delete(file, "yanked")
				case "schema":
					file["digests"] = map[string]string{"sha256": "bad"}
				case "duplicate":
					files = append(files, file)
				}
			}
			body = map[string]any{"info": info, "urls": files}
		default:
			t.Fatalf("unexpected request (no artifacts or tools permitted): %s", req.URL)
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestPythonInventoryRuntimeDenial(t *testing.T) {
	m, l := pythonInventoryDocuments()
	m = strings.Replace(m, "[project]", "[project]\nrequires-python='>=3.9'", 1)
	dir := writePythonInventory(t, m, l)
	version := "1.0.0"
	r := pythonInventoryClient(t, dir, "python runtime").AssessLocal(context.Background(), dir, "pypi", "pkg", &version, DefaultPolicy(14), checked)
	if r.Decision != Deny || !slices.Contains(r.Reasons, "PYTHON_VERSION_INCOMPATIBLE") || !slices.Contains(r.Reasons, "LOCKED_INVENTORY_VIOLATION") || r.Recommendation != nil {
		t.Fatal(r)
	}
	for _, node := range r.Details.Project.Findings {
		if node.Finding.Runtime == nil || node.Finding.Runtime.Status != "incompatible" {
			t.Fatal(node)
		}
	}
}

func TestAssessPythonInventory(t *testing.T) {
	for _, test := range []struct{ mode, decision, reason string }{
		{"normal", Unknown, ""}, {"vulnerable", Deny, "KNOWN_VULNERABILITY"}, {"lookup vulnerable", Deny, "KNOWN_VULNERABILITY"}, {"root lookup vulnerable", Deny, "KNOWN_VULNERABILITY"}, {"hash", Deny, "ARTIFACT_MISMATCH"}, {"identity", Deny, "IDENTITY_CONFLICT"}, {"yanked", Deny, "WITHDRAWN_RELEASE"}, {"unavailable", Deny, "WITHDRAWN_RELEASE"}, {"missing artifact", Deny, "ARTIFACT_MISMATCH"}, {"size", Deny, "ARTIFACT_MISMATCH"}, {"missing yanked", Unknown, ""}, {"schema", Unknown, ""}, {"duplicate", Unknown, ""}, {"lookup", Unknown, ""}, {"mutation", Unknown, ""},
	} {
		t.Run(test.mode, func(t *testing.T) {
			m, l := pythonInventoryDocuments()
			dir := writePythonInventory(t, m, l)
			c := pythonInventoryClient(t, dir, test.mode)
			version := "1.0.0"
			r := c.AssessLocal(context.Background(), dir, "pypi", "pkg", &version, DefaultPolicy(14), checked)
			if r.Decision != test.decision || r.Scope != "package" || r.Recommendation != nil {
				t.Fatal(r.Decision, r.Scope, r.Evidence, r.Reasons)
			}
			if test.reason != "" && !slices.Contains(r.Reasons, test.reason) {
				t.Fatal(r.Reasons)
			}
			if len(r.Details.Project.Findings) != 2 || r.Details.Project.Complete {
				t.Fatal("inventory not audited or promoted", r.Details.Project)
			}
			if test.mode == "normal" && (!r.Details.Project.InventoryComplete || !slices.Contains(r.Evidence.Missing, "PROJECT_RESOLUTION")) {
				t.Fatal(r.Details.Project, r.Evidence)
			}
			if test.mode == "mutation" && !slices.Contains(r.Evidence.Missing, "PROJECT_CHANGED_DURING_ASSESSMENT") {
				t.Fatal("changed inputs not detected")
			}
		})
	}
}
