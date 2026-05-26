// purpose: Validate coverage command behavior including thresholds, output styles, and failure handling.
// responsibilities: Exercise cov flag parsing, coverage threshold enforcement, per_test rendering, and error paths via stubs.
// architecture notes: Tests stub command execution to isolate CLI behavior from real toolchain output variability.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupCoverageConfig() string {
	return testConfigCue() + `

coverage: {
	min: 90
	enforceMin: true
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
	min: 90
	enforceMin: false
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
		t.Fatalf("expected success when enforceMin=false, got %d stderr=%s", res.ExitCode, errOut.String())
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

func TestRunCovDartFailsBelowThreshold(t *testing.T) {
	cfg := strings.Replace(setupCoverageConfig(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" && len(args) >= 3 && args[0] == "test" && args[1] == "--coverage" {
			if err := os.MkdirAll(filepath.Join(".dart_tool", "coverage"), 0o755); err != nil {
				return "", "", err
			}
			lcov := strings.Join([]string{
				"SF:lib/a.dart",
				"LF:10",
				"LH:7",
				"end_of_record",
				"",
			}, "\n")
			if err := os.WriteFile(filepath.Join(".dart_tool", "coverage", "lcov.info"), []byte(lcov), 0o644); err != nil {
				return "", "", err
			}
			return "", "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov"}, &out, &errOut)
	if res.ExitCode != ExitFailure {
		t.Fatalf("expected failure, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !strings.Contains(errOut.String(), "COV FAIL") || !strings.Contains(errOut.String(), "total=70.00% min=90.00%") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunCovDartMinFlagOverridesConfigThreshold(t *testing.T) {
	cfg := strings.Replace(setupCoverageConfig(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" && len(args) >= 3 && args[0] == "test" && args[1] == "--coverage" {
			if err := os.MkdirAll(filepath.Join(".dart_tool", "coverage"), 0o755); err != nil {
				return "", "", err
			}
			lcov := strings.Join([]string{
				"SF:lib/a.dart",
				"LF:10",
				"LH:7",
				"end_of_record",
				"",
			}, "\n")
			if err := os.WriteFile(filepath.Join(".dart_tool", "coverage", "lcov.info"), []byte(lcov), 0o644); err != nil {
				return "", "", err
			}
			return "", "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov", "--min", "70"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "min=70.00%") || !strings.Contains(out.String(), "total=70.00%") {
		t.Fatalf("unexpected stdout: %s", out.String())
	}
}

func TestRunCovDartPerTestStyleOutput(t *testing.T) {
	cfg := strings.Replace(perTestStyleConfig(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" && len(args) >= 3 && args[0] == "test" && args[1] == "--coverage" {
			if err := os.MkdirAll(filepath.Join(".dart_tool", "coverage"), 0o755); err != nil {
				return "", "", err
			}
			lcov := strings.Join([]string{
				"SF:lib/a.dart",
				"LF:10",
				"LH:10",
				"end_of_record",
				"SF:lib/b.dart",
				"LF:10",
				"LH:0",
				"end_of_record",
				"",
			}, "\n")
			if err := os.WriteFile(filepath.Join(".dart_tool", "coverage", "lcov.info"), []byte(lcov), 0o644); err != nil {
				return "", "", err
			}
			return "", "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "✓ lib/a.dart 100.0%") || !strings.Contains(out.String(), "✗ lib/b.dart 0.0%") {
		t.Fatalf("unexpected per-test output: %s", out.String())
	}
}

func TestRunCovDartIgnoresPubCacheEntries(t *testing.T) {
	cfg := strings.Replace(perTestStyleConfig(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" && len(args) >= 3 && args[0] == "test" && args[1] == "--coverage" {
			if err := os.MkdirAll(filepath.Join(".dart_tool", "coverage"), 0o755); err != nil {
				return "", "", err
			}
			lcov := strings.Join([]string{
				"SF:/Users/olivier/.pub-cache/hosted/pub.dev/test_core-0.6.12/lib/src/scaffolding.dart",
				"LF:10",
				"LH:2",
				"end_of_record",
				"SF:lib/a.dart",
				"LF:10",
				"LH:10",
				"end_of_record",
				"",
			}, "\n")
			if err := os.WriteFile(filepath.Join(".dart_tool", "coverage", "lcov.info"), []byte(lcov), 0o644); err != nil {
				return "", "", err
			}
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if strings.Contains(out.String(), ".pub-cache") {
		t.Fatalf("expected pub-cache entries filtered out, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "total=100.00%") {
		t.Fatalf("expected total based on project files only, got: %s", out.String())
	}
}

func TestRunCovDartUsesRelativePathsAndScopesToCurrentDir(t *testing.T) {
	cfg := strings.Replace(perTestStyleConfig(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	cwdPath, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name == "dart" && len(args) >= 3 && args[0] == "test" && args[1] == "--coverage" {
			if err := os.MkdirAll(filepath.Join(".dart_tool", "coverage"), 0o755); err != nil {
				return "", "", err
			}
			lcov := strings.Join([]string{
				"SF:" + filepath.Join(cwdPath, "lib", "src", "client", "rest_requests.dart"),
				"LF:10",
				"LH:3",
				"end_of_record",
				"SF:/tmp/outside.dart",
				"LF:10",
				"LH:10",
				"end_of_record",
				"",
			}, "\n")
			if err := os.WriteFile(filepath.Join(".dart_tool", "coverage", "lcov.info"), []byte(lcov), 0o644); err != nil {
				return "", "", err
			}
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "lib/src/client/rest_requests.dart 30.0%") {
		t.Fatalf("expected relative local path in output, got: %s", out.String())
	}
	if strings.Contains(out.String(), cwdPath) || strings.Contains(out.String(), "/tmp/outside.dart") {
		t.Fatalf("expected absolute/outside paths filtered, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "total=30.00%") {
		t.Fatalf("expected total scoped to current dir only, got: %s", out.String())
	}
}

func TestRunCovDartGeneratesLCOVWhenMissing(t *testing.T) {
	cfg := strings.Replace(setupCoverageConfig(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name != "dart" {
			return "", "", nil
		}
		if len(args) >= 3 && args[0] == "test" && args[1] == "--coverage" {
			return "", "", nil
		}
		if len(args) >= 4 && args[0] == "pub" && args[1] == "global" && args[2] == "run" && args[3] == "coverage:format_coverage" {
			if err := os.MkdirAll(filepath.Join(".dart_tool", "coverage"), 0o755); err != nil {
				return "", "", err
			}
			lcov := strings.Join([]string{
				"SF:lib/a.dart",
				"LF:10",
				"LH:10",
				"end_of_record",
				"",
			}, "\n")
			if err := os.WriteFile(filepath.Join(".dart_tool", "coverage", "lcov.info"), []byte(lcov), 0o644); err != nil {
				return "", "", err
			}
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !strings.Contains(out.String(), "COV PASS") {
		t.Fatalf("expected pass summary, got: %s", out.String())
	}
}

func TestRunCovDartReactivatesCoverageToolWhenGlobalRunDepsDrift(t *testing.T) {
	cfg := strings.Replace(setupCoverageConfig(), `language:     "go"`, `language:     "dart"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldRun := runCommandCapture
	t.Cleanup(func() { runCommandCapture = oldRun })
	formatAttempts := 0
	activateCalled := false
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name != "dart" {
			return "", "", nil
		}
		if len(args) >= 3 && args[0] == "test" && args[1] == "--coverage" {
			return "", "", nil
		}
		if len(args) >= 4 && args[0] == "pub" && args[1] == "global" && args[2] == "run" && args[3] == "coverage:format_coverage" {
			formatAttempts++
			if formatAttempts == 1 {
				return "", "The current activation of `coverage` cannot resolve to the same set of dependencies.", fmt.Errorf("boom")
			}
			if err := os.MkdirAll(filepath.Join(".dart_tool", "coverage"), 0o755); err != nil {
				return "", "", err
			}
			lcov := strings.Join([]string{
				"SF:lib/a.dart",
				"LF:10",
				"LH:10",
				"end_of_record",
				"",
			}, "\n")
			if err := os.WriteFile(filepath.Join(".dart_tool", "coverage", "lcov.info"), []byte(lcov), 0o644); err != nil {
				return "", "", err
			}
			return "", "", nil
		}
		if len(args) >= 4 && args[0] == "pub" && args[1] == "global" && args[2] == "activate" && args[3] == "coverage" {
			activateCalled = true
			return "", "", nil
		}
		return "", "", nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"cov"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", res.ExitCode, errOut.String())
	}
	if !activateCalled {
		t.Fatalf("expected coverage activation to be called")
	}
	if formatAttempts < 2 {
		t.Fatalf("expected format_coverage retry, got %d attempt(s)", formatAttempts)
	}
}
