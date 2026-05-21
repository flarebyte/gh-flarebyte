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
	Action string `json:"Action"`
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

func handleTest(args []string, stdout, stderr io.Writer) Result {
	if len(args) != 0 {
		err := fmt.Errorf("invalid invocation: unknown argument %q", args[0])
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	env := buildCommandEnv(cfg)
	start := time.Now()
	switch cfg.Build.Language {
	case "go":
		cmdOut, cmdErr, runErr := runCommandCapture("go", []string{"test", "-json", "./..."}, env)
		if runErr != nil {
			tests, failed, skipped := parseGoTestJSONSummary(cmdOut)
			details := formatGoTestDetails(tests, failed, skipped, "FAIL")
			if strings.TrimSpace(cmdErr) != "" {
				if details != "" {
					details += " "
				}
				details += strings.TrimSpace(cmdErr)
			}
			printDevSummary(stderr, cfg, devSummary{Kind: "test", Status: "FAIL", Duration: time.Since(start), Details: details})
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
		tests, failed, skipped := parseGoTestJSONSummary(cmdOut)
		printDevSummary(stdout, cfg, devSummary{Kind: "test", Status: "PASS", Duration: time.Since(start), Details: formatGoTestDetails(tests, failed, skipped, "PASS")})
		return Result{ExitCode: ExitOK}
	case "dart":
		_, cmdErr, runErr := runCommandCapture("dart", []string{"test"}, env)
		if runErr != nil {
			printDevSummary(stderr, cfg, devSummary{Kind: "test", Status: "FAIL", Duration: time.Since(start), Details: strings.TrimSpace(cmdErr)})
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
	default:
		err := fmt.Errorf("build.language %q is not supported. Supported values: go, dart", cfg.Build.Language)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	printDevSummary(stdout, cfg, devSummary{Kind: "test", Status: "PASS", Duration: time.Since(start)})
	return Result{ExitCode: ExitOK}
}

func handleFormat(args []string, stdout, stderr io.Writer) Result {
	if len(args) != 0 {
		err := fmt.Errorf("invalid invocation: unknown argument %q", args[0])
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
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
		err := fmt.Errorf("build.language %q is not supported. Supported values: go, dart", cfg.Build.Language)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
}

func handleLint(args []string, stdout, stderr io.Writer) Result {
	if len(args) != 0 {
		err := fmt.Errorf("invalid invocation: unknown argument %q", args[0])
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
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
		err := fmt.Errorf("build.language %q is not supported. Supported values: go, dart", cfg.Build.Language)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	printDevSummary(stdout, cfg, devSummary{Kind: "lint", Status: "PASS", Duration: time.Since(start)})
	return Result{ExitCode: ExitOK}
}

func handleCov(args []string, stdout, stderr io.Writer) Result {
	min, err := parseCovArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	env := buildCommandEnv(cfg)
	start := time.Now()
	switch cfg.Build.Language {
	case "go":
		profilePath := filepath.Join(os.TempDir(), "gh-flarebyte.coverprofile")
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
		if effectiveMin != nil && cfg.Coverage.FailBelowMin && coverage < *effectiveMin {
			printDevSummary(stderr, cfg, devSummary{Kind: "cov", Status: "FAIL", Duration: time.Since(start), Details: fmt.Sprintf("total=%.2f%% min=%.2f%%", coverage, *effectiveMin)})
			return Result{ExitCode: ExitFailure, Err: fmt.Errorf("coverage %.2f below minimum %.2f", coverage, *effectiveMin)}
		}
		if effectiveMin != nil {
			printDevSummary(stdout, cfg, devSummary{Kind: "cov", Status: "PASS", Duration: time.Since(start), Details: fmt.Sprintf("total=%.2f%% min=%.2f%%", coverage, *effectiveMin)})
		} else {
			printDevSummary(stdout, cfg, devSummary{Kind: "cov", Status: "PASS", Duration: time.Since(start), Details: fmt.Sprintf("total=%.2f%%", coverage)})
		}
		return Result{ExitCode: ExitOK}
	case "dart":
		_, cmdErr, runErr := runCommandCapture("dart", []string{"test", "--coverage", ".dart_tool/coverage"}, env)
		if runErr != nil {
			_, _ = fmt.Fprintln(stderr, strings.TrimSpace(cmdErr))
			return Result{ExitCode: ExitFailure, Err: commandError(runErr, cmdErr)}
		}
		printDevSummary(stdout, cfg, devSummary{Kind: "cov", Status: "PASS", Duration: time.Since(start)})
		return Result{ExitCode: ExitOK}
	default:
		err := fmt.Errorf("build.language %q is not supported. Supported values: go, dart", cfg.Build.Language)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
}

func parseCovArgs(args []string) (*float64, error) {
	var min *float64
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--min":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("invalid invocation: --min requires a numeric percentage")
			}
			v, err := strconv.ParseFloat(args[i+1], 64)
			if err != nil || v < 0 || v > 100 {
				return nil, fmt.Errorf("invalid invocation: --min must be a number between 0 and 100")
			}
			min = &v
			i++
		default:
			return nil, fmt.Errorf("invalid invocation: unknown argument %q", args[i])
		}
	}
	return min, nil
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
	case "one_line":
		if s.Details != "" {
			_, _ = fmt.Fprintf(w, "%s kind=%s %s duration=%s\n", status, s.Kind, s.Details, d)
		} else {
			_, _ = fmt.Fprintf(w, "%s kind=%s duration=%s\n", status, s.Kind, d)
		}
	case "list":
		_, _ = fmt.Fprintf(w, "- kind: %s\n- status: %s\n- duration: %s\n", s.Kind, status, d)
		if s.Details != "" {
			_, _ = fmt.Fprintf(w, "- details: %s\n", s.Details)
		}
	default:
		if s.Details != "" {
			_, _ = fmt.Fprintf(w, "%s %s duration=%s %s\n", strings.ToUpper(s.Kind), status, d, s.Details)
		} else {
			_, _ = fmt.Fprintf(w, "%s %s duration=%s\n", strings.ToUpper(s.Kind), status, d)
		}
	}
	if s.Status == "FAIL" && s.Details != "" && cfg.DevOutput.Style != "list" {
		_, _ = fmt.Fprintln(w, s.Details)
	}
}

func parseGoTestJSONSummary(out string) (tests int, failed int, skipped int) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			tests++
		}
		if ev.Action == "fail" {
			failed++
		}
		if ev.Action == "skip" {
			skipped++
		}
	}
	return tests, failed, skipped
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
