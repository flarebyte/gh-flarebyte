package dependencies

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func testIntegrity() string { return "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64)) }

func projectDocuments() (map[string]any, map[string]any) {
	manifest := map[string]any{"name": "local", "version": "1.0.0", "dependencies": map[string]string{"pkg": "^1.0.0"}}
	root := map[string]any{"name": "local", "version": "1.0.0", "dependencies": map[string]string{"pkg": "^1.0.0"}}
	lock := map[string]any{"lockfileVersion": 3, "packages": map[string]any{
		"":                 root,
		"node_modules/pkg": map[string]any{"version": "1.0.0", "resolved": "https://registry.npmjs.org/pkg/-/pkg-1.0.0.tgz", "integrity": testIntegrity()},
	}}
	return manifest, lock
}

func writeProjectDocuments(t *testing.T, manifest, lock map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	for name, value := range map[string]any{"package.json": manifest, "package-lock.json": lock} {
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReadProject(t *testing.T) {
	for _, tt := range []struct {
		name    string
		change  func(map[string]any, map[string]any)
		missing string
	}{
		{"valid", func(map[string]any, map[string]any) {}, ""},
		{"lock v2", func(_ map[string]any, l map[string]any) { l["lockfileVersion"] = 2 }, ""},
		{"old lock", func(_ map[string]any, l map[string]any) { l["lockfileVersion"] = 1 }, "PROJECT_LOCK_FORMAT"},
		{"no root", func(_ map[string]any, l map[string]any) { delete(l["packages"].(map[string]any), "") }, "PROJECT_LOCK_ROOT"},
		{"workspaces", func(m, _ map[string]any) { m["workspaces"] = []string{"packages/*"} }, "PROJECT_WORKSPACES_OR_OVERRIDES"},
		{"overrides", func(m, _ map[string]any) { m["overrides"] = map[string]string{"pkg": "2.0.0"} }, "PROJECT_WORKSPACES_OR_OVERRIDES"},
		{"stale manifest", func(m, _ map[string]any) { m["dependencies"] = map[string]string{"pkg": "^2.0.0"} }, "PROJECT_LOCK_STALE"},
		{"stale version", func(_ map[string]any, l map[string]any) {
			l["packages"].(map[string]any)["node_modules/pkg"].(map[string]any)["version"] = "2.0.0"
		}, "PROJECT_LOCK_STALE"},
		{"range unknown", func(m, l map[string]any) {
			m["dependencies"] = map[string]string{"pkg": "1.x"}
			l["packages"].(map[string]any)[""].(map[string]any)["dependencies"] = map[string]string{"pkg": "1.x"}
		}, "PROJECT_RANGE_SEMANTICS"},
		{"missing edge", func(_ map[string]any, l map[string]any) { delete(l["packages"].(map[string]any), "node_modules/pkg") }, "PROJECT_GRAPH_INCOMPLETE"},
		{"link", func(_ map[string]any, l map[string]any) {
			l["packages"].(map[string]any)["node_modules/pkg"].(map[string]any)["link"] = true
		}, "PROJECT_NONREGISTRY_DEPENDENCY"},
		{"bundle", func(_ map[string]any, l map[string]any) {
			l["packages"].(map[string]any)["node_modules/pkg"].(map[string]any)["inBundle"] = true
		}, "PROJECT_NONREGISTRY_DEPENDENCY"},
		{"shrinkwrap", func(_ map[string]any, l map[string]any) {
			l["packages"].(map[string]any)["node_modules/pkg"].(map[string]any)["hasShrinkwrap"] = true
		}, "PROJECT_SHRINKWRAP"},
		{"alias", func(_ map[string]any, l map[string]any) {
			l["packages"].(map[string]any)["node_modules/pkg"].(map[string]any)["name"] = "other"
		}, "PROJECT_NONREGISTRY_DEPENDENCY"},
		{"version unknown", func(_ map[string]any, l map[string]any) {
			l["packages"].(map[string]any)["node_modules/pkg"].(map[string]any)["version"] = "1.0.0-rc1"
		}, "PROJECT_VERSION_SEMANTICS"},
		{"foreign registry", func(_ map[string]any, l map[string]any) {
			l["packages"].(map[string]any)["node_modules/pkg"].(map[string]any)["resolved"] = "https://evil.test/pkg.tgz"
		}, "PROJECT_ARTIFACT_IDENTITY"},
		{"weak hash", func(_ map[string]any, l map[string]any) {
			l["packages"].(map[string]any)["node_modules/pkg"].(map[string]any)["integrity"] = "sha1-weak"
		}, "PROJECT_ARTIFACT_IDENTITY"},
		{"path escape", func(_ map[string]any, l map[string]any) {
			l["packages"].(map[string]any)["../pkg"] = l["packages"].(map[string]any)["node_modules/pkg"]
		}, "PROJECT_NONREGISTRY_DEPENDENCY"},
		{"too many nodes", func(_ map[string]any, l map[string]any) {
			packages := l["packages"].(map[string]any)
			for i := 0; i < maxProjectNodes; i++ {
				packages[fmt.Sprintf("node_modules/pkg%d", i)] = packages["node_modules/pkg"]
			}
		}, "PROJECT_GRAPH_LIMIT"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, l := projectDocuments()
			tt.change(m, l)
			dir := writeProjectDocuments(t, m, l)
			s := ReadProject(dir, "npm")
			if tt.missing == "" {
				if !s.Complete || !s.binds("pkg", "1.0.0") || s.binds("pkg", "2.0.0") || len(s.Nodes) != 1 || !s.unchanged(dir) {
					t.Fatalf("%+v", s)
				}
			} else if s.Complete || !slices.Contains(s.Missing, tt.missing) {
				t.Fatalf("%+v", s)
			}
		})
	}
}

func TestProjectFileSafety(t *testing.T) {
	dir := t.TempDir()
	if !slices.Contains(ReadProject(dir, "cargo").Missing, "PROJECT_RESOLVER_UNSUPPORTED") {
		t.Fatal("unsupported resolver")
	}
	if !slices.Contains(ReadProject(filepath.Join(dir, "absent"), "npm").Missing, "PROJECT_READ") {
		t.Fatal("unreadable project")
	}
	if !slices.Contains(ReadProject(dir, "npm").Missing, "PROJECT_MANIFEST") {
		t.Fatal("missing manifest")
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ReadProject(dir, "npm").Missing, "PROJECT_LOCKFILE") {
		t.Fatal("missing lock")
	}
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"packages":{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ReadProject(dir, "npm").Missing, "PROJECT_JSON") {
		t.Fatal("invalid JSON")
	}
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	linkDir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(linkDir, "package.json")); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ReadProject(linkDir, "npm").Missing, "PROJECT_MANIFEST") {
		t.Fatal("followed manifest symlink")
	}
	m, l := projectDocuments()
	dir = writeProjectDocuments(t, m, l)
	s := ReadProject(dir, "npm")
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if s.unchanged(dir) || s.unchanged(filepath.Join(dir, "absent")) {
		t.Fatal("changed project trusted")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := projectFile(root, "package.json", 1); err == nil {
		t.Fatal("oversized project file accepted")
	}
	if _, err := projectFile(root, "absent", 100); err == nil {
		t.Fatal("missing project file accepted")
	}
	if err := os.Mkdir(filepath.Join(dir, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := projectFile(root, "directory", 100); err == nil {
		t.Fatal("directory accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if s.unchanged(dir) {
		t.Fatal("changed lock trusted")
	}
}

func TestProjectEdges(t *testing.T) {
	m, l := projectDocuments()
	packages := l["packages"].(map[string]any)
	root := packages[""].(map[string]any)
	root["devDependencies"] = map[string]string{"dev": "1.0.0"}
	m["devDependencies"] = map[string]string{"dev": "1.0.0"}
	for _, name := range []string{"dev", "child"} {
		packages["node_modules/"+name] = map[string]any{"version": "1.0.0", "resolved": "https://registry.npmjs.org/" + name + "/-/" + name + "-1.0.0.tgz", "integrity": testIntegrity()}
	}
	pkg := packages["node_modules/pkg"].(map[string]any)
	pkg["dependencies"] = map[string]string{"child": "1.0.0"}
	pkg["peerDependencies"] = map[string]string{"absent": "1.0.0"}
	pkg["peerDependenciesMeta"] = map[string]any{"absent": map[string]bool{"optional": true}}
	s := ReadProject(writeProjectDocuments(t, m, l), "npm")
	if !s.Complete || len(s.Nodes) != 3 {
		t.Fatalf("%+v", s)
	}
	lookup := map[string]npmPackage{"node_modules/pkg": {}, "node_modules/pkg/node_modules/child": {}, "node_modules/child": {}}
	if resolveLocked(lookup, "node_modules/pkg", "child") != "node_modules/pkg/node_modules/child" || resolveLocked(lookup, "node_modules/pkg/node_modules/sub", "child") != "node_modules/pkg/node_modules/child" || resolveLocked(lookup, "", "bad/name") != "" {
		t.Fatal("incorrect nested resolution")
	}
	for _, path := range []string{"../x", "node_modules/../x", "node_modules/@scope/../x"} {
		if lockName(path) != "" {
			t.Fatal(path)
		}
	}
	if lockName("node_modules/@scope/pkg/node_modules/child") != "child" {
		t.Fatal("scoped nested path")
	}
}

func TestJSONAndArtifactValidation(t *testing.T) {
	for _, raw := range []string{`{"x":1,"x":2}`, `{"x":{"y":1,"y":2}}`, `{} {}`, `[]`, `null`, `{`, `{"x":[}`} {
		var out map[string]any
		if uniqueJSON([]byte(raw), &out) == nil {
			t.Fatal(raw)
		}
	}
	var out map[string]any
	if err := uniqueJSON([]byte(`{"x":[1,{"y":true}]}`), &out); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"http://registry.npmjs.org/pkg/-/pkg.tgz", "https://registry.npmjs.org/pkg.tgz", "https://user@registry.npmjs.org/pkg/-/pkg.tgz", "https://registry.npmjs.org/pkg/-/pkg.tgz?secret=x", ":bad"} {
		if npmArtifact(raw, testIntegrity()) {
			t.Fatal(raw)
		}
	}
	for _, hash := range []string{"sha1-x", "sha512-INVALID", "sha512-", "sha512-" + base64.StdEncoding.EncodeToString([]byte("short")), testIntegrity() + "\n"} {
		if npmArtifact("https://registry.npmjs.org/pkg/-/pkg.tgz", hash) {
			t.Fatal(hash)
		}
	}
}

func TestNPMRanges(t *testing.T) {
	for _, tt := range []struct {
		req, version string
		match, known bool
	}{
		{"1.2.3", "1.2.3", true, true}, {"=1.2.3", "1.2.4", false, true}, {"*", "1.0.0", true, true},
		{"^1.2.3", "1.9.0", true, true}, {"^1.2.3", "2.0.0", false, true}, {"^0.2.3", "0.3.0", false, true}, {"^0.0.3", "0.0.4", false, true},
		{"~1.2.3", "1.2.9", true, true}, {"~1.2.3", "1.3.0", false, true}, {">=1.0.0 <2.0.0", "1.2.3", true, true},
		{">1.0.0 <=2.0.0", "2.0.0", true, true}, {">1.0.0 <=2.0.0", "1.0.0", false, true},
		{"^1", "1.2.3", false, false}, {"1.x", "1.2.3", false, false}, {"1.0.0 || 2.0.0", "1.0.0", false, false}, {"", "1.0.0", false, false},
		{"1.0.0", "1.0.0-rc1", false, false}, {"^18446744073709551615.0.0", "1.0.0", false, false},
	} {
		match, known := npmSatisfies(tt.req, tt.version)
		if match != tt.match || known != tt.known {
			t.Fatal(tt, match, known)
		}
	}
	if compareNumbers([]uint64{2, 0}, []uint64{1, 9}) != 1 {
		t.Fatal("compare")
	}
}

func TestLeafAdditionPlan(t *testing.T) {
	m, l := projectDocuments()
	delete(m, "dependencies")
	packages := l["packages"].(map[string]any)
	delete(packages[""].(map[string]any), "dependencies")
	delete(packages, "node_modules/pkg")
	dir := writeProjectDocuments(t, m, l)
	snapshot := ReadProject(dir, "npm")
	release := NPMRelease{Name: "leaf", Version: "1.0.0"}
	release.Dist.Tarball = "https://registry.npmjs.org/leaf/-/leaf-1.0.0.tgz"
	release.Dist.Integrity = testIntegrity()
	if !snapshot.planLeaf(&release) || len(snapshot.Plans) != 1 || !snapshot.unchanged(dir) {
		t.Fatal(snapshot)
	}
	for _, change := range []func(*NPMRelease){
		func(r *NPMRelease) { r.Name = "../bad" }, func(r *NPMRelease) { r.Version = "bad" },
		func(r *NPMRelease) { r.Dependencies = map[string]string{"child": "1.0.0"} }, func(r *NPMRelease) { r.OptionalDependencies = map[string]string{"child": "1.0.0"} },
		func(r *NPMRelease) { r.PeerDependencies = map[string]string{"peer": "1.0.0"} }, func(r *NPMRelease) { r.BundleDependencies = true }, func(r *NPMRelease) { r.Dist.Integrity = "bad" },
		func(r *NPMRelease) { r.HasShrinkwrap = true },
	} {
		r := release
		change(&r)
		if snapshot.planLeaf(&r) {
			t.Fatal("unsupported addition accepted", r)
		}
	}
	if snapshot.planLeaf(nil) || (*ProjectSnapshot)(nil).planLeaf(&release) {
		t.Fatal("missing plan accepted")
	}
	snapshot.packages["node_modules/leaf"] = npmPackage{Version: "1.0.0"}
	if snapshot.planLeaf(&release) {
		t.Fatal("existing package replaced")
	}
	delete(snapshot.packages, "node_modules/leaf")
	snapshot.packages[""] = npmPackage{PeerDependencies: map[string]string{"leaf": "^2.0.0"}}
	if snapshot.planLeaf(&release) {
		t.Fatal("unsatisfied peer activated")
	}
	snapshot.packages[""] = npmPackage{PeerDependencies: map[string]string{"leaf": "1.x"}}
	if snapshot.planLeaf(&release) {
		t.Fatal("unknown peer constraint accepted")
	}
	snapshot.packages[""] = npmPackage{PeerDependencies: map[string]string{"leaf": "^1.0.0"}}
	if !snapshot.planLeaf(&release) {
		t.Fatal("satisfied peer rejected")
	}
}

func FuzzProjectJSON(f *testing.F) {
	f.Add(`{"packages":{"":{}}}`)
	f.Add(`{"x":1,"x":2}`)
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 1<<16 {
			t.Skip()
		}
		var out map[string]any
		_ = uniqueJSON([]byte(raw), &out)
	})
}

func FuzzNPMRange(f *testing.F) {
	f.Add("^1.2.3", "1.2.4")
	f.Add("*", "1.0.0")
	f.Fuzz(func(t *testing.T, requirement, version string) {
		if len(requirement) > 512 || len(version) > 128 {
			t.Skip()
		}
		match, known := npmSatisfies(requirement, version)
		if match && !known {
			t.Fatal("unknown constraint approved")
		}
		if strings.Contains(requirement, "||") && known {
			t.Fatal("unsupported OR accepted")
		}
	})
}
