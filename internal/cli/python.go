// purpose: Run Python 3 project workflows through uv using shared configuration and output policies.
// responsibilities: Check the project and interpreter; execute tools; build fresh package artifacts.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/config"
	"github.com/pelletier/go-toml/v2"
)

func uvRun(args []string, env []string) (string, error) {
	out, errText, err := runCommandCapture("uv", append([]string{"run", "--locked"}, args...), env)
	if err != nil {
		return out, fmt.Errorf("uv run failed (ensure uv, Python 3, and the required dev tools are installed): %w\n%s", err, strings.TrimSpace(out+"\n"+errText))
	}
	return out, nil
}

func preparePython(env []string) error {
	for _, name := range []string{"pyproject.toml", "uv.lock"} {
		info, err := os.Stat(name)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("Python requires %s in the project directory; initialize the uv project and run uv lock", name)
		}
	}
	data, err := os.ReadFile("pyproject.toml")
	if err != nil {
		return err
	}
	var metadata struct {
		Project struct {
			RequiresPython string `toml:"requires-python"`
		} `toml:"project"`
	}
	if err := toml.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("invalid pyproject.toml: %w", err)
	}
	if metadata.Project.RequiresPython == "" {
		return fmt.Errorf("pyproject.toml requires [project].requires-python declaring Python 3 support")
	}
	_, err = uvRun([]string{"python", "-c", "import sys; sys.exit(0 if sys.version_info[0] == 3 else 'Python 3 is required')"}, env)
	return err
}

func pythonFailure(cfg config.Config, kind string, start time.Time, err error, stderr io.Writer) Result {
	printDevSummary(stderr, cfg, devSummary{Kind: kind, Status: "FAIL", Duration: time.Since(start), Details: err.Error()})
	return Result{ExitCode: ExitFailure, Err: err}
}

func runPythonTool(cfg config.Config, env []string, kind string, args []string, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	if err := preparePython(env); err != nil {
		return pythonFailure(cfg, kind, start, err, stderr)
	}
	if _, err := uvRun(args, env); err != nil {
		return pythonFailure(cfg, kind, start, err, stderr)
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: kind, Status: "PASS", Duration: time.Since(start)})
	}
	return Result{ExitCode: ExitOK}
}

func runPythonTest(cfg config.Config, env []string, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	if err := preparePython(env); err != nil {
		return pythonFailure(cfg, "test", start, err, stderr)
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-python-test-")
	if err != nil {
		return pythonFailure(cfg, "test", start, err, stderr)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	reportPath := filepath.Join(dir, "junit.xml")
	_, runErr := uvRun([]string{"python", "-m", "pytest", "--junit-xml=" + reportPath}, env)
	data, readErr := os.ReadFile(reportPath)
	report, parseErr := parsePythonTests(data)
	if cfg.DevOutput.Style == "per_test" && readErr == nil && parseErr == nil {
		printPerTestEvents(stdout, cfg, report.Events, failedOnly)
	}
	if runErr != nil {
		if readErr == nil && parseErr == nil {
			runErr = fmt.Errorf("tests=%d failed=%d skipped=%d\n%w", report.Tests, report.Failed, report.Skipped, runErr)
		}
		return pythonFailure(cfg, "test", start, runErr, stderr)
	}
	if readErr != nil {
		return pythonFailure(cfg, "test", start, fmt.Errorf("pytest report missing: %w", readErr), stderr)
	}
	if parseErr != nil {
		return pythonFailure(cfg, "test", start, parseErr, stderr)
	}
	if report.Tests == 0 || report.Failed != 0 {
		return pythonFailure(cfg, "test", start, fmt.Errorf("pytest report: tests=%d failed=%d; expected passing tests", report.Tests, report.Failed), stderr)
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: "test", Status: "PASS", Duration: time.Since(start), Details: formatGoTestDetails(report.Tests, report.Failed, report.Skipped, "PASS")})
	}
	return Result{ExitCode: ExitOK}
}

func runPythonCov(cfg config.Config, env []string, min *float64, failedOnly bool, stdout, stderr io.Writer) Result {
	start := time.Now()
	if err := preparePython(env); err != nil {
		return pythonFailure(cfg, "cov", start, err, stderr)
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-python-cov-")
	if err != nil {
		return pythonFailure(cfg, "cov", start, err, stderr)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	env = append(env, "COVERAGE_FILE="+filepath.Join(dir, ".coverage"))
	if _, err := uvRun([]string{"python", "-m", "coverage", "run", "-m", "pytest"}, env); err != nil {
		return pythonFailure(cfg, "cov", start, err, stderr)
	}
	reportPath := filepath.Join(dir, "coverage.json")
	// Threshold enforcement belongs to gh-flarebyte, including --min overrides.
	if _, err := uvRun([]string{"python", "-m", "coverage", "json", "--fail-under=0", "-o", reportPath}, env); err != nil {
		return pythonFailure(cfg, "cov", start, err, stderr)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return pythonFailure(cfg, "cov", start, fmt.Errorf("coverage report missing: %w", err), stderr)
	}
	percent, details, err := parsePythonCoverage(data)
	if err != nil {
		return pythonFailure(cfg, "cov", start, err, stderr)
	}
	effectiveMin := resolveCoverageMin(min, cfg)
	if cfg.DevOutput.Style == "per_test" {
		printCoverageDetails(stdout, details, effectiveMin, failedOnly)
	}
	text := fmt.Sprintf("total=%.2f%%", percent)
	if effectiveMin != nil {
		text += fmt.Sprintf(" min=%.2f%%", *effectiveMin)
		if cfg.Coverage.FailBelowMin && percent < *effectiveMin {
			return pythonFailure(cfg, "cov", start, fmt.Errorf("%s: coverage below minimum", text), stderr)
		}
	}
	if !failedOnly {
		printDevSummary(stdout, cfg, devSummary{Kind: "cov", Status: "PASS", Duration: time.Since(start), Details: text})
	}
	return Result{ExitCode: ExitOK}
}

// Only artifacts produced in this temporary build directory are copied and released.
func buildPythonPackage(cfg config.Config, outputOverride string, stderr io.Writer) ([]string, error) {
	env := buildCommandEnv(cfg, stderr)
	if err := preparePython(env); err != nil {
		return nil, err
	}
	if cfg.Build.RunTests {
		result := runPythonTest(cfg, env, false, io.Discard, io.Discard)
		if result.ExitCode != ExitOK {
			return nil, result.Err
		}
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-python-build-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	out, errText, err := runCommandCapture("uv", []string{"build", "--no-sources", "--out-dir", dir}, env)
	if err != nil {
		return nil, fmt.Errorf("uv build failed: %w\n%s", err, strings.TrimSpace(out+"\n"+errText))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	outputDir := cfg.Build.OutputDir
	if outputOverride != "" {
		outputDir = outputOverride
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}
	var artifacts []string
	var checksums strings.Builder
	wheel, sdist := false, false
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !(strings.HasSuffix(name, ".whl") || strings.HasSuffix(name, ".tar.gz")) {
			continue
		}
		wheel = wheel || strings.HasSuffix(name, ".whl")
		sdist = sdist || strings.HasSuffix(name, ".tar.gz")
		artifact := filepath.Join(outputDir, name)
		if err := copyFile(filepath.Join(dir, name), artifact, 0o644); err != nil {
			return nil, err
		}
		sum, err := sha256File(artifact)
		if err != nil {
			return nil, err
		}
		_, _ = fmt.Fprintf(&checksums, "%s  %s\n", sum, name)
		artifacts = append(artifacts, artifact)
	}
	if !wheel || !sdist {
		return nil, fmt.Errorf("uv build did not produce both a wheel and source distribution")
	}
	checksumPath := resolveChecksumPath(cfg.Build.ChecksumFile, outputOverride)
	checksumAbs, err := filepath.Abs(checksumPath)
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		artifactAbs, err := filepath.Abs(artifact)
		if err != nil {
			return nil, err
		}
		if artifactAbs == checksumAbs {
			return nil, fmt.Errorf("build.checksumFile conflicts with a package artifact")
		}
	}
	if err := os.MkdirAll(filepath.Dir(checksumPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(checksumPath, []byte(checksums.String()), 0o644); err != nil {
		return nil, err
	}
	return append(artifacts, checksumPath), nil
}
