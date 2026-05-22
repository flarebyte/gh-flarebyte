// purpose: Render dev-command output in consistent summary and per-test/per-entry formats.
// responsibilities: Format status lines; apply color policy; print test and coverage entries; format failure snippets.
// architecture notes: Output behavior is config-driven (`devOutput`) so presentation can evolve without changing command execution paths.
package cli

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

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
