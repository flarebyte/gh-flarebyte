package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestPythonWheelCompatibility(t *testing.T) {
	for _, tt := range []struct {
		filename, manifest, status, reason string
		gap                                bool
	}{
		{"pkg-1.0-py3-none-any.whl", ">=3.9", "compatible", "", false},
		{"pkg-1.0-2build-py2.py3-none-any.whl", ">=3.9.1", "compatible", "", false},
		{"pkg-1.0-py39-none-any.whl", ">=3.10", "compatible", "", false},
		{"pkg-1.0-py310-none-any.whl", ">=3.9", "incompatible", "PYPI_WHEEL_TAG_INCOMPATIBLE", false},
		{"pkg-1.0-py2-none-any.whl", ">=3.9", "incompatible", "PYPI_WHEEL_TAG_INCOMPATIBLE", false},
		{"pkg-1.0-cp39-cp39-manylinux_2_17_x86_64.whl", ">=3.9", "unknown", "", true},
		{"pkg-1.0-cp39-none-any.whl", ">=3.9", "unknown", "", true},
		{"pkg-1.0-cp39.py2-none-any.whl", ">=3.9", "unknown", "", true},
		{"pkg-1.0-cp39.py3-none-any.whl", ">=3.9", "compatible", "", false},
		{"pkg-1.0-py3-none-win_amd64.whl", ">=3.9", "unknown", "", true},
		{"pkg-1.0-py3-none-any.win_amd64.whl", ">=3.9", "compatible", "", false},
		{"pkg-1.0-py2-cp39.none-any.whl", ">=3.9", "unknown", "", true},
		{"pkg-1.0-py3-none-any.whl", "", "unknown", "", true},
		{"pkg-1.0-py3-none-any.whl", ">=3.9,<4", "unknown", "", true},
		{"pkg-1.0-py300-none-any.whl", ">=3.9", "unknown", "", true},
		{"other-1.0-py3-none-any.whl", ">=3.9", "conflict", "IDENTITY_CONFLICT", false},
		{"pkg-2.0-py3-none-any.whl", ">=3.9", "conflict", "IDENTITY_CONFLICT", false},
		{"pkg-1.0-badbuild-py3-none-any.whl", ">=3.9", "unknown", "", true},
		{"pkg-1.0-py3..py2-none-any.whl", ">=3.9", "unknown", "", true},
	} {
		t.Run(tt.filename+tt.manifest, func(t *testing.T) {
			f := Finding{Missing: []string{"PYTHON_ENVIRONMENT_CONTEXT", "PROJECT_RESOLUTION"}}
			checkPythonWheel(tt.filename, "pkg", "1.0.0", tt.manifest, &f)
			if f.Wheel == nil || f.Wheel.Status != tt.status || slices.Contains(f.Missing, "PYPI_WHEEL_COMPATIBILITY") != tt.gap {
				t.Fatal(f)
			}
			if tt.reason != "" && !slices.Contains(f.Reasons, tt.reason) || tt.reason == "" && len(f.Reasons) > 0 {
				t.Fatal(f)
			}
			if !slices.Contains(f.Missing, "PYTHON_ENVIRONMENT_CONTEXT") || !slices.Contains(f.Missing, "PROJECT_RESOLUTION") {
				t.Fatal("approval gates removed")
			}
		})
	}
}

func TestPythonWheelBoundsAndIdentity(t *testing.T) {
	for _, name := range []string{"pkg.whl", "pkg-1.0-py3-none-any.zip", "pkg-1.0rc1-py3-none-any.whl", "pkg-1.0-py3-none-any?.whl", strings.Repeat("x", 256) + "-1.0-py3-none-any.whl", "pkg-1.0-" + strings.Repeat("py3.", 8) + "py2-none-any.whl", "pkg-1.0-py2.py3.py39.py310-py2.py3.py39.py310-any.win32.win_amd64.macosx_11_0_arm64.linux_x86_64.whl"} {
		if _, ok := parsePythonWheel(name); ok {
			t.Fatal(name)
		}
	}
	f := Finding{}
	checkPythonWheel("some_pkg-1.0-py3-none-any.whl", "some-pkg", "1.0.0", ">=3.9", &f)
	if f.Wheel.Status != "compatible" {
		t.Fatal(f)
	}
}

func TestPythonWheelInventoryDenial(t *testing.T) {
	m, l := pythonInventoryDocuments()
	m = strings.Replace(m, "[project]", "[project]\nrequires-python='>=3.9'", 1)
	l = strings.ReplaceAll(l, "py3-none-any", "py2-none-any")
	dir := writePythonInventory(t, m, l)
	c := pythonInventoryClient(t, dir, "normal")
	base := c.HTTP
	c.HTTP = doFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := base.Do(req)
		if err != nil || req.URL.Host != "pypi.org" || strings.HasPrefix(req.URL.Path, "/simple/") {
			return resp, err
		}
		var body map[string]any
		if json.NewDecoder(resp.Body).Decode(&body) != nil {
			t.Fatal("fixture schema")
		}
		resp.Body.Close()
		b, _ := json.Marshal(body)
		return response(strings.ReplaceAll(string(b), "py3-none-any", "py2-none-any"), 200), nil
	})
	v := "1.0.0"
	r := c.AssessLocal(context.Background(), dir, "pypi", "pkg", &v, DefaultPolicy(14), checked)
	if r.Decision != Deny || !slices.Contains(r.Reasons, "PYPI_WHEEL_TAG_INCOMPATIBLE") || !slices.Contains(r.Reasons, "LOCKED_INVENTORY_VIOLATION") || r.Recommendation != nil {
		t.Fatal(r)
	}
	for _, finding := range r.Details.Project.Findings {
		if finding.Finding.Wheel == nil || finding.Finding.Wheel.Status != "incompatible" {
			t.Fatal(finding)
		}
	}
}
