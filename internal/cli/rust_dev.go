// purpose: Run Rust development commands with shared output and coverage policies.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func cargoRun(args []string, env []string) (string, error) {
	// Cargo diagnostics go to stderr; harness output and compiler JSON go to stdout.
	out, errText, err := runCommandCapture("cargo", args, append(env, "CARGO_TERM_COLOR=never"))
	if err != nil {
		return out, fmt.Errorf("cargo %s failed (check the toolchain, components, and Cargo.lock): %w\n%s", args[0], err, strings.TrimSpace(out+"\n"+errText))
	}
	return out, nil
}

func rustFailure(cfg config.Config, kind string, start time.Time, err error, stderr io.Writer) Result {
	printDevSummary(stderr, cfg, devSummary{Kind: kind, Status: "FAIL", Duration: time.Since(start), Details: err.Error()})
	return Result{ExitCode: ExitFailure, Err: err}
}

func runRustTool(cfg config.Config, env []string, kind string, args []string, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	if _, err := cargoMetadata("Cargo.toml", env); err != nil {
		return rustFailure(cfg, kind, start, err, stderr)
	}
	if _, err := cargoRun(args, env); err != nil {
		return rustFailure(cfg, kind, start, err, stderr)
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: kind, Status: "PASS", Duration: time.Since(start)})
	}
	return Result{ExitCode: ExitOK}
}

func runRustTest(cfg config.Config, env []string, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	project, err := cargoMetadata("Cargo.toml", env)
	if err != nil {
		return rustFailure(cfg, "test", start, err, stderr)
	}
	packages, err := rustPackages(project, []string{"./..."})
	if err != nil {
		return rustFailure(cfg, "test", start, err, stderr)
	}
	custom, err := rustCustomHarness(packages)
	if err != nil {
		return rustFailure(cfg, "test", start, err, stderr)
	}
	if custom && cfg.DevOutput.Style == "per_test" {
		return rustFailure(cfg, "test", start, fmt.Errorf("Rust per_test output requires the standard test harness; use --style summary for harness=false targets"), stderr)
	}
	args := []string{"test", "--locked", "--workspace", "--no-fail-fast"}
	if !custom {
		args = append(args, "--", "--format", "pretty", "--color", "never")
	}
	out, runErr := cargoRun(args, append(env, "RUST_TEST_NOCAPTURE=0"))
	report, parseErr := parseRustTests(out)
	if cfg.DevOutput.Style == "per_test" {
		printPerTestEvents(stdout, cfg, report.Events, failedOnly)
	}
	if runErr != nil {
		return rustFailure(cfg, "test", start, runErr, stderr)
	}
	if !custom && parseErr != nil {
		return rustFailure(cfg, "test", start, parseErr, stderr)
	}
	if report.Failed > 0 {
		return rustFailure(cfg, "test", start, fmt.Errorf("Rust test report contains %d failures", report.Failed), stderr)
	}
	if !failedOnly {
		details := formatGoTestDetails(report.Tests, report.Failed, report.Skipped, "PASS")
		if custom {
			details = "custom test harness passed"
		}
		printDevSummary(stdout, cfg, devSummary{Kind: "test", Status: "PASS", Duration: time.Since(start), Details: details})
	}
	return Result{ExitCode: ExitOK}
}

func runRustCov(cfg config.Config, env []string, min *float64, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	project, err := cargoMetadata("Cargo.toml", env)
	if err != nil {
		return rustFailure(cfg, "cov", start, err, stderr)
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-rust-cov-")
	if err != nil {
		return rustFailure(cfg, "cov", start, err, stderr)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "coverage.lcov")
	env = append(env, "CARGO_LLVM_COV_TARGET_DIR="+filepath.Join(dir, "target"), "CARGO_LLVM_COV_BUILD_DIR="+filepath.Join(dir, "target"), "CARGO_LLVM_COV_SETUP=no")
	if _, err := cargoRun([]string{"llvm-cov", "--locked", "--workspace", "--lcov", "--output-path", path}, env); err != nil {
		if strings.Contains(err.Error(), "no such command") || strings.Contains(err.Error(), "llvm-tools-preview") {
			err = fmt.Errorf("install cargo-llvm-cov and rustup component llvm-tools-preview for the active toolchain\n%w", err)
		}
		return rustFailure(cfg, "cov", start, err, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return rustFailure(cfg, "cov", start, fmt.Errorf("Rust coverage report missing: %w", err), stderr)
	}
	percent, details, err := parseRustCoverage(string(data), project.Root)
	if err != nil {
		return rustFailure(cfg, "cov", start, err, stderr)
	}
	effectiveMin := resolveCoverageMin(min, cfg)
	if cfg.DevOutput.Style == "per_test" {
		printCoverageDetails(stdout, details, effectiveMin, failedOnly)
	}
	text := fmt.Sprintf("total=%.2f%%", percent)
	if effectiveMin != nil {
		text += fmt.Sprintf(" min=%.2f%%", *effectiveMin)
		if cfg.Coverage.FailBelowMin && percent < *effectiveMin {
			return rustFailure(cfg, "cov", start, fmt.Errorf("%s: coverage below minimum", text), stderr)
		}
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: "cov", Status: "PASS", Duration: time.Since(start), Details: text})
	}
	return Result{ExitCode: ExitOK}
}
