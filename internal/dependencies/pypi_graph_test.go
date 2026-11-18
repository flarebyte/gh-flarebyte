package dependencies

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func pythonGraphClient(t *testing.T, dir, mode string) *Client {
	t.Helper()
	base := pythonInventoryClient(t, dir, "normal")
	delegate := base.HTTP
	base.HTTP = doFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Fatal("credential sent")
		}
		if req.URL.Host != "files.pythonhosted.org" && !(req.URL.Host == "pypi.org" && strings.HasPrefix(req.URL.Path, "/simple/")) {
			return delegate.Do(req)
		}
		name := "pkg"
		if strings.Contains(req.URL.Path, "child") {
			name = "child"
		}
		body := "Metadata-Version: 2.4\nName: " + name + "\nVersion: 1.0.0\nRequires-Python: >=3.8\n"
		if name == "pkg" {
			requirement := "child>=1.0,<2.0"
			switch mode {
			case "unresolved":
				requirement += "; sys_platform == 'win32'"
			case "active":
				requirement += "; python_version >= '3.8'"
			case "orphan":
				requirement += "; python_version < '3.8'"
			case "missing":
				requirement = "missing==1.0"
			}
			body += "Requires-Dist: " + requirement + "\n"
		} else if mode == "cycle" {
			body += "Requires-Dist: pkg==1.0\n"
		}
		body += "\n"
		if req.URL.Host == "files.pythonhosted.org" {
			if !strings.HasSuffix(req.URL.Path, ".whl.metadata") {
				t.Fatal("artifact download", req.URL)
			}
			if mode == "lookup" && name == "child" {
				return response("", 404), nil
			}
			if mode == "mutation" {
				if err := os.WriteFile(filepath.Join(dir, "pylock.toml"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			return response(body, 200), nil
		}
		sum := sha256.Sum256([]byte(body))
		file := map[string]any{"filename": name + "-1.0.0-py3-none-any.whl", "url": "https://files.pythonhosted.org/packages/ab/" + name + "-1.0.0-py3-none-any.whl", "hashes": map[string]string{"sha256": strings.Repeat("a", 64)}, "core-metadata": map[string]string{"sha256": hex.EncodeToString(sum[:])}}
		b, _ := json.Marshal(map[string]any{"name": name, "meta": map[string]string{"api-version": "1.4"}, "files": []any{file}})
		return response(string(b), 200), nil
	})
	return base
}

func TestPythonGraphClosureAssessment(t *testing.T) {
	for _, tt := range []struct {
		mode, status, gap string
		edges             int
	}{
		{"closed", "closed", "", 1}, {"active", "closed", "", 1}, {"cycle", "closed", "", 2},
		{"unresolved", "incomplete", "PYPI_GRAPH_METADATA_UNVERIFIED", 0},
		{"orphan", "incomplete", "PYPI_GRAPH_UNREACHABLE_NODE", 0},
		{"lookup", "incomplete", "PYPI_GRAPH_METADATA_UNVERIFIED", 1},
		{"missing", "incomplete", "PYPI_GRAPH_METADATA_UNVERIFIED", 0},
		{"mutation", "incomplete", "PROJECT_CHANGED_DURING_ASSESSMENT", 1},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			m, l := pythonInventoryDocuments()
			m = strings.Replace(m, "[project]", "[project]\nrequires-python='>=3.9'", 1)
			dir := writePythonInventory(t, m, l)
			v := "1.0.0"
			r := pythonGraphClient(t, dir, tt.mode).AssessLocal(context.Background(), dir, "pypi", "pkg", &v, DefaultPolicy(14), checked)
			g := r.Details.Project.PythonGraph
			if g == nil || g.Status != tt.status || g.Edges != tt.edges {
				t.Fatal(g)
			}
			if tt.gap != "" && !slices.Contains(g.Missing, tt.gap) {
				t.Fatal(g)
			}
			if r.Decision == Allow || r.Recommendation != nil || r.Details.Project.Complete || r.Scope != "package" || !slices.Contains(r.Evidence.Missing, "PROJECT_RESOLUTION") {
				t.Fatal("closure granted approval", r)
			}
			if tt.mode == "missing" && (r.Decision != Deny || !slices.Contains(r.Reasons, "PYPI_LOCK_DEPENDENCY_MISSING")) {
				t.Fatal(r)
			}
			if tt.status == "closed" && (g.ReachableNodes != 2 || g.Roots != 1) {
				t.Fatal(g)
			}
		})
	}
}

func TestPythonGraphRejectsUnboundFacts(t *testing.T) {
	for _, mode := range []string{"public only", "artifact changed", "native", "root changed", "empty", "unassessed"} {
		t.Run(mode, func(t *testing.T) {
			m, l := pythonInventoryDocuments()
			m = strings.Replace(m, "[project]", "[project]\nrequires-python='>=3.9'", 1)
			dir := writePythonInventory(t, m, l)
			v := "1.0.0"
			r := pythonGraphClient(t, dir, "closed").AssessLocal(context.Background(), dir, "pypi", "pkg", &v, DefaultPolicy(14), checked)
			p := r.Details.Project
			switch mode {
			case "public only":
				p.pypiGraph = nil
			case "artifact changed":
				p.Nodes[0].Integrity = "sha256-" + strings.Repeat("b", 64)
			case "native":
				proof := p.pypiGraph[p.Nodes[0].Path]
				proof.WheelCompatible = false
				p.pypiGraph[p.Nodes[0].Path] = proof
			case "root changed":
				p.runtime["pkg"] = "2.0.0"
			case "empty":
				p.Nodes = nil
			case "unassessed":
				p.Findings = nil
			}
			finishPythonGraph(p)
			if p.PythonGraph.Status == "closed" {
				t.Fatal("unverified facts promoted", p.PythonGraph)
			}
		})
	}
}
