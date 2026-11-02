// purpose: Run TypeScript developer commands using project-local tools and shared policies.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
	"github.com/pelletier/go-toml/v2"
)

func typescriptTest(project typescriptProject, env []string, coverage bool, dir string) (goTestReport, error) {
	// A private config preserves the project's Bun settings while keeping reports and
	// threshold enforcement under gh-flarebyte's control. Never rewrite bunfig.toml.
	data, err := toml.Marshal(project.Bunfig)
	if err != nil {
		return goTestReport{}, err
	}
	settings := map[string]any{}
	if err := toml.Unmarshal(data, &settings); err != nil {
		return goTestReport{}, err
	}
	test, ok := settings["test"].(map[string]any)
	if !ok {
		test = map[string]any{}
		settings["test"] = test
	}
	test["coverage"] = coverage
	test["coverageThreshold"] = 0
	test["coverageReporter"] = []string{"lcov"}
	test["coverageSkipTestFiles"] = true
	test["coverageIgnoreSourcemaps"] = false
	if root, ok := test["root"].(string); ok && !filepath.IsAbs(root) {
		test["root"] = filepath.Join(project.Root, root)
	}
	delete(test, "reporter")
	data, err = toml.Marshal(settings)
	if err != nil {
		return goTestReport{}, err
	}
	configPath := filepath.Join(dir, "bunfig.toml")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		return goTestReport{}, err
	}
	reportPath := filepath.Join(dir, "junit.xml")
	args := []string{"test", "--config=" + configPath, "--reporter=junit", "--reporter-outfile=" + reportPath}
	if coverage {
		args = append(args, "--coverage", "--coverage-reporter=lcov", "--coverage-dir="+dir)
	}
	_, runErr := typescriptRun(args, env)
	data, readErr := os.ReadFile(reportPath)
	report, parseErr := parseTypeScriptTests(data)
	if runErr != nil {
		return report, runErr
	}
	if readErr != nil {
		return report, fmt.Errorf("Bun JUnit report missing: %w", readErr)
	}
	if parseErr != nil {
		return report, parseErr
	}
	if report.Tests == 0 || report.Failed > 0 {
		return report, fmt.Errorf("Bun test report: tests=%d failed=%d; expected passing tests", report.Tests, report.Failed)
	}
	return report, nil
}

func runTypeScriptDev(cfg config.Config, env []string, kind string, min *float64, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	fail := func(err error) Result {
		printDevSummary(stderr, cfg, devSummary{Kind: kind, Status: "FAIL", Duration: time.Since(start), Details: err.Error()})
		return Result{ExitCode: ExitFailure, Err: err}
	}
	project, err := prepareTypeScript(env)
	if err != nil {
		return fail(err)
	}
	details := ""
	switch kind {
	case "format":
		_, err = typescriptRun([]string{typescriptBiome, "format", "--write", "."}, env)
	case "lint":
		_, err = typescriptRun([]string{typescriptBiome, "check", "."}, env)
		if err == nil {
			err = typescriptTypecheck(env)
		}
	case "test", "cov":
		dir, mkErr := os.MkdirTemp("", "gh-flarebyte-typescript-")
		if mkErr != nil {
			return fail(mkErr)
		}
		defer func() { _ = os.RemoveAll(dir) }()
		report, testErr := typescriptTest(project, env, kind == "cov", dir)
		if kind == "test" && cfg.DevOutput.Style == "per_test" {
			printPerTestEvents(stdout, cfg, report.Events, failedOnly)
		}
		if testErr != nil {
			return fail(testErr)
		}
		details = formatGoTestDetails(report.Tests, report.Failed, report.Skipped, "PASS")
		if kind == "cov" {
			allowed, err := project.coverageFiles(env)
			if err != nil {
				return fail(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "lcov.info"))
			if err != nil {
				return fail(fmt.Errorf("Bun coverage report missing: %w", err))
			}
			percent, entries, err := parseTypeScriptCoverage(string(data), project.Root, allowed)
			if err != nil {
				return fail(err)
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
		return fail(fmt.Errorf("unsupported TypeScript developer command %q", kind))
	}
	if err != nil {
		return fail(err)
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: kind, Status: "PASS", Duration: time.Since(start), Details: details})
	}
	return Result{ExitCode: ExitOK}
}
