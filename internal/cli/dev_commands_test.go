package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHelpIncludesDevCommands(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"--help"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, res.ExitCode)
	}
	text := out.String()
	for _, cmd := range []string{"gh flarebyte test", "gh flarebyte format", "gh flarebyte lint", "gh flarebyte cov"} {
		if !strings.Contains(text, cmd) {
			t.Fatalf("expected help to include %q, got: %s", cmd, text)
		}
	}
}

func TestRunCovRejectsInvalidMin(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov", "--min", "abc"}, &out, &errOut)
	if res.ExitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, res.ExitCode)
	}
	if !strings.Contains(errOut.String(), "--min") {
		t.Fatalf("expected --min error, got: %s", errOut.String())
	}
}

func TestRunTestGoUsesConfiguredCaches(t *testing.T) {
	cfg := testConfigCue() + `

go: {
	cache_dir: "./.gocache"
	mod_cache_dir: "./.gomodcache"
	toolchain: "local"
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	var captured []string
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		captured = env
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d err=%v stderr=%s", ExitOK, res.ExitCode, res.Err, errOut.String())
	}
	want := []string{"GOCACHE=./.gocache", "GOMODCACHE=./.gomodcache", "GOTOOLCHAIN=local"}
	for _, entry := range want {
		if !contains(captured, entry) {
			t.Fatalf("expected env to include %q", entry)
		}
	}
}

func TestRunCovFailsBelowThreshold(t *testing.T) {
	cfg := testConfigCue() + `

coverage: {
	default_min_percent: 90
	fail_below_min: true
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "cover" {
			return "total: (statements) 75.0%\n", "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov"}, &out, &errOut)
	if res.ExitCode != ExitFailure {
		t.Fatalf("expected exit code %d, got %d", ExitFailure, res.ExitCode)
	}
	if !strings.Contains(errOut.String(), "COV FAIL") {
		t.Fatalf("expected coverage failure output, got: %s", errOut.String())
	}
}

func TestRunCovMinFlagOverridesConfigThreshold(t *testing.T) {
	cfg := testConfigCue() + `

coverage: {
	default_min_percent: 90
	fail_below_min: true
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "cover" {
			return "total: (statements) 75.0%\n", "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov", "--min", "70"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d stderr=%s", ExitOK, res.ExitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "min=70.00%") {
		t.Fatalf("expected CLI min in output, got: %s", out.String())
	}
}

func TestRunDartRoutingForLint(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	var invoked string
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		invoked = fmt.Sprintf("%s %s", name, strings.Join(args, " "))
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"lint"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d stderr=%s", ExitOK, res.ExitCode, errOut.String())
	}
	if invoked != "dart analyze" {
		t.Fatalf("expected dart analyze invocation, got: %s", invoked)
	}
}

func TestDiscoverGoFilesSkipsExcludedDirs(t *testing.T) {
	tmp := t.TempDir()
	mustWrite := func(rel string) {
		t.Helper()
		path := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := os.WriteFile(path, []byte("package x\n"), 0o644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}
	mustWrite("a/main.go")
	mustWrite("vendor/v.go")
	mustWrite("build/b.go")
	mustWrite(".gocache/c.go")
	files, err := discoverGoFiles(tmp)
	if err != nil {
		t.Fatalf("discoverGoFiles failed: %v", err)
	}
	if len(files) != 1 || !strings.Contains(files[0], "a/main.go") {
		t.Fatalf("unexpected files: %v", files)
	}
}

func TestRunTestOneLineStyleOutput(t *testing.T) {
	cfg := testConfigCue() + `

dev_output: {
	color: "false"
	style: "one_line"
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		return "{\"Action\":\"pass\"}\n{\"Action\":\"skip\"}\n", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, res.ExitCode)
	}
	if !strings.Contains(out.String(), "PASS kind=test") || !strings.Contains(out.String(), "tests=2 failed=0 skipped=1") {
		t.Fatalf("expected one_line output, got: %s", out.String())
	}
}

func TestRunTestHidePassedOutput(t *testing.T) {
	cfg := testConfigCue() + `

dev_output: {
	color: "false"
	style: "summary"
	show_passed: false
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		return "{\"Action\":\"pass\"}\n", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, res.ExitCode)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected no pass output when show_passed=false, got: %s", out.String())
	}
}
