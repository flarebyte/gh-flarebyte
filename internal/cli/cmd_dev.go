// purpose: Implement developer-oriented commands (test/format/lint/cov) with shared names across languages.
// responsibilities: Parse command arguments; dispatch by configured build language; run toolchain commands; emit concise summaries.
// architecture notes: Process execution is wrapped through a function variable so tests can stub command behavior deterministically.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runTest(styleOverride, colorOverride string, failedOnly bool, stdout, stderr io.Writer) Result {
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	if styleOverride != "" {
		cfg.DevOutput.Style = styleOverride
	}
	if colorOverride != "" {
		cfg.DevOutput.Color = colorOverride
	}
	env := buildCommandEnv(cfg)
	start := time.Now()
	switch cfg.Build.Language {
	case "go":
		cmdOut, cmdErr, runErr := runCommandCapture("go", []string{"test", "-json", "./..."}, env)
		report := parseGoTestReport(cmdOut)
		if cfg.DevOutput.Style == "per_test" {
			printPerTestEvents(stdout, cfg, report.Events, failedOnly)
		}
		if runErr != nil {
			details := formatGoTestDetails(report.Tests, report.Failed, report.Skipped, "FAIL")
			if failureDetails := formatFailureDetails(report.Failures); failureDetails != "" {
				if details != "" {
					details += "\n"
				}
				details += failureDetails
			}
			if strings.TrimSpace(cmdErr) != "" {
				if details != "" {
					details += "\n"
				}
				details += strings.TrimSpace(cmdErr)
			}
			printDevSummary(stderr, cfg, devSummary{Kind: "test", Status: "FAIL", Duration: time.Since(start), Details: details})
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
		printDevSummary(stdout, cfg, devSummary{Kind: "test", Status: "PASS", Duration: time.Since(start), Details: formatGoTestDetails(report.Tests, report.Failed, report.Skipped, "PASS")})
		return Result{ExitCode: ExitOK}
	case "dart":
		_, cmdErr, runErr := runCommandCapture("dart", []string{"test"}, env)
		if runErr != nil {
			printDevSummary(stderr, cfg, devSummary{Kind: "test", Status: "FAIL", Duration: time.Since(start), Details: strings.TrimSpace(cmdErr)})
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
	default:
		return unsupportedLanguageResult(cfg.Build.Language, stderr)
	}
	printDevSummary(stdout, cfg, devSummary{Kind: "test", Status: "PASS", Duration: time.Since(start)})
	return Result{ExitCode: ExitOK}
}

func runFormat(stdout, stderr io.Writer) Result {
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	env := buildCommandEnv(cfg)
	start := time.Now()
	switch cfg.Build.Language {
	case "go":
		files, err := discoverGoFiles(".")
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitFailure, Err: err}
		}
		if len(files) > 0 {
			_, cmdErr, runErr := runCommandCapture("gofmt", append([]string{"-w"}, files...), env)
			if runErr != nil {
				_, _ = fmt.Fprintln(stderr, strings.TrimSpace(cmdErr))
				return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
			}
		}
		printDevSummary(stdout, cfg, devSummary{Kind: "format", Status: "PASS", Duration: time.Since(start), Details: fmt.Sprintf("files=%d", len(files))})
		return Result{ExitCode: ExitOK}
	case "dart":
		_, cmdErr, runErr := runCommandCapture("dart", []string{"format", "."}, env)
		if runErr != nil {
			_, _ = fmt.Fprintln(stderr, strings.TrimSpace(cmdErr))
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
		printDevSummary(stdout, cfg, devSummary{Kind: "format", Status: "PASS", Duration: time.Since(start)})
		return Result{ExitCode: ExitOK}
	default:
		return unsupportedLanguageResult(cfg.Build.Language, stderr)
	}
}

func runLint(colorOverride string, failedOnly bool, stdout, stderr io.Writer) Result {
	cfg, env, usage := prepareDevCommand(colorOverride, stderr)
	if usage != nil {
		return *usage
	}
	start := time.Now()
	switch cfg.Build.Language {
	case "go":
		_, cmdErr, runErr := runCommandCapture("go", []string{"vet", "./..."}, env)
		if runErr != nil {
			printDevSummary(stderr, cfg, devSummary{Kind: "lint", Status: "FAIL", Duration: time.Since(start), Details: strings.TrimSpace(cmdErr)})
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
	case "dart":
		_, cmdErr, runErr := runCommandCapture("dart", []string{"analyze"}, env)
		if runErr != nil {
			printDevSummary(stderr, cfg, devSummary{Kind: "lint", Status: "FAIL", Duration: time.Since(start), Details: strings.TrimSpace(cmdErr)})
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
	default:
		return unsupportedLanguageResult(cfg.Build.Language, stderr)
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: "lint", Status: "PASS", Duration: time.Since(start)})
	}
	return Result{ExitCode: ExitOK}
}

func runCov(min *float64, colorOverride string, failedOnly bool, stdout, stderr io.Writer) Result {
	cfg, env, usage := prepareDevCommand(colorOverride, stderr)
	if usage != nil {
		return *usage
	}
	start := time.Now()
	switch cfg.Build.Language {
	case "go":
		profileDir, mkErr := os.MkdirTemp("", "gh-flarebyte-cov-")
		if mkErr != nil {
			_, _ = fmt.Fprintln(stderr, mkErr.Error())
			return Result{ExitCode: ExitFailure, Err: mkErr}
		}
		defer func() { _ = os.RemoveAll(profileDir) }()
		profilePath := filepath.Join(profileDir, "cover.out")
		_, cmdErr, runErr := runCommandCapture("go", []string{"test", "-coverprofile=" + profilePath, "./..."}, env)
		if runErr != nil {
			_, _ = fmt.Fprintln(stderr, strings.TrimSpace(cmdErr))
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
		coverOut, coverErr, coverRunErr := runCommandCapture("go", []string{"tool", "cover", "-func=" + profilePath}, env)
		if coverRunErr != nil {
			_, _ = fmt.Fprintln(stderr, strings.TrimSpace(coverErr))
			return Result{ExitCode: ExitFailure, Err: commandError(coverRunErr, coverErr)}
		}
		coverage, parseErr := parseTotalCoverage(coverOut)
		if parseErr != nil {
			_, _ = fmt.Fprintln(stderr, parseErr.Error())
			return Result{ExitCode: ExitFailure, Err: parseErr}
		}
		effectiveMin := resolveCoverageMin(min, cfg)
		if cfg.DevOutput.Style == "per_test" {
			printCoverageDetails(stdout, parseCoverageDetails(coverOut), effectiveMin, failedOnly)
		}
		if effectiveMin != nil && cfg.Coverage.FailBelowMin && coverage < *effectiveMin {
			printDevSummary(stderr, cfg, devSummary{Kind: "cov", Status: "FAIL", Duration: time.Since(start), Details: fmt.Sprintf("total=%.2f%% min=%.2f%%", coverage, *effectiveMin)})
			return Result{ExitCode: ExitFailure, Err: fmt.Errorf("coverage %.2f below minimum %.2f", coverage, *effectiveMin)}
		}
		if !failedOnly {
			if effectiveMin != nil {
				printDevSummary(stdout, cfg, devSummary{Kind: "cov", Status: "PASS", Duration: time.Since(start), Details: fmt.Sprintf("total=%.2f%% min=%.2f%%", coverage, *effectiveMin)})
			} else {
				printDevSummary(stdout, cfg, devSummary{Kind: "cov", Status: "PASS", Duration: time.Since(start), Details: fmt.Sprintf("total=%.2f%%", coverage)})
			}
		}
		return Result{ExitCode: ExitOK}
	case "dart":
		_, cmdErr, runErr := runCommandCapture("dart", []string{"test", "--coverage", ".dart_tool/coverage"}, env)
		if runErr != nil {
			_, _ = fmt.Fprintln(stderr, strings.TrimSpace(cmdErr))
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
		if !failedOnly {
			printDevSummary(stdout, cfg, devSummary{Kind: "cov", Status: "PASS", Duration: time.Since(start)})
		}
		return Result{ExitCode: ExitOK}
	default:
		return unsupportedLanguageResult(cfg.Build.Language, stderr)
	}
}
