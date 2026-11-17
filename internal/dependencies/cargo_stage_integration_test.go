package dependencies

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCargoStageOracle(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_CARGO_INTEGRATION") != "1" {
		t.Skip("opt-in offline Cargo staged resolution oracle")
	}
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Fatal(err)
	}
	cargo, err = filepath.EvalSymlinks(cargo)
	if err != nil {
		t.Fatal(err)
	}
	rustc, err := exec.LookPath("rustc")
	if err != nil {
		t.Fatal(err)
	}
	rustc, err = filepath.EvalSymlinks(rustc)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(cargo) != "cargo" || filepath.Base(rustc) != "rustc" {
		t.Fatal("standalone SDK required")
	}
	dir, report, blobs := approvedCrateArtifacts(t, "1.85", "hooks")
	s, err := PrepareCargoStage(dir, privateCargoStageDir(t), report)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range s.Artifacts() {
		if err := s.AddArtifact(blobs[a.Node.Name], a); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, name := range []string{"user", "tmp"} {
		if err := os.Mkdir(filepath.Join(s.dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{"PATH=" + filepath.Dir(rustc) + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin", "HOME=" + filepath.Join(s.dir, "user"), "CARGO_HOME=" + filepath.Join(s.dir, "home"), "CARGO_NET_OFFLINE=true", "TMPDIR=" + filepath.Join(s.dir, "tmp"), "RUSTC=" + rustc, "RUSTC_WRAPPER=", "RUSTC_WORKSPACE_WRAPPER=", "CARGO_TARGET_DIR=" + filepath.Join(s.dir, "target")}
	run := func(tool string, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, tool, args...)
		cmd.Dir = filepath.Join(s.dir, "project")
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("isolated SDK fixture: %v: %s", err, stderr.String())
		}
		return stdout.Bytes()
	}
	c, r := strings.Fields(string(run(cargo, "--version"))), strings.Fields(string(run(rustc, "--version")))
	if len(c) < 2 || len(r) < 2 {
		t.Fatal("invalid SDK output")
	}
	if err := s.ValidateSDK(c[1], r[1]); err != nil {
		t.Fatal(err)
	}
	metadata := run(cargo, "metadata", "--offline", "--locked", "--format-version", "1")
	if err := s.VerifyMetadata(metadata); err != nil {
		t.Fatalf("staged Cargo disagrees: %v\n%s", err, metadata)
	}
	if !s.Unchanged(dir) {
		t.Fatal("source project changed")
	}
	if err := filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "hook-executed" {
			t.Fatal("metadata executed a build script or proc macro")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "target")); !os.IsNotExist(err) {
		t.Fatal("metadata produced build outputs")
	}
}
