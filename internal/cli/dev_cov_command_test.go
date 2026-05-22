// purpose: Validate coverage command behavior including thresholds, output styles, and failure handling.
// responsibilities: Exercise cov flag parsing, coverage threshold enforcement, per_test rendering, and error paths via stubs.
// architecture notes: Tests stub command execution to isolate CLI behavior from real toolchain output variability.
package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
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
	cfg := perTestStyleConfig()
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
