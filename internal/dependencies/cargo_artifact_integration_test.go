package dependencies

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Cargo generates this archive from controlled source only. --no-verify and
// offline mode are mandatory; neither the build script nor proc macro may run.
func TestCargoArchiveOracle(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_CARGO_INTEGRATION") != "1" {
		t.Skip("opt-in offline Cargo archive oracle")
	}
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Fatal(err)
	}
	cargo, err = filepath.EvalSymlinks(cargo)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := filepath.Rel(repo, cargo); err != nil || filepath.IsLocal(rel) {
		t.Fatal("repository-local Cargo refused")
	}
	dir := t.TempDir()
	crate := filepath.Join(dir, "crate")
	if err := os.MkdirAll(filepath.Join(crate, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := `[package]
name = "archive-fixture"
version = "1.0.0"
edition = "2021"
rust-version = "1.85"
include = ["Cargo.toml", "src/lib.rs", "build.rs"]
[lib]
proc-macro = true
[features]
default = ["std"]
std = []
`
	for name, data := range map[string]string{"Cargo.toml": manifest, "src/lib.rs": "extern crate proc_macro;\n", "build.rs": `fn main() { std::fs::write("hook-executed", "unexpected").unwrap(); }`} {
		if err := os.WriteFile(filepath.Join(crate, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cargo, "package", "--offline", "--no-verify", "--allow-dirty", "--manifest-path", filepath.Join(crate, "Cargo.toml"))
	cmd.Dir = crate
	cmd.Env = []string{"PATH=" + filepath.Dir(cargo) + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin", "CARGO_HOME=" + filepath.Join(dir, "cache"), "HOME=" + dir, "TMPDIR=" + dir, "RUSTC_WRAPPER=", "RUSTC_WORKSPACE_WRAPPER=", "CARGO_NET_OFFLINE=true", "GIT_CEILING_DIRECTORIES=" + dir}
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Cargo package: %v: %s", err, stderr.String())
	}
	b, err := os.ReadFile(filepath.Join(crate, "target", "package", "archive-fixture-1.0.0.crate"))
	if err != nil {
		t.Fatal(err)
	}
	a := sealedCrate(ProjectNode{Name: "archive-fixture", Version: "1.0.0"}, &cargoFeaturePackage{complete: true, rustVersion: "1.85", edition: "2021", features: map[string][]string{"default": {"std"}, "std": {}}}, b)
	info, err := VerifyCargoArtifact(b, a)
	if err != nil || !info.BuildScript || !info.ProcMacro || info.Files < 4 {
		t.Fatal(info, err)
	}
	if _, err := os.Stat(filepath.Join(crate, "hook-executed")); !os.IsNotExist(err) {
		t.Fatal("Cargo package executed a fixture build script")
	}
}
