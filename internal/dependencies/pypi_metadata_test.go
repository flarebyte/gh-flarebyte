package dependencies

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestPythonDistributionMetadataParser(t *testing.T) {
	valid := "Metadata-Version: 2.4\nName: pkg\nVersion: 1.0.0\nRequires-Dist: child\n (>=1.0,<2.0)\n\nDescription\n"
	m, ok := parsePyPIMetadata([]byte(strings.ReplaceAll(valid, "\n", "\r\n")))
	if !ok || m.Name != "pkg" || len(m.Dependencies) != 1 {
		t.Fatal(m, ok)
	}
	for _, body := range []string{
		strings.Replace(valid, "2.4", "3.0", 1), strings.Replace(valid, "Name: pkg", "Name: pkg\nName: other", 1),
		strings.Replace(valid, "Version: 1.0.0", "Version: 1.0.0\nVersion: 2.0.0", 1),
		strings.Replace(valid, "Name: pkg", "Name: invalid/name", 1),
		strings.Replace(valid, "Version: 1.0.0", "Version: 1.0.0rc1", 1),
		strings.Replace(valid, "Name: pkg", "Name: p\x00kg", 1),
		strings.Replace(valid, "Name: pkg", "Name: p\x01kg", 1),
		strings.Replace(valid, "Name: pkg", "Name: p\xffkg", 1),
		strings.Replace(valid, "Name: pkg", "Name: p\rkg", 1),
		strings.Replace(valid, "Name: pkg", "Name: pkg\nRequires-Python: >=3.9\nRequires-Python: >=3.10", 1),
		"Metadata-Version: 2.4\nName: pkg\nVersion: 1.0.0\n",
		"Metadata-Version: 2.4\nName: pkg\nVersion: 1.0.0\n" + strings.Repeat("Requires-Dist: child\n", 257) + "\n",
		strings.Repeat("x", maxPyPIMetadata+1),
	} {
		if _, ok := parsePyPIMetadata([]byte(body)); ok {
			t.Fatal("invalid metadata accepted")
		}
	}
	for _, raw := range []string{"child", "Child_Name (>=1.0, <2.0)", "child==1!2.0"} {
		if _, _, ok := pythonDistributionRequirement(raw); !ok {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"child; python_version > '3'", "child[extra]", "child @ https://evil.test", "child~=1.0", "child>1.0.post1", "child==1.*", "child>=1,bad", "child==1.0rc1", "child ()"} {
		if _, _, ok := pythonDistributionRequirement(raw); ok {
			t.Fatal(raw)
		}
	}
}

func TestPythonWheelMetadataEvidence(t *testing.T) {
	for _, tt := range []struct {
		mode, reason       string
		complete, download bool
	}{
		{"verified", "", true, true}, {"legacy", "", true, true},
		{"missing dependency", "PYPI_LOCK_DEPENDENCY_MISSING", true, true},
		{"constraint conflict", "DEPENDENCY_METADATA_MISMATCH", true, true},
		{"identity", "IDENTITY_CONFLICT", false, true},
		{"hash conflict", "METADATA_HASH_MISMATCH", false, true},
		{"wheel hash", "ARTIFACT_MISMATCH", false, false},
		{"marker", "", false, true}, {"malformed", "", false, true},
		{"inactive marker", "", true, true},
		{"active marker", "PYPI_LOCK_DEPENDENCY_MISSING", true, true},
		{"partial marker", "", false, true},
		{"duplicate file", "", false, false}, {"unhashed", "", false, false},
		{"legacy conflict", "", false, false}, {"unavailable", "", false, true},
		{"oversized", "", false, true},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			manifest, lock := pythonInventoryDocuments()
			manifest = strings.Replace(manifest, "[project]", "[project]\nrequires-python='>=3.9'", 1)
			project := ReadProject(writePythonInventory(t, manifest, lock), "pypi")
			var node ProjectNode
			for _, n := range project.Nodes {
				if n.Name == "pkg" {
					node = n
				}
			}
			body := "Metadata-Version: 2.4\nName: pkg\nVersion: 1.0.0\nRequires-Dist: child (>=1.0,<2.0)\n\n"
			switch tt.mode {
			case "missing dependency":
				body = strings.Replace(body, "child (>=1.0,<2.0)", "missing==1.0", 1)
			case "constraint conflict":
				body = strings.Replace(body, ">=1.0,<2.0", ">=2.0", 1)
			case "marker":
				body = strings.Replace(body, "child (>=1.0,<2.0)", "missing; extra == 'optional'", 1)
			case "inactive marker":
				body = strings.Replace(body, "child (>=1.0,<2.0)", "missing; python_version < '3.8'", 1)
			case "active marker":
				body = strings.Replace(body, "child (>=1.0,<2.0)", "missing; python_version >= '3.8'", 1)
			case "partial marker":
				body = strings.Replace(body, "child (>=1.0,<2.0)", "missing; python_version >= '3.10'", 1)
			case "identity":
				body = strings.Replace(body, "Name: pkg", "Name: other", 1)
			case "malformed":
				body = "malformed\n\n"
			case "oversized":
				body = strings.Repeat("x", maxPyPIMetadata+1)
			}
			sum := sha256.Sum256([]byte(body))
			hash := hex.EncodeToString(sum[:])
			if tt.mode == "hash conflict" {
				hash = strings.Repeat("b", 64)
			}
			downloads := 0
			c := &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("Authorization") != "" {
					t.Fatal("credential leak")
				}
				if req.URL.Host == "files.pythonhosted.org" {
					if req.URL.String() != node.Resolved+".metadata" {
						t.Fatal("distribution request", req.URL)
					}
					downloads++
					if tt.mode == "unavailable" {
						return response("", 404), nil
					}
					return response(body, 200), nil
				}
				if req.URL.String() != "https://pypi.org/simple/pkg/" || req.Header.Get("Accept") != "application/vnd.pypi.simple.v1+json" {
					t.Fatal(req.URL, req.Header)
				}
				file := map[string]any{"filename": project.pypi[node.Path].Filename, "url": node.Resolved, "hashes": map[string]string{"sha256": strings.Repeat("a", 64)}, "core-metadata": map[string]string{"sha256": hash}}
				switch tt.mode {
				case "wheel hash":
					file["hashes"] = map[string]string{"sha256": strings.Repeat("c", 64)}
				case "unhashed":
					file["core-metadata"] = true
				case "legacy":
					delete(file, "core-metadata")
					file["dist-info-metadata"] = map[string]string{"sha256": hash}
				case "legacy conflict":
					file["dist-info-metadata"] = map[string]string{"sha256": strings.Repeat("c", 64)}
				}
				files := []any{file}
				if tt.mode == "duplicate file" {
					files = append(files, file)
				}
				b, _ := json.Marshal(map[string]any{"meta": map[string]string{"api-version": "1.4"}, "name": "pkg", "files": files})
				return response(string(b), 200), nil
			})}
			f := Finding{Missing: []string{"PYPI_DISTRIBUTION_METADATA", "PYTHON_ENVIRONMENT_CONTEXT"}}
			sources := []Source{}
			c.checkPyPIMetadata(withBudget(context.Background()), project, node, &f, &sources)
			if (downloads == 1) != tt.download || slices.Contains(f.Missing, "PYPI_DISTRIBUTION_METADATA") == tt.complete {
				t.Fatal(downloads, f)
			}
			if tt.reason != "" && !slices.Contains(f.Reasons, tt.reason) || tt.reason == "" && len(f.Reasons) > 0 {
				t.Fatal(f)
			}
			if !slices.Contains(f.Missing, "PYTHON_ENVIRONMENT_CONTEXT") {
				t.Fatal("environment gate removed")
			}
			if tt.complete && (f.Distribution == nil || f.Distribution.DependencyCount != 1) {
				t.Fatal(f)
			}
		})
	}
}

func TestPythonMetadataOriginAndBudget(t *testing.T) {
	c := &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) { return response("valid", 200), nil })}
	sources := []Source{}
	if _, err := c.readPyPIMetadata(context.Background(), "https://evil.test/pkg.whl", &sources); err == nil || len(sources) != 0 {
		t.Fatal(err, sources)
	}
	ctx := withBudget(context.Background())
	if err := consumeBytes(ctx, 32<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := c.readPyPIMetadata(ctx, "https://files.pythonhosted.org/packages/pkg.whl", &sources); err == nil {
		t.Fatal("budget ignored")
	}
}
