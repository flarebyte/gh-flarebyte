// purpose: Validate test command behavior for style/color overrides, filtering, and failure reporting.
// responsibilities: Cover arg validation, env wiring, per_test rendering, showPassed behavior, and failure snippet propagation.
// architecture notes: Tests use captured command output stubs so assertions stay stable independent of real `go test` execution.
package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTestGoUsesConfiguredCaches(t *testing.T) {
	cfg := testConfigCue() + `

go: {
	cacheDir: "./.gocache"
	modCacheDir: "./.gomodcache"
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
	cacheAbs, err := filepath.Abs("./.gocache")
	if err != nil {
		t.Fatalf("abs cache path: %v", err)
	}
	modCacheAbs, err := filepath.Abs("./.gomodcache")
	if err != nil {
		t.Fatalf("abs mod cache path: %v", err)
	}
	want := []string{"GOCACHE=" + cacheAbs, "GOMODCACHE=" + modCacheAbs, "GOTOOLCHAIN=local"}
	for _, entry := range want {
		if !contains(captured, entry) {
			t.Fatalf("expected env to include %q", entry)
		}
	}
	if strings.Contains(errOut.String(), "warning: config.go.cacheDir is absolute") || strings.Contains(errOut.String(), "warning: config.go.modCacheDir is absolute") {
		t.Fatalf("expected no absolute-path warning for relative config, got: %s", errOut.String())
	}
}

func TestRunTestGoWarnsWhenCachesAreAbsolute(t *testing.T) {
	cfg := testConfigCue() + `

go: {
	cacheDir: "/tmp/codex-abs-gocache"
	modCacheDir: "/tmp/codex-abs-gomodcache"
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
	want := []string{"GOCACHE=/tmp/codex-abs-gocache", "GOMODCACHE=/tmp/codex-abs-gomodcache", "GOTOOLCHAIN=local"}
	for _, entry := range want {
		if !contains(captured, entry) {
			t.Fatalf("expected env to include %q", entry)
		}
	}
	if !strings.Contains(errOut.String(), "warning: config.go.cacheDir is absolute (/tmp/codex-abs-gocache)") {
		t.Fatalf("expected cacheDir absolute warning, got: %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "warning: config.go.modCacheDir is absolute (/tmp/codex-abs-gomodcache)") {
		t.Fatalf("expected modCacheDir absolute warning, got: %s", errOut.String())
	}
}

func TestRunTestPerTestStyleOutput(t *testing.T) {
	cfg := perTestStyleConfig()
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		return "{\"Action\":\"pass\",\"Test\":\"TestOne\"}\n{\"Action\":\"skip\",\"Test\":\"TestTwo\"}\n", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, res.ExitCode)
	}
	if !strings.Contains(out.String(), "✓ TestOne") || !strings.Contains(out.String(), "↷ TestTwo") || !strings.Contains(out.String(), "TEST PASS") {
		t.Fatalf("expected per_test output, got: %s", out.String())
	}
}

func TestRunTestHidePassedOutput(t *testing.T) {
	cfg := testConfigCue() + `

devOutput: {
	color: "false"
	style: "summary"
	showPassed: false
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
		t.Fatalf("expected no pass output when showPassed=false, got: %s", out.String())
	}
}

func TestDevTestCommandHelpAndUsage(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	if res := Run([]string{"test", "-h"}, &out, &errOut); res.ExitCode != ExitOK || !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("unexpected test help output: code=%d out=%s", res.ExitCode, out.String())
	}
	if !strings.Contains(out.String(), "--style") {
		t.Fatalf("expected style flag in test help, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "--color") {
		t.Fatalf("expected color flag in test help, got: %s", out.String())
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"test", "--bad"}, &out, &errOut); res.ExitCode != ExitUsage {
		t.Fatalf("expected usage error for test, got %d", res.ExitCode)
	}
}

func TestRunTestStyleOverridePerTest(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		return "{\"Action\":\"pass\",\"Test\":\"TestOne\"}\n", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test", "--style", "per_test"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "✓ TestOne") {
		t.Fatalf("expected per-test marker output, got: %s", out.String())
	}
}

func TestRunTestFailedOnlyFiltersPassedAndSkipped(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		return strings.Join([]string{
			`{"Action":"pass","Test":"TestOne"}`,
			`{"Action":"skip","Test":"TestTwo"}`,
			`{"Action":"output","Test":"TestThree","Output":"pkg/x_test.go:10: expected 1, got 2"}`,
			`{"Action":"fail","Test":"TestThree"}`,
			"",
		}, "\n"), "stderr detail", fmt.Errorf("boom")
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test", "--style", "per_test", "--failed-only"}, &out, &errOut)
	if res.ExitCode != ExitFailure {
		t.Fatalf("expected failure, got %d", res.ExitCode)
	}
	if strings.Contains(out.String(), "✓ TestOne") || strings.Contains(out.String(), "↷ TestTwo") {
		t.Fatalf("expected passed/skipped tests hidden, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "✗ TestThree") {
		t.Fatalf("expected failed test line, got: %s", out.String())
	}
	if !strings.Contains(errOut.String(), "FAILED:") {
		t.Fatalf("expected failed details, got: %s", errOut.String())
	}
}

func TestRunTestStyleOverrideRejectsInvalidValue(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test", "--style", "bad"}, &out, &errOut)
	if res.ExitCode != ExitUsage {
		t.Fatalf("expected usage error, got %d", res.ExitCode)
	}
	if !strings.Contains(errOut.String(), "--style") {
		t.Fatalf("expected style error, got: %s", errOut.String())
	}
}

func TestRunTestColorOverrideRejectsInvalidValue(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test", "--color", "bad"}, &out, &errOut)
	if res.ExitCode != ExitUsage {
		t.Fatalf("expected usage error, got %d", res.ExitCode)
	}
	if !strings.Contains(errOut.String(), "--color") {
		t.Fatalf("expected color error, got: %s", errOut.String())
	}
}

func TestRunGoTestFailureIncludesSummaryDetails(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "test" && args[1] == "-json" {
			out := "{\"Action\":\"pass\",\"Test\":\"TestOne\"}\n{\"Action\":\"output\",\"Test\":\"TestTwo\",\"Output\":\"internal/config/parse_test.go:87: expected version\"}\n{\"Action\":\"fail\",\"Test\":\"TestTwo\"}\n"
			return out, "stderr detail", fmt.Errorf("boom")
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test"}, &out, &errOut)
	if res.ExitCode != ExitFailure {
		t.Fatalf("expected go test failure, got %d", res.ExitCode)
	}
	if !strings.Contains(errOut.String(), "tests=2 failed=1 skipped=0") {
		t.Fatalf("expected summarized failure counts, got: %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "FAILED:") || !strings.Contains(errOut.String(), "snippet: internal/config/parse_test.go:87: expected version") {
		t.Fatalf("expected failure snippets, got: %s", errOut.String())
	}
}

func TestRunDartTestPerTestStyleOutput(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	cfg += `

devOutput: {
	color: "false"
	style: "per_test"
	showPassed: true
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" {
			if len(args) < 3 || args[0] != "test" || args[1] != "-r" || args[2] != "json" {
				t.Fatalf("expected dart test -r json, got: %v", args)
			}
			return strings.Join([]string{
				`{"type":"testDone","test":{"id":1,"name":"adds numbers"},"result":"success"}`,
				`{"type":"testDone","test":{"id":2,"name":"skips legacy","skip":true},"result":"success"}`,
				"",
			}, "\n"), "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "✓ adds numbers") || !strings.Contains(out.String(), "↷ skips legacy") {
		t.Fatalf("expected per-test dart output, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "TEST PASS") || !strings.Contains(out.String(), "tests=2 failed=0 skipped=1") {
		t.Fatalf("expected dart summary details, got: %s", out.String())
	}
}

func TestRunDartTestFailedOnlyFiltersPassedAndSkipped(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	cfg += `

devOutput: {
	color: "false"
	style: "per_test"
	showPassed: true
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		return strings.Join([]string{
			`{"type":"testDone","test":{"id":1,"name":"ok"},"result":"success"}`,
			`{"type":"testDone","test":{"id":2,"name":"skipped","skip":true},"result":"success"}`,
			`{"type":"testDone","test":{"id":3,"name":"fails"},"result":"failure"}`,
			"",
		}, "\n"), "stderr detail", fmt.Errorf("boom")
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test", "--failed-only"}, &out, &errOut)
	if res.ExitCode != ExitFailure {
		t.Fatalf("expected failure, got %d", res.ExitCode)
	}
	if strings.Contains(out.String(), "✓ ok") || strings.Contains(out.String(), "↷ skipped") {
		t.Fatalf("expected pass/skip entries hidden, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "✗ fails") {
		t.Fatalf("expected failing entry shown, got: %s", out.String())
	}
	if !strings.Contains(errOut.String(), "tests=3 failed=1 skipped=1") {
		t.Fatalf("expected failure summary details, got: %s", errOut.String())
	}
}

func TestRunDartTestPerTestStyleWithTestIDEvents(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	cfg += `

devOutput: {
	color: "false"
	style: "per_test"
	showPassed: true
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		return strings.Join([]string{
			`{"type":"testStart","test":{"id":1,"name":"adds numbers"}}`,
			`{"type":"testDone","testID":1,"result":"success","hidden":false}`,
			`{"type":"testStart","test":{"id":2,"name":"skips legacy"}}`,
			`{"type":"testDone","testID":2,"result":"skipped","skipped":true}`,
			"",
		}, "\n"), "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"test"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "✓ adds numbers") || !strings.Contains(out.String(), "↷ skips legacy") {
		t.Fatalf("expected per-test output, got: %s", out.String())
	}
}
