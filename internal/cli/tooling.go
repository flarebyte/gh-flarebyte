// purpose: Execute Bun/Python scripts and e2e suites independently of the build language.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
	"github.com/spf13/cobra"
)

// Working directories belong to each child process; never change the parent cwd.
var runToolingCapture = func(name string, args, env []string, dir string) (string, string, error) {
	cmd, err := newAllowedDevCommand(name, args)
	if err != nil {
		return "", "", err
	}
	cmd.Dir, cmd.Env = dir, env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

func newToolingCommand(kind string, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{Use: kind, Short: "Run configured " + kind + " using Bun or Python 3", Args: noExtraArgs}
	if kind == "script" {
		cmd.Use = "script <file> -- [args...]"
		cmd.Args = cobra.MinimumNArgs(1)
	}
	cmd.RunE = func(_ *cobra.Command, args []string) error {
		return resultToError(runTooling(kind, args, stdout, stderr))
	}
	return cmd
}

func containedPath(root, path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be relative: %s", path)
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(base, path))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes configured root: %s", path)
	}
	return resolved, nil
}

func regularToolingFile(root, name string) error {
	path, err := containedPath(root, name)
	if err != nil {
		return fmt.Errorf("requires %s: %w", name, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("requires regular file %s", name)
	}
	return nil
}

func prepareTooling(repo string, cfg *config.ToolingConfig, env []string) (string, error) {
	if cfg.Dependencies == nil {
		return "", nil
	}
	deps := cfg.Dependencies
	root, err := containedPath(repo, deps.Root)
	if err != nil {
		return "", err
	}
	var files, args []string
	switch deps.Manager {
	case "bun":
		files = []string{"package.json", "bun.lock"}
		args = []string{"install", "--frozen-lockfile", "--ignore-scripts"}
	case "npm":
		files = []string{"package.json", "package-lock.json"}
		args = []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}
	case "uv":
		files = []string{"pyproject.toml", "uv.lock"}
		args = []string{"sync", "--locked"}
	}
	for _, name := range files {
		if err := regularToolingFile(root, name); err != nil {
			return "", err
		}
	}
	out, diagnostics, err := runToolingCapture(deps.Manager, args, env, root)
	if err != nil {
		return "", fmt.Errorf("%s dependency preparation failed: %w\n%s", deps.Manager, err, strings.TrimSpace(out+"\n"+diagnostics))
	}
	return root, nil
}

const pythonE2ERunner = `import sys, unittest
suite = unittest.defaultTestLoader.discover(sys.argv[1], pattern="test_*.py")
result = unittest.TextTestRunner(verbosity=2).run(suite)
if result.testsRun == 0:
    print("No e2e tests found", file=sys.stderr)
sys.exit(0 if result.testsRun > 0 and result.wasSuccessful() else 1)
`

func runTooling(kind string, args []string, stdout, stderr io.Writer) Result {
	fail := func(code int, err error) Result {
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: code, Err: err}
	}
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	suite := cfg.E2E
	if kind == "script" {
		suite = cfg.Script
	}
	if suite == nil {
		return fail(ExitUsage, fmt.Errorf("configure the optional %s section in .gh-flarebyte.cue first", kind))
	}
	repo, err := os.Getwd()
	if err != nil {
		return fail(ExitFailure, err)
	}
	root, err := containedPath(repo, suite.Root)
	if err != nil {
		return fail(ExitUsage, err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return fail(ExitUsage, fmt.Errorf("%s.root must be an existing directory", kind))
	}
	var entry string
	if kind == "script" {
		entry, err = containedPath(root, args[0])
		if err != nil {
			return fail(ExitUsage, err)
		}
		ext := filepath.Ext(entry)
		if (suite.Runtime == "bun" && ext != ".ts" && ext != ".mts" && ext != ".cts" && ext != ".tsx") || (suite.Runtime == "python3" && ext != ".py") {
			return fail(ExitUsage, fmt.Errorf("script extension does not match runtime %s", suite.Runtime))
		}
		if err := regularToolingFile(root, args[0]); err != nil {
			return fail(ExitUsage, err)
		}
	}
	env := buildCommandEnv(cfg, stderr)
	depRoot, err := prepareTooling(repo, suite, env)
	if err != nil {
		return fail(ExitFailure, err)
	}
	name := suite.Runtime
	var command []string
	reportPath := ""
	if kind == "script" {
		command = append([]string{entry}, args[1:]...)
		if name == "bun" {
			command = append([]string{"--no-install", "run"}, command...)
		}
	} else if name == "bun" {
		dir, err := os.MkdirTemp("", "gh-flarebyte-e2e-")
		if err != nil {
			return fail(ExitFailure, err)
		}
		defer func() { _ = os.RemoveAll(dir) }()
		reportPath = filepath.Join(dir, "junit.xml")
		// ./ prevents Bun interpreting the folder as a test-name filter.
		rel, _ := filepath.Rel(repo, root)
		command = []string{"--no-install", "test", "./" + filepath.ToSlash(rel), "--reporter=junit", "--reporter-outfile=" + reportPath}
	} else {
		command = []string{"-c", pythonE2ERunner, root}
	}
	if name == "python3" && depRoot != "" {
		command = append([]string{"run", "--project", depRoot, "--no-sync", "python3"}, command...)
		name = "uv"
	}
	out, diagnostics, err := runToolingCapture(name, command, env, repo)
	_, _ = io.WriteString(stdout, out)
	_, _ = io.WriteString(stderr, diagnostics)
	if err != nil {
		code := ExitFailure
		if status, ok := err.(interface{ ExitCode() int }); ok && status.ExitCode() > 0 {
			code = status.ExitCode()
		}
		return fail(code, fmt.Errorf("%s failed: %w", kind, err))
	}
	if reportPath != "" {
		data, err := os.ReadFile(reportPath)
		if err != nil {
			return fail(ExitFailure, fmt.Errorf("e2e JUnit report missing: %w", err))
		}
		report, err := parseTypeScriptTests(data)
		if err != nil {
			return fail(ExitFailure, err)
		}
		if report.Tests == 0 || report.Failed > 0 {
			return fail(ExitFailure, fmt.Errorf("e2e suite requires passing tests; tests=%d failed=%d", report.Tests, report.Failed))
		}
	}
	return Result{ExitCode: ExitOK}
}
