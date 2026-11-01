// purpose: Run SwiftPM developer commands with shared output and coverage policies.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func swiftFailure(cfg config.Config, kind string, start time.Time, err error, stderr io.Writer) Result {
	printDevSummary(stderr, cfg, devSummary{Kind: kind, Status: "FAIL", Duration: time.Since(start), Details: err.Error()})
	return Result{ExitCode: ExitFailure, Err: err}
}

func runSwiftTool(cfg config.Config, env []string, kind string, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	project, err := loadSwiftProject(env)
	if err != nil {
		return swiftFailure(cfg, kind, start, err, stderr)
	}
	sources, err := swiftSourceFiles(project, false)
	if err != nil {
		return swiftFailure(cfg, kind, start, err, stderr)
	}
	var files []string
	for path := range sources {
		if strings.HasSuffix(path, ".swift") {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	args := []string{"format", "format", "--in-place", "--no-color-diagnostics"}
	if kind == "lint" {
		args = []string{"format", "lint", "--strict", "--no-color-diagnostics"}
	}
	// Explicit source lists keep generated code and dependencies out of formatting.
	if _, _, err = swiftRun(append(args, files...), env); err != nil {
		return swiftFailure(cfg, kind, start, err, stderr)
	}
	if kind == "lint" {
		_, _, err = swiftRun([]string{"build", "--force-resolved-versions", "--build-tests", "-Xswiftc", "-warnings-as-errors"}, env)
		if err != nil {
			return swiftFailure(cfg, kind, start, err, stderr)
		}
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: kind, Status: "PASS", Duration: time.Since(start)})
	}
	return Result{ExitCode: ExitOK}
}

func runSwiftTest(cfg config.Config, env []string, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	if _, err := loadSwiftProject(env); err != nil {
		return swiftFailure(cfg, "test", start, err, stderr)
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-swift-test-")
	if err != nil {
		return swiftFailure(cfg, "test", start, err, stderr)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	streamPath := filepath.Join(dir, "events.jsonl")
	out, diagnostics, runErr := swiftRun([]string{"test", "--force-resolved-versions", "--no-parallel", "--event-stream-version", "0", "--event-stream-output-path", streamPath}, env)
	stream, readErr := os.ReadFile(streamPath)
	report, parseErr := parseSwiftTests(out+"\n"+diagnostics, string(stream))
	if cfg.DevOutput.Style == "per_test" {
		printPerTestEvents(stdout, cfg, report.Events, failedOnly)
	}
	if runErr != nil {
		return swiftFailure(cfg, "test", start, runErr, stderr)
	}
	if readErr != nil {
		return swiftFailure(cfg, "test", start, fmt.Errorf("Swift Testing event report missing: %w", readErr), stderr)
	}
	if parseErr != nil {
		return swiftFailure(cfg, "test", start, parseErr, stderr)
	}
	if report.Failed > 0 {
		return swiftFailure(cfg, "test", start, fmt.Errorf("Swift test report contains %d failures", report.Failed), stderr)
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: "test", Status: "PASS", Duration: time.Since(start), Details: formatGoTestDetails(report.Tests, report.Failed, report.Skipped, "PASS")})
	}
	return Result{ExitCode: ExitOK}
}

func runSwiftCov(cfg config.Config, env []string, min *float64, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	project, err := loadSwiftProject(env)
	if err != nil {
		return swiftFailure(cfg, "cov", start, err, stderr)
	}
	sources, err := swiftSourceFiles(project, true)
	if err != nil {
		return swiftFailure(cfg, "cov", start, err, stderr)
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-swift-cov-")
	if err != nil {
		return swiftFailure(cfg, "cov", start, err, stderr)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	args := []string{"test", "--force-resolved-versions", "--scratch-path", dir, "--enable-code-coverage"}
	if _, _, err := swiftRun(args, env); err != nil {
		return swiftFailure(cfg, "cov", start, err, stderr)
	}
	path, _, err := swiftRun(append(args, "--show-codecov-path"), env)
	if err != nil {
		return swiftFailure(cfg, "cov", start, err, stderr)
	}
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return swiftFailure(cfg, "cov", start, fmt.Errorf("Swift coverage report missing: %w", err), stderr)
	}
	percent, details, err := parseSwiftCoverage(data, sources)
	if err != nil {
		return swiftFailure(cfg, "cov", start, err, stderr)
	}
	effectiveMin := resolveCoverageMin(min, cfg)
	if cfg.DevOutput.Style == "per_test" {
		printCoverageDetails(stdout, details, effectiveMin, failedOnly)
	}
	text := fmt.Sprintf("total=%.2f%%", percent)
	if effectiveMin != nil {
		text += fmt.Sprintf(" min=%.2f%%", *effectiveMin)
		if cfg.Coverage.FailBelowMin && percent < *effectiveMin {
			return swiftFailure(cfg, "cov", start, fmt.Errorf("%s: coverage below minimum", text), stderr)
		}
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: "cov", Status: "PASS", Duration: time.Since(start), Details: text})
	}
	return Result{ExitCode: ExitOK}
}
