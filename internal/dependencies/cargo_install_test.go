package dependencies

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCargoPinPreservesManifest(t *testing.T) {
	for _, declaration := range []string{
		"[dependencies]\npkg = '1.0' # retain comment\n",
		"[dependencies]\npkg = { version = '1.0', default-features = false } # options\n",
		"[dependencies.pkg]\nversion = '1.0' # table\nfeatures = []\n",
		"dependencies.pkg.version = '1.0' # dotted\n",
		"dependencies = { 'pkg' = { version = '1.0', features = [] } }\n",
	} {
		t.Run(declaration, func(t *testing.T) {
			// Root dotted/inline declarations must precede the package table.
			data := declaration + "[package]\nname='local'\nversion='1.0.0'\n"
			if strings.HasPrefix(declaration, "[") {
				data = "[package]\nname='local'\nversion='1.0.0'\n" + declaration
			}
			b, err := pinCargoRequirement([]byte(data), "pkg", "1.0.0")
			if err != nil || string(b) != strings.Replace(data, "'1.0'", `"=1.0.0"`, 1) {
				t.Fatal(string(b), err)
			}
		})
	}
	for _, data := range []string{"bad", "[dependencies]\nother='1'\n", "[dependencies]\npkg={features=[]}\n"} {
		if _, err := pinCargoRequirement([]byte(data), "pkg", "1.0.0"); err == nil {
			t.Fatal("unpinnable manifest accepted")
		}
	}
}

func installCargoSource(t *testing.T, dir string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(dir, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "lib.rs"), []byte("// source stays untouched\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCargoInstallPlan(t *testing.T) {
	dir, r, blobs := approvedCrateArtifacts(t, "1.85")
	installCargoSource(t, dir)
	p, err := PrepareCargoInstall(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(p.Manifest), `pkg = "=1.0.0"`) || !p.Unchanged(dir) {
		t.Fatal("missing exact pin", string(p.Manifest))
	}
	s, err := PrepareCargoStage(dir, privateCargoStageDir(t), r)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UseInstallPlan(p); err != nil {
		t.Fatal(err)
	}
	for _, a := range s.Artifacts() {
		if err := s.AddArtifact(blobs[a.Node.Name], a); err != nil {
			t.Fatal(err)
		}
	}
	m := cargoStageMetadata(t, s)
	for _, p := range m["packages"].([]any) {
		pkg := p.(map[string]any)
		if pkg["id"] == s.original.cargo.root {
			pkg["targets"] = []any{map[string]any{"name": "local", "kind": []string{"lib"}, "crate_types": []string{"lib"}, "edition": "2021", "src_path": filepath.Join(s.dir, "project", "src", "lib.rs")}}
		}
	}
	b, _ := json.Marshal(m)
	if err := s.VerifyMetadata(b); err != nil {
		t.Fatal(err)
	}
	for _, pkg := range m["packages"].([]any) {
		entry := pkg.(map[string]any)
		if entry["id"] == s.original.cargo.root {
			entry["targets"].([]any)[0].(map[string]any)["kind"] = []string{"proc-macro"}
		}
	}
	b, _ = json.Marshal(m)
	if s.VerifyMetadata(b) == nil {
		t.Fatal("changed root target accepted")
	}
	p.Manifest = append(p.Manifest, ' ')
	other, err := PrepareCargoStage(dir, privateCargoStageDir(t), r)
	if err != nil {
		t.Fatal(err)
	}
	if other.UseInstallPlan(p) == nil {
		t.Fatal("changed pin accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.rs"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if p.ContextUnchanged(dir) {
		t.Fatal("new source target ignored")
	}
}

func TestCargoInstallContext(t *testing.T) {
	base := "[package]\nname='local'\nversion='1.0.0'\n"
	for _, test := range []struct{ name, extra, path string }{
		{"custom lib", "[lib]\npath='src/lib.rs'\n", ""},
		{"build", "build='custom.rs'\n", ""},
		{"links", "links='native'\n", ""},
		{"autobins", "autobins=false\n", ""},
		{"resolver", "resolver='future'\n", ""},
		{"readme escape", "readme='../secret'\n", ""},
		{"dev deps", "[dev-dependencies]\nother='1'\n", ""},
		{"build deps", "[build-dependencies]\nother='1'\n", ""},
		{"build script", "", "build.rs"},
		{"extra targets", "", "src/bin"},
		{"ancestor config", "", ".cargo/config.toml"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			installCargoSource(t, dir)
			if test.path != "" {
				path := filepath.Join(dir, test.path)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := cargoInstallContext(dir, []byte(base+test.extra)); err == nil {
				t.Fatal("unsupported context accepted")
			}
		})
	}
	dir := t.TempDir()
	installCargoSource(t, dir)
	if err := os.Remove(filepath.Join(dir, "src", "lib.rs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside.rs"), filepath.Join(dir, "src", "lib.rs")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cargoInstallContext(dir, []byte(base)); err == nil {
		t.Fatal("symlink source accepted")
	}
	dir = t.TempDir()
	installCargoSource(t, dir)
	if _, _, err := cargoInstallContext(dir, []byte(base+"build=false\nreadme=false\nresolver='2'\n")); err != nil {
		t.Fatal(err)
	}
}

func TestCargoInstallRejectsExecutableContexts(t *testing.T) {
	dir, r, blobs := approvedCrateArtifacts(t, "1.85", "hooks")
	installCargoSource(t, dir)
	p, err := PrepareCargoInstall(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	s, err := PrepareCargoStage(dir, privateCargoStageDir(t), r)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UseInstallPlan(p); err != nil {
		t.Fatal(err)
	}
	a := s.Artifacts()[0]
	if s.AddArtifact(blobs[a.Node.Name], a) == nil {
		t.Fatal("build/proc-macro context accepted for install")
	}
}

func FuzzCargoRequirementPin(f *testing.F) {
	for _, seed := range []string{"[dependencies]\npkg='1'\n", "dependencies={pkg={version='1',features=[]}}\n", "[dependencies.pkg]\nversion=\"\"\"1\"\"\"\n", "[dependencies]\npkg={version='1',package='renamed'}\n", "bad"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		b, err := pinCargoRequirement(data, "pkg", "1.0.0")
		if err == nil && !strings.Contains(string(b), `"=1.0.0"`) {
			t.Fatal("successful pin omitted exact version")
		}
	})
}
