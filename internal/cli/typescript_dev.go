// purpose: Run TypeScript developer commands using project-local tools and shared policies.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func typescriptTest(project typescriptProject, env []string, coverage bool, dir string) (goTestReport, error) {
	reportPath := filepath.Join(dir, "junit.xml")
	env = nodeReportEnv(env, reportPath)
	var out string
	var runErr error
	if coverage {
		tool := "./node_modules/c8/bin/c8.js"
		if info, err := os.Stat(tool); err != nil || !info.Mode().IsRegular() {
			return goTestReport{}, fmt.Errorf("TypeScript coverage requires project-local c8; add c8 to devDependencies")
		}
		args := []string{tool, "--all", "--extension=.ts", "--extension=.tsx", "--extension=.mts", "--extension=.cts", "--extension=.js", "--extension=.jsx", "--extension=.mjs", "--extension=.cjs", "--reporter=lcov", "--reports-dir=" + dir, "--temp-directory=" + filepath.Join(dir, "raw")}
		files, err := project.coverageFiles(env)
		if err != nil {
			return goTestReport{}, err
		}
		labels := make([]string, 0, len(files))
		for _, label := range files {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, label := range labels {
			args = append(args, "--include="+label)
		}
		args = append(args, "npm", "test")
		out, runErr = typescriptRun(args, env)
	} else {
		out, runErr = npmRun([]string{"test"}, env)
	}
	data, readErr := os.ReadFile(reportPath)
	report, parseErr := parseNodeTests(data, project.Root)
	if runErr != nil {
		return report, runErr
	}
	if readErr != nil {
		return report, fmt.Errorf("Node JUnit report missing: %w\n%s", readErr, out)
	}
	if parseErr != nil {
		return report, parseErr
	}
	if report.Tests == 0 || report.Failed > 0 {
		return report, fmt.Errorf("Node test report: tests=%d failed=%d; expected passing tests", report.Tests, report.Failed)
	}
	return report, nil
}

func nodeReportEnv(env []string, report string) []string {
	result := make([]string, 0, len(env)+1)
	options := ""
	for _, value := range env {
		if strings.HasPrefix(value, "NODE_OPTIONS=") {
			options = strings.TrimPrefix(value, "NODE_OPTIONS=")
		} else {
			result = append(result, value)
		}
	}
	// Quote the destination for Node's NODE_OPTIONS parser, including paths with spaces.
	return append(result, "NODE_OPTIONS="+options+" --test-reporter=junit --test-reporter-destination="+strconv.Quote(report))
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
	for _, tooling := range []*config.ToolingConfig{cfg.E2E, cfg.Script} {
		if tooling != nil && filepath.Clean(tooling.Root) != "." {
			project.ToolingRoots = append(project.ToolingRoots, filepath.ToSlash(filepath.Clean(tooling.Root)))
		}
	}
	details := ""
	switch kind {
	case "format":
		if err := requireBiome(); err != nil {
			return fail(err)
		}
		_, err = typescriptRun([]string{typescriptBiome, "format", "--write", "."}, env)
	case "lint":
		if err := requireBiome(); err != nil {
			return fail(err)
		}
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
				return fail(fmt.Errorf("Node coverage report missing: %w", err))
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

func requireBiome() error {
	if _, err := os.Stat("biome.json"); err != nil {
		if _, err := os.Stat("biome.jsonc"); err != nil {
			return fmt.Errorf("TypeScript formatting/lint requires biome.json or biome.jsonc")
		}
	}
	if info, err := os.Stat(typescriptBiome); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("TypeScript formatting/lint requires project-local @biomejs/biome")
	}
	return nil
}
