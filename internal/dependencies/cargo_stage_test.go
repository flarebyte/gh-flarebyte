package dependencies

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func privateCargoStageDir(t *testing.T) string {
	t.Helper()
	stage, err := os.MkdirTemp(t.TempDir(), "stage-")
	if err != nil {
		t.Fatal(err)
	}
	return stage
}

func preparedCargoStage(t *testing.T) (string, *CargoStage) {
	t.Helper()
	dir, r, blobs := approvedCrateArtifacts(t, "1.85")
	s, err := PrepareCargoStage(dir, privateCargoStageDir(t), r)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range s.Artifacts() {
		if err := s.AddArtifact(blobs[a.Node.Name], a); err != nil {
			t.Fatal(err)
		}
	}
	return dir, s
}

func cargoStageMetadata(t *testing.T, s *CargoStage) map[string]any {
	t.Helper()
	packages, nodes := []any{}, []any{}
	for path, features := range s.features {
		p := s.original.cargo.packages[path]
		var source any = cargoRegistry
		manifest := filepath.Join(s.dir, "vendor", p.Name+"-"+p.Version, "Cargo.toml")
		if path == s.original.cargo.root {
			source = nil
			manifest = filepath.Join(s.dir, "project", "Cargo.toml")
		}
		packages = append(packages, map[string]any{"id": path, "name": p.Name, "version": p.Version, "source": source, "manifest_path": manifest})
		deps := []string{}
		for _, edge := range s.original.cargo.edges[path] {
			if _, ok := s.features[edge]; ok {
				deps = append(deps, edge)
			}
		}
		nodes = append(nodes, map[string]any{"id": path, "dependencies": deps, "features": features})
	}
	return map[string]any{"packages": packages, "workspace_members": []string{s.original.cargo.root}, "resolve": map[string]any{"root": s.original.cargo.root, "nodes": nodes}}
}

func TestCargoStageRoundTrip(t *testing.T) {
	dir, s := preparedCargoStage(t)
	data, err := json.Marshal(cargoStageMetadata(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyMetadata(data); err != nil {
		t.Fatal(err)
	}
	if !s.Unchanged(dir) {
		t.Fatal("staging changed source inputs")
	}
	for _, a := range s.Artifacts() {
		b, err := os.ReadFile(filepath.Join(s.dir, "vendor", a.Node.Name+"-"+a.Node.Version, ".cargo-checksum.json"))
		if err != nil {
			t.Fatal(err)
		}
		var checksums struct {
			Package string
			Files   map[string]string
		}
		if json.Unmarshal(b, &checksums) != nil || "sha256-"+checksums.Package != a.Node.Integrity || len(checksums.Files) != 2 {
			t.Fatal("invalid vendor checksums", string(b))
		}
		info, err := os.Stat(filepath.Join(s.dir, "vendor", a.Node.Name+"-"+a.Node.Version, "Cargo.toml"))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("archive permissions were inherited")
		}
	}
	if err := s.ValidateSDK("1.99.1", "1.99.1"); err != nil {
		t.Fatal(err)
	}
	for _, versions := range [][2]string{{"1.84.0", "1.99.0"}, {"1.99.0", "1.84.0"}, {"1.99.0-nightly", "1.99.0"}, {"1.99.0", "bad"}} {
		if s.ValidateSDK(versions[0], versions[1]) == nil {
			t.Fatal("invalid SDK accepted", versions)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if s.Unchanged(dir) {
		t.Fatal("source edit ignored")
	}
}

func TestCargoStageDisagreements(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"missing resolve", func(m map[string]any) { delete(m, "resolve") }},
		{"extra package", func(m map[string]any) { m["packages"] = append(m["packages"].([]any), m["packages"].([]any)[0]) }},
		{"duplicate node", func(m map[string]any) { ns := m["resolve"].(map[string]any)["nodes"].([]any); ns[1] = ns[0] }},
		{"wrong root", func(m map[string]any) { m["resolve"].(map[string]any)["root"] = "other" }},
		{"workspace", func(m map[string]any) { m["workspace_members"] = []string{"other"} }},
		{"feature", func(m map[string]any) {
			m["resolve"].(map[string]any)["nodes"].([]any)[0].(map[string]any)["features"] = []string{"unexpected"}
		}},
		{"edge", func(m map[string]any) {
			m["resolve"].(map[string]any)["nodes"].([]any)[0].(map[string]any)["dependencies"] = []string{"missing"}
		}},
		{"missing edge", func(m map[string]any) {
			for _, n := range m["resolve"].(map[string]any)["nodes"].([]any) {
				n.(map[string]any)["dependencies"] = []string{}
			}
		}},
		{"source", func(m map[string]any) {
			m["packages"].([]any)[0].(map[string]any)["source"] = "git+https://evil.test/repo"
		}},
		{"manifest", func(m map[string]any) {
			m["packages"].([]any)[0].(map[string]any)["manifest_path"] = "/outside/Cargo.toml"
		}},
		{"duplicate identity", func(m map[string]any) { ps := m["packages"].([]any); ps[1] = ps[0] }},
		{"unknown identity", func(m map[string]any) { m["packages"].([]any)[0].(map[string]any)["version"] = "2.0.0" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, s := preparedCargoStage(t)
			m := cargoStageMetadata(t, s)
			test.change(m)
			b, _ := json.Marshal(m)
			if s.VerifyMetadata(b) == nil {
				t.Fatal("metadata discrepancy accepted")
			}
		})
	}
	_, s := preparedCargoStage(t)
	for _, b := range [][]byte{[]byte(`{"resolve":{},"resolve":{}}`), []byte("bad"), make([]byte, maxBody+1)} {
		if s.VerifyMetadata(b) == nil {
			t.Fatal("malformed metadata accepted")
		}
	}
	b, _ := json.Marshal(cargoStageMetadata(t, s))
	if err := os.WriteFile(filepath.Join(s.dir, "project", "Cargo.lock"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if s.VerifyMetadata(b) == nil {
		t.Fatal("changed staged lock ignored")
	}
}

func TestCargoStageBoundaries(t *testing.T) {
	dir, r, blobs := approvedCrateArtifacts(t)
	inside := filepath.Join(dir, "stage")
	if err := os.Mkdir(inside, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareCargoStage(dir, inside, r); err == nil {
		t.Fatal("repository-local stage accepted")
	}
	stage := t.TempDir()
	if err := os.Chmod(stage, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareCargoStage(dir, stage, r); err == nil {
		t.Fatal("public stage accepted")
	}
	if err := os.Chmod(stage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "existing"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareCargoStage(dir, stage, r); err == nil {
		t.Fatal("nonempty stage accepted")
	}
	s, err := PrepareCargoStage(dir, privateCargoStageDir(t), r)
	if err != nil {
		t.Fatal(err)
	}
	if s.VerifyMetadata(nil) == nil {
		t.Fatal("incomplete stage accepted")
	}
	if s.ValidateSDK("1.99.0", "1.99.0") == nil {
		t.Fatal("undeclared baseline accepted")
	}
	a := s.Artifacts()[0]
	bad := a
	bad.Node.Name = "other"
	if s.AddArtifact(blobs[a.Node.Name], bad) == nil {
		t.Fatal("mutated descriptor accepted")
	}
	if s.AddArtifact([]byte("corrupt"), a) == nil {
		t.Fatal("corrupted crate accepted")
	}
	if err := s.AddArtifact(blobs[a.Node.Name], a); err != nil {
		t.Fatal(err)
	}
	if s.AddArtifact(blobs[a.Node.Name], a) == nil {
		t.Fatal("duplicate extraction accepted")
	}
	for _, budget := range []string{"compressed", "expanded"} {
		s, err := PrepareCargoStage(dir, privateCargoStageDir(t), r)
		if err != nil {
			t.Fatal(err)
		}
		if budget == "compressed" {
			s.compressed = 100 << 20
		} else {
			s.expanded = 300 << 20
		}
		a := s.Artifacts()[0]
		if s.AddArtifact(blobs[a.Node.Name], a) == nil {
			t.Fatal("artifact budget ignored")
		}
	}
	s, err = PrepareCargoStage(dir, privateCargoStageDir(t), r)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(s.dir, "vendor")); err != nil {
		t.Fatal(err)
	}
	a = s.Artifacts()[0]
	if s.AddArtifact(blobs[a.Node.Name], a) == nil {
		t.Fatal("symlink directory accepted")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatal("wrote outside stage")
	}
}
