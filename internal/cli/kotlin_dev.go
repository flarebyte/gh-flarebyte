// purpose: Run Kotlin developer commands with shared output and coverage policy.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func runKotlinDev(cfg config.Config, env []string, kind string, min *float64, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	fail := func(err error) Result {
		printDevSummary(stderr, cfg, devSummary{Kind: kind, Status: "FAIL", Duration: time.Since(start), Details: err.Error()})
		return Result{ExitCode: ExitFailure, Err: err}
	}
	project, err := prepareKotlin()
	if err != nil {
		return fail(err)
	}
	defer func() { _ = os.RemoveAll(project.Dir) }()
	details := ""
	switch kind {
	case "format":
		err = project.run(env, kind, "ktlintFormat")
	case "lint":
		err = project.run(env, kind, "ktlintCheck", "classes", "testClasses")
	case "test", "cov":
		tasks := []string{"test"}
		if kind == "cov" {
			tasks = append(tasks, "jacocoTestReport")
		}
		runErr := project.run(env, kind, tasks...)
		report, reportErr := project.tests()
		if kind == "test" && cfg.DevOutput.Style == "per_test" {
			printPerTestEvents(stdout, cfg, report.Events, failedOnly)
		}
		if runErr != nil {
			return fail(runErr)
		}
		if reportErr != nil {
			return fail(reportErr)
		}
		details = formatGoTestDetails(report.Tests, report.Failed, report.Skipped, "PASS")
		if kind == "cov" {
			data, readErr := os.ReadFile(filepath.Join(project.Dir, "jacoco.xml"))
			if readErr != nil {
				return fail(fmt.Errorf("Kotlin JaCoCo report missing: %w", readErr))
			}
			percent, entries, parseErr := parseKotlinCoverage(data)
			if parseErr != nil {
				return fail(parseErr)
			}
			threshold := resolveCoverageMin(min, cfg)
			if cfg.DevOutput.Style == "per_test" {
				printCoverageDetails(stdout, entries, threshold, failedOnly)
			}
			details = fmt.Sprintf("total=%.2f%%", percent)
			if threshold != nil {
				details += fmt.Sprintf(" min=%.2f%%", *threshold)
				if cfg.Coverage.FailBelowMin && percent < *threshold {
					return fail(fmt.Errorf("%s: coverage below minimum", details))
				}
			}
		}
	default:
		return fail(fmt.Errorf("unsupported Kotlin developer command %q", kind))
	}
	if err != nil {
		return fail(err)
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: kind, Status: "PASS", Duration: time.Since(start), Details: details})
	}
	return Result{ExitCode: ExitOK}
}
