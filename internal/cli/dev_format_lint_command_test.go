// purpose: Validate format and lint command routing, usage handling, and failure semantics.
// responsibilities: Assert help/usage output, language dispatch, pass/fail behavior, and failed-only output suppression.
// architecture notes: Command runner stubs keep tests deterministic and focused on CLI orchestration rather than external tools.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestDevFormatLintCommandHelpAndUsage(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	if res := Run([]string{"format", "--help"}, &out, &errOut); res.ExitCode != ExitOK || !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("unexpected format help output: code=%d out=%s", res.ExitCode, out.String())
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"lint", "-h"}, &out, &errOut); res.ExitCode != ExitOK || !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("unexpected lint help output: code=%d out=%s", res.ExitCode, out.String())
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"format", "--bad"}, &out, &errOut); res.ExitCode != ExitUsage {
		t.Fatalf("expected usage error for format, got %d", res.ExitCode)
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"lint", "--bad"}, &out, &errOut); res.ExitCode != ExitUsage {
		t.Fatalf("expected usage error for lint, got %d", res.ExitCode)
	}
	out.Reset()
	errOut.Reset()
	if res := Run([]string{"lint", "--color", "bad"}, &out, &errOut); res.ExitCode != ExitUsage || !strings.Contains(errOut.String(), "--color") {
		t.Fatalf("expected lint color usage error, got code=%d stderr=%s", res.ExitCode, errOut.String())
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
