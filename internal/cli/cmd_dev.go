// purpose: Implement developer-oriented commands (test/format/lint/cov) with shared names across languages.
// responsibilities: Parse command arguments; dispatch by configured build language; run toolchain commands; emit concise summaries.
// architecture notes: Process execution is wrapped through a function variable so tests can stub command behavior deterministically.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

type devSummary struct {
	Kind     string
	Status   string
	Duration time.Duration
	Details  string
}

type goTestEvent struct {
	Action  string `json:"Action"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
	Package string `json:"Package"`
}

type goFailure struct {
	Test    string
	At      string
	Snippet string
}

type goTestReport struct {
	Tests    int
	Failed   int
	Skipped  int
	Events   []goTestEvent
	Failures []goFailure
}

type coverageDetail struct {
	Label   string
	Percent float64
}

var runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
	cmd := exec.Command(name, args...)
	if len(env) > 0 {
		cmd.Env = env
	}
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

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
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	if colorOverride != "" {
		cfg.DevOutput.Color = colorOverride
	}
	env := buildCommandEnv(cfg)
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
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	if colorOverride != "" {
		cfg.DevOutput.Color = colorOverride
	}
	env := buildCommandEnv(cfg)
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

func unsupportedLanguageResult(language string, stderr io.Writer) Result {
	err := fmt.Errorf("build.language %q is not supported. Supported values: go, dart", language)
	_, _ = fmt.Fprintln(stderr, err.Error())
	return Result{ExitCode: ExitUsage, Err: err}
}

func resolveCoverageMin(cliMin *float64, cfg config.Config) *float64 {
	if cliMin != nil {
		return cliMin
	}
	return cfg.Coverage.DefaultMinPercent
}

func buildCommandEnv(cfg config.Config) []string {
	env := os.Environ()
	if cfg.Go.CacheDir != "" {
		env = append(env, "GOCACHE="+cfg.Go.CacheDir)
	}
	if cfg.Go.ModCacheDir != "" {
		env = append(env, "GOMODCACHE="+cfg.Go.ModCacheDir)
	}
	if cfg.Go.Toolchain != "" {
		env = append(env, "GOTOOLCHAIN="+cfg.Go.Toolchain)
	}
	return env
}

func discoverGoFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "build" || name == ".gocache" || name == ".gomodcache" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func parseTotalCoverage(out string) (float64, error) {
	re := regexp.MustCompile(`(?m)^total:\s+\(statements\)\s+([0-9]+(?:\.[0-9]+)?)%$`)
	m := re.FindStringSubmatch(out)
	if len(m) < 2 {
		return 0, fmt.Errorf("unable to parse total coverage from go tool cover output")
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("unable to parse total coverage value: %w", err)
	}
	return v, nil
}

func parseCoverageDetails(out string) []coverageDetail {
	lines := strings.Split(out, "\n")
	details := make([]coverageDetail, 0, len(lines))
	re := regexp.MustCompile(`^(.*)\s+([0-9]+(?:\.[0-9]+)?)%$`)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total:") {
			continue
		}
		m := re.FindStringSubmatch(line)
		if len(m) < 3 {
			continue
		}
		pct, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		details = append(details, coverageDetail{
			Label:   strings.TrimSpace(m[1]),
			Percent: pct,
		})
	}
	return details
}

func printDevSummary(w io.Writer, cfg config.Config, s devSummary) {
	if s.Status == "PASS" && !cfg.DevOutput.ShowPassed {
		return
	}
	status := s.Status
	if useColor(cfg.DevOutput.Color) {
		if s.Status == "PASS" {
			status = "\x1b[32mPASS\x1b[0m"
		} else {
			status = "\x1b[31mFAIL\x1b[0m"
		}
	}
	d := s.Duration.Round(time.Millisecond)
	switch cfg.DevOutput.Style {
	case "summary", "per_test":
		if s.Details != "" {
			_, _ = fmt.Fprintf(w, "%s %s duration=%s %s\n", strings.ToUpper(s.Kind), status, d, s.Details)
		} else {
			_, _ = fmt.Fprintf(w, "%s %s duration=%s\n", strings.ToUpper(s.Kind), status, d)
		}
	default:
		if s.Details != "" {
			_, _ = fmt.Fprintf(w, "%s %s duration=%s %s\n", strings.ToUpper(s.Kind), status, d, s.Details)
		} else {
			_, _ = fmt.Fprintf(w, "%s %s duration=%s\n", strings.ToUpper(s.Kind), status, d)
		}
	}
}

func parseGoTestJSONSummary(out string) (tests int, failed int, skipped int) {
	report := parseGoTestReport(out)
	return report.Tests, report.Failed, report.Skipped
}

func parseGoTestReport(out string) goTestReport {
	report := goTestReport{}
	lastOutput := map[string]string{}
	lastLocation := map[string]string{}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		report.Events = append(report.Events, ev)
		// Count only concrete test-case events and ignore package-level pass/fail lines.
		switch ev.Action {
		case "pass", "fail", "skip":
			if ev.Test == "" {
				continue
			}
			report.Tests++
		}
		if ev.Action == "fail" {
			if ev.Test != "" {
				report.Failed++
				report.Failures = append(report.Failures, goFailure{
					Test:    ev.Test,
					At:      lastLocation[ev.Test],
					Snippet: lastOutput[ev.Test],
				})
			}
		}
		if ev.Action == "skip" {
			report.Skipped++
		}
		if ev.Test != "" && ev.Output != "" {
			output := strings.TrimSpace(ev.Output)
			if output != "" {
				lastOutput[ev.Test] = output
				if loc := extractFailureLocation(output); loc != "" {
					lastLocation[ev.Test] = loc
				}
			}
		}
	}
	return report
}

func formatGoTestDetails(tests int, failed int, skipped int, status string) string {
	if tests == 0 {
		return ""
	}
	if status == "PASS" {
		return fmt.Sprintf("tests=%d failed=0 skipped=%d", tests, skipped)
	}
	return fmt.Sprintf("tests=%d failed=%d skipped=%d", tests, failed, skipped)
}

func useColor(mode string) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	switch mode {
	case "true":
		return true
	case "false":
		return false
	default:
		return os.Getenv("TERM") != "" && os.Getenv("TERM") != "dumb"
	}
}

func printPerTestEvents(w io.Writer, cfg config.Config, events []goTestEvent, failedOnly bool) {
	for _, ev := range events {
		if ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "pass":
			if !failedOnly && cfg.DevOutput.ShowPassed {
				_, _ = fmt.Fprintf(w, "✓ %s\n", ev.Test)
			}
		case "skip":
			if !failedOnly {
				_, _ = fmt.Fprintf(w, "↷ %s\n", ev.Test)
			}
		case "fail":
			_, _ = fmt.Fprintf(w, "✗ %s\n", ev.Test)
		}
	}
}

func printCoverageDetails(w io.Writer, details []coverageDetail, min *float64, failedOnly bool) {
	for _, d := range details {
		mark := "✓"
		isFail := false
		if min != nil {
			if d.Percent < *min {
				mark = "✗"
				isFail = true
			}
		} else if d.Percent == 0 {
			mark = "✗"
			isFail = true
		}
		if failedOnly && !isFail {
			continue
		}
		_, _ = fmt.Fprintf(w, "%s %s %.1f%%\n", mark, d.Label, d.Percent)
	}
}

func formatFailureDetails(failures []goFailure) string {
	if len(failures) == 0 {
		return ""
	}
	lines := []string{"FAILED:"}
	for _, f := range failures {
		lines = append(lines, "- "+f.Test)
		if f.At != "" {
			lines = append(lines, "  at "+f.At)
		}
		if f.Snippet != "" {
			lines = append(lines, "  snippet: "+f.Snippet)
		}
	}
	return strings.Join(lines, "\n")
}

func extractFailureLocation(output string) string {
	re := regexp.MustCompile(`([A-Za-z0-9_./\\-]+_test\.go:\d+)`)
	m := re.FindStringSubmatch(output)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
