package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

func TestIsolatedDependencySDK(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	tools := t.TempDir()
	log := filepath.Join(tools, "arguments")
	script := "#!/bin/sh\n[ -z \"$GH_TOKEN$GITHUB_TOKEN$NODE_OPTIONS$NPM_CONFIG_IGNORE_SCRIPTS\" ] || exit 9\nif [ \"$1\" = --version ]; then printf 'v24.0.0'; exit 0; fi\nif [ \"$2\" = --version ]; then printf '11.0.0'; exit 0; fi\nprintf '%s\\n' \"$@\" > '" + log + "'\n"
	for name, body := range map[string]string{"node": script, "npm": "fixture"} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", tools)
	t.Setenv("GH_TOKEN", "must-not-leak")
	t.Setenv("GITHUB_TOKEN", "must-not-leak")
	t.Setenv("NODE_OPTIONS", "must-not-load")
	t.Setenv("NPM_CONFIG_IGNORE_SCRIPTS", "false")
	runner, err := prepareDependencyNPM(root, stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner(context.Background(), []string{"ci"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--ignore-scripts", "--offline", "--no-audit", "--no-fund", "--userconfig=", "--globalconfig=", "--registry=https://registry.npmjs.org", "--cache="} {
		if !strings.Contains(string(b), flag) {
			t.Fatal("missing safety flag", flag, string(b))
		}
	}
	if err := runner(context.Background(), nil); err == nil {
		t.Fatal("empty command accepted")
	}
	if err := runner(context.Background(), []string{"cache", "add", "fixture.tgz"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tools, "node"), []byte("#!/bin/sh\nprintf 'v18.0.0'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	runner, err = prepareDependencyNPM(root, stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner(context.Background(), []string{"ci"}); !errors.Is(err, dependencies.ErrInstallUnsupported) {
		t.Fatal(err)
	}
	if nativeMajor("bad") != 0 || nativeMajor("v24.1.0") != 24 {
		t.Fatal("SDK version parsing")
	}
	var out dependencyOutput
	if _, err := out.Write(make([]byte, (64<<10)+1)); err == nil {
		t.Fatal("unbounded native output")
	}
}

func TestDependencyToolBoundary(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "node"), []byte("#!/bin/sh\nexit 9"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	if _, err := trustedDependencyTool(root, "node"); err == nil {
		t.Fatal("repository tool accepted")
	}
	if _, err := trustedDependencyTool(root, "absent"); !errors.Is(err, dependencies.ErrInstallUnsupported) {
		t.Fatal(err)
	}
	external := t.TempDir()
	tool := filepath.Join(external, "node")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", external)
	if err := os.Chmod(tool, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := trustedDependencyTool(root, "node"); err == nil {
		t.Fatal("world-writable executable accepted")
	}
	if err := os.Chmod(tool, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(external, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := trustedDependencyTool(root, "node"); err == nil {
		t.Fatal("world-writable SDK directory accepted")
	}
}

func TestInstalledTreeRejectsLinks(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if validateInstalledTree(dir) == nil {
		t.Fatal("unsafe installed link accepted")
	}
	if _, err := dependencyTreeDigest(dir); err == nil {
		t.Fatal("unsafe tree fingerprinted")
	}
	if validateInstalledTree(filepath.Join(dir, "absent")) == nil {
		t.Fatal("missing installed tree accepted")
	}
}
