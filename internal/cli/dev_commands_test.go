// purpose: Verify developer command behavior so agents can safely change test/format/lint/cov flows without regressions.
// responsibilities: Exercise command help/usage, language routing, output formatting, thresholds, and failure paths via stubs.
// architecture notes: Tests stub `runCommandCapture` and rely on temp configs to keep assertions deterministic without external toolchain/GitHub dependencies.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func setupCoverageConfig() string {
	return testConfigCue() + `

coverage: {
	default_min_percent: 90
	fail_below_min: true
}

devOutput: {
	color: "false"
}
`
}

func stubGoCoverTotal(t *testing.T, total string) {
	t.Helper()
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "cover" {
			return total, "", nil
		}
		return "", "", nil
	}
}

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

func TestRunCovHelpIncludesMinGuidance(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov", "-h"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, res.ExitCode)
	}
	if !strings.Contains(out.String(), "--min") || !strings.Contains(out.String(), "threshold") {
		t.Fatalf("expected cov help guidance, got: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
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
	cfg := setupCoverageConfig()
	_ = setupTempWorkdirWithConfig(t, cfg)
	stubGoCoverTotal(t, "total: (statements) 75.0%\n")
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
	cfg := setupCoverageConfig()
	_ = setupTempWorkdirWithConfig(t, cfg)
	stubGoCoverTotal(t, "total: (statements) 75.0%\n")
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

func TestRunTestPerTestStyleOutput(t *testing.T) {
	cfg := testConfigCue() + `

devOutput: {
	color: "false"
	style: "per_test"
}
`
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

func TestParseGoTestJSONSummaryIgnoresPackageLevelEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"Action":"run","Package":"x/y","Test":"TestA"}`,
		`{"Action":"pass","Package":"x/y","Test":"TestA"}`,
		`{"Action":"pass","Package":"x/y"}`,
		"",
	}, "\n")
	tests, failed, skipped := parseGoTestJSONSummary(input)
	if tests != 1 || failed != 0 || skipped != 0 {
		t.Fatalf("unexpected summary tests=%d failed=%d skipped=%d", tests, failed, skipped)
	}
}

func TestRunCovUsesUniqueProfileTempPath(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	var coverFuncArgs []string
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "cover" {
			coverFuncArgs = append([]string{}, args...)
			return "total: (statements) 80.0%\n", "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov", "--min", "70"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d stderr=%s", ExitOK, res.ExitCode, errOut.String())
	}
	if len(coverFuncArgs) < 3 || !strings.Contains(coverFuncArgs[2], "-func=") {
		t.Fatalf("unexpected go tool cover args: %v", coverFuncArgs)
	}
	if strings.Contains(coverFuncArgs[2], "gh-flarebyte.coverprofile") {
		t.Fatalf("expected unique temp profile path, got fixed path arg: %s", coverFuncArgs[2])
	}
}

func TestUseColorModes(t *testing.T) {
	oldNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	oldTerm, hadTerm := os.LookupEnv("TERM")
	t.Cleanup(func() {
		if hadNoColor {
			_ = os.Setenv("NO_COLOR", oldNoColor)
		} else {
			_ = os.Unsetenv("NO_COLOR")
		}
		if hadTerm {
			_ = os.Setenv("TERM", oldTerm)
		} else {
			_ = os.Unsetenv("TERM")
		}
	})

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Setenv("TERM", "xterm-256color")
	if !useColor("auto") || !useColor("true") {
		t.Fatalf("expected color enabled for auto/true")
	}
	if useColor("false") {
		t.Fatalf("expected color disabled for false")
	}
	_ = os.Setenv("NO_COLOR", "1")
	if useColor("true") {
		t.Fatalf("expected NO_COLOR to disable color")
	}
}

func TestPrintDevSummaryPerTestStyleUsesSummaryLine(t *testing.T) {
	var b bytes.Buffer
	cfg := config.Config{DevOutput: config.DevOutputConfig{Color: "false", Style: "per_test", ShowPassed: true}}
	printDevSummary(&b, cfg, devSummary{Kind: "lint", Status: "PASS", Duration: 1500 * 1e6, Details: "ok=1"})
	out := b.String()
	if !strings.Contains(out, "LINT PASS duration=") || !strings.Contains(out, "ok=1") {
		t.Fatalf("unexpected per_test summary output: %s", out)
	}
}

func TestRunFormatAndLintFailures(t *testing.T) {
	tmp := setupTempWorkdirWithConfig(t, testConfigCue())
	if err := os.MkdirAll(filepath.Join(tmp, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "pkg", "x.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })

	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "gofmt" {
			return "", "format failed", fmt.Errorf("boom")
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"format"}, &out, &errOut)
	if res.ExitCode != ExitFailure {
		t.Fatalf("expected format failure, got %d", res.ExitCode)
	}

	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 1 && args[0] == "vet" {
			return "", "lint failed", fmt.Errorf("boom")
		}
		return "", "", nil
	}
	out.Reset()
	errOut.Reset()
	res = Run([]string{"lint"}, &out, &errOut)
	if res.ExitCode != ExitFailure {
		t.Fatalf("expected lint failure, got %d", res.ExitCode)
	}
}

func TestDevCommandHelpHandlers(t *testing.T) {
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
	if res := Run([]string{"format", "--help"}, &out, &errOut); res.ExitCode != ExitOK || !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("unexpected format help output: code=%d out=%s", res.ExitCode, out.String())
	}
	out.Reset()
	if res := Run([]string{"lint", "-h"}, &out, &errOut); res.ExitCode != ExitOK || !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("unexpected lint help output: code=%d out=%s", res.ExitCode, out.String())
	}
}

func TestDevCommandUsageErrors(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	if res := Run([]string{"test", "--bad"}, &out, &errOut); res.ExitCode != ExitUsage {
		t.Fatalf("expected usage error for test, got %d", res.ExitCode)
	}
	if res := Run([]string{"format", "--bad"}, &out, &errOut); res.ExitCode != ExitUsage {
		t.Fatalf("expected usage error for format, got %d", res.ExitCode)
	}
	if res := Run([]string{"lint", "--bad"}, &out, &errOut); res.ExitCode != ExitUsage {
		t.Fatalf("expected usage error for lint, got %d", res.ExitCode)
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"lint", "--color", "bad"}, &out, &errOut); res.ExitCode != ExitUsage || !strings.Contains(errOut.String(), "--color") {
		t.Fatalf("expected lint color usage error, got code=%d stderr=%s", res.ExitCode, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"cov", "--color", "bad"}, &out, &errOut); res.ExitCode != ExitUsage || !strings.Contains(errOut.String(), "--color") {
		t.Fatalf("expected cov color usage error, got code=%d stderr=%s", res.ExitCode, errOut.String())
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

func TestRunDartRoutingForFormatAndTest(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	invocations := make([]string, 0)
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		invocations = append(invocations, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	if res := Run([]string{"format"}, &out, &errOut); res.ExitCode != ExitOK {
		t.Fatalf("expected format success, got %d", res.ExitCode)
	}
	out.Reset()
	if res := Run([]string{"test"}, &out, &errOut); res.ExitCode != ExitOK {
		t.Fatalf("expected test success, got %d", res.ExitCode)
	}
	joined := strings.Join(invocations, "\n")
	if !strings.Contains(joined, "dart format .") || !strings.Contains(joined, "dart test") {
		t.Fatalf("unexpected invocations: %s", joined)
	}
}

func TestRunCovFailurePaths(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	var out bytes.Buffer
	var errOut bytes.Buffer

	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) > 0 && strings.HasPrefix(args[0], "test") {
			return "", "go test failed", fmt.Errorf("boom")
		}
		return "", "", nil
	}
	if res := Run([]string{"cov"}, &out, &errOut); res.ExitCode != ExitFailure {
		t.Fatalf("expected cov test-step failure, got %d", res.ExitCode)
	}

	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "cover" {
			return "", "cover failed", fmt.Errorf("boom")
		}
		return "", "", nil
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"cov"}, &out, &errOut); res.ExitCode != ExitFailure {
		t.Fatalf("expected cov cover-step failure, got %d", res.ExitCode)
	}

	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "cover" {
			return "nonsense", "", nil
		}
		return "", "", nil
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"cov"}, &out, &errOut); res.ExitCode != ExitFailure {
		t.Fatalf("expected cov parse failure, got %d", res.ExitCode)
	}
}

func TestRunDartFailurePaths(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })

	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" && len(args) > 0 && args[0] == "format" {
			return "", "format failed", fmt.Errorf("boom")
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	if res := Run([]string{"format"}, &out, &errOut); res.ExitCode != ExitFailure {
		t.Fatalf("expected dart format failure, got %d", res.ExitCode)
	}

	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" && len(args) > 0 && args[0] == "analyze" {
			return "", "lint failed", fmt.Errorf("boom")
		}
		return "", "", nil
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"lint"}, &out, &errOut); res.ExitCode != ExitFailure {
		t.Fatalf("expected dart lint failure, got %d", res.ExitCode)
	}

	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" && len(args) > 0 && args[0] == "test" {
			return "", "test failed", fmt.Errorf("boom")
		}
		return "", "", nil
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"test"}, &out, &errOut); res.ExitCode != ExitFailure {
		t.Fatalf("expected dart test failure, got %d", res.ExitCode)
	}

	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" && len(args) >= 2 && args[0] == "test" && args[1] == "--coverage" {
			return "", "cov failed", fmt.Errorf("boom")
		}
		return "", "", nil
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"cov"}, &out, &errOut); res.ExitCode != ExitFailure {
		t.Fatalf("expected dart cov failure, got %d", res.ExitCode)
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

func TestRunCovBelowMinAllowedWhenFailBelowDisabled(t *testing.T) {
	cfg := testConfigCue() + `

coverage: {
	default_min_percent: 90
	fail_below_min: false
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "cover" {
			return "total: (statements) 10.0%\n", "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success when fail_below_min=false, got %d stderr=%s", res.ExitCode, errOut.String())
	}
}

func TestRunCovPerTestStyleOutput(t *testing.T) {
	cfg := testConfigCue() + `

devOutput: {
	color: "false"
	style: "per_test"
}

func TestRunCovFailedOnlyHidesPassOutput(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "cover" {
			return "total: (statements) 80.0%\n", "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov", "--failed-only"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected no pass output, got: %s", out.String())
	}
}

func TestRunLintFailedOnlyHidesPassOutput(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"lint", "--failed-only"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected no pass output, got: %s", out.String())
	}
}
`
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "cover" {
			return strings.Join([]string{
				"pkg/a.go:10:\tFuncA\t100.0%",
				"pkg/a.go:20:\tFuncB\t0.0%",
				"total: (statements) 80.0%",
				"",
			}, "\n"), "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "✓ pkg/a.go:10:\tFuncA 100.0%") {
		t.Fatalf("expected per-function pass line, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "✗ pkg/a.go:20:\tFuncB 0.0%") {
		t.Fatalf("expected per-function fail line, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "COV PASS duration=") {
		t.Fatalf("expected summary line, got: %s", out.String())
	}
}
