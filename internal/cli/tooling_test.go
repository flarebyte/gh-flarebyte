package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupTooling(t *testing.T, extra string) string {
	t.Helper()
	root := setupTempWorkdirWithConfig(t, rustConfigCue("library")+"\n"+extra+"\n")
	for _, dir := range []string{"e2e", "script", "checks"} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestToolingExecution(t *testing.T) {
	for _, runtime := range []string{"bun", "python3"} {
		for _, isolated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", runtime, isolated), func(t *testing.T) {
				manager := "npm"
				if runtime == "python3" {
					manager = "uv"
				}
				deps := "."
				if isolated {
					deps = "checks"
				}
				root := setupTooling(t, fmt.Sprintf(`e2e: {runtime: %q, root: "checks", dependencies: {manager: %q, root: %q}}`, runtime, manager, deps))
				files := []string{"package.json", "package-lock.json"}
				if manager == "uv" {
					files = []string{"pyproject.toml", "uv.lock"}
				}
				for _, file := range files {
					if err := os.WriteFile(filepath.Join(deps, file), []byte("locked"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				old := runToolingCapture
				t.Cleanup(func() { runToolingCapture = old })
				calls := 0
				runToolingCapture = func(name string, args, env []string, dir string) (string, string, error) {
					calls++
					if calls == 1 {
						if name != manager || dir != filepath.Join(root, deps) {
							t.Fatalf("wrong install: %s %v in %s", name, args, dir)
						}
						return "", "", nil
					}
					if dir != root {
						t.Fatalf("execution cwd %s", dir)
					}
					if runtime == "bun" {
						if name != "bun" || !contains(args, "./checks") || !contains(args, "--no-install") {
							t.Fatalf("unexpected Bun invocation %s %v", name, args)
						}
						for _, arg := range args {
							if strings.HasPrefix(arg, "--reporter-outfile=") {
								if err := os.WriteFile(strings.TrimPrefix(arg, "--reporter-outfile="), []byte(typescriptJUnitFixture), 0o600); err != nil {
									t.Fatal(err)
								}
							}
						}
					} else {
						if name != "uv" || !contains(args, "--no-sync") || !contains(args, filepath.Join(root, "checks")) {
							t.Fatalf("unexpected Python invocation %s %v", name, args)
						}
					}
					return "test output\n", "", nil
				}
				var out, diagnostics bytes.Buffer
				result := Run([]string{"e2e"}, &out, &diagnostics)
				if result.ExitCode != 0 || calls != 2 || !strings.Contains(out.String(), "test output") {
					t.Fatalf("%+v calls=%d %s", result, calls, &diagnostics)
				}
			})
		}
	}
}

func TestScriptArgumentsAndFailure(t *testing.T) {
	setupTooling(t, `script: {runtime: "python3"}`)
	if err := os.WriteFile("script/example.py", []byte("pass"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := runToolingCapture
	t.Cleanup(func() { runToolingCapture = old })
	runToolingCapture = func(name string, args, env []string, dir string) (string, string, error) {
		if name != "python3" || len(args) != 3 || args[1] != "--flag" || args[2] != "literal $(value)" {
			t.Fatalf("arguments changed: %s %v", name, args)
		}
		return "", "deliberate failure", fmt.Errorf("failed")
	}
	var out, diagnostics bytes.Buffer
	result := Run([]string{"script", "example.py", "--", "--flag", "literal $(value)"}, &out, &diagnostics)
	if result.ExitCode == 0 || !strings.Contains(diagnostics.String(), "deliberate failure") {
		t.Fatalf("%+v %s", result, &diagnostics)
	}
}

func TestToolingRejectsInvalidPathsAndReports(t *testing.T) {
	for _, mode := range []string{"missing config", "escape", "symlink", "wrong extension", "zero tests", "missing report"} {
		t.Run(mode, func(t *testing.T) {
			extra := `script: {runtime: "bun"}`
			if mode == "missing config" {
				extra = ""
			}
			if mode == "zero tests" || mode == "missing report" {
				extra = `e2e: {runtime: "bun"}`
			}
			setupTooling(t, extra)
			old := runToolingCapture
			t.Cleanup(func() { runToolingCapture = old })
			called := false
			runToolingCapture = func(name string, args, env []string, dir string) (string, string, error) {
				called = true
				if mode != "zero tests" && mode != "missing report" {
					t.Fatal("invalid path executed")
				}
				if mode == "zero tests" {
					for _, arg := range args {
						if strings.HasPrefix(arg, "--reporter-outfile=") {
							if err := os.WriteFile(strings.TrimPrefix(arg, "--reporter-outfile="), []byte(`<testsuite tests="0" failures="0" skipped="0"/>`), 0o600); err != nil {
								t.Fatal(err)
							}
						}
					}
				}
				return "", "", nil
			}
			file := "bad.py"
			if mode == "escape" {
				file = "../outside.ts"
			}
			if mode == "symlink" {
				file = "linked.ts"
				outside := filepath.Join(t.TempDir(), "outside.ts")
				if err := os.WriteFile(outside, []byte(""), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, "script/linked.ts"); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"script", file}
			if mode == "zero tests" || mode == "missing report" {
				args = []string{"e2e"}
			}
			var out, diagnostics bytes.Buffer
			result := Run(args, &out, &diagnostics)
			if result.ExitCode == 0 {
				t.Fatalf("accepted %s: %s", mode, &diagnostics)
			}
			if called != (mode == "zero tests" || mode == "missing report") {
				t.Fatalf("unexpected execution %t", called)
			}
		})
	}
}

func TestPythonE2ERealDiscovery(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	setupTooling(t, `e2e: {runtime: "python3"}`)
	for _, data := range []string{"", "import unittest\nclass Check(unittest.TestCase):\n def test_ok(self): self.assertEqual(1,1)\n", "import unittest\nclass Check(unittest.TestCase):\n def test_bad(self): self.assertEqual(1,2)\n"} {
		if err := os.WriteFile("e2e/test_example.py", []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, diagnostics bytes.Buffer
		result := Run([]string{"e2e"}, &out, &diagnostics)
		passing := strings.Contains(data, "test_ok")
		if (result.ExitCode == 0) != passing {
			t.Fatalf("%+v %s", result, &diagnostics)
		}
	}
}

func TestBunToolingRealExecution(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is not installed")
	}
	setupTooling(t, `e2e: {runtime: "bun"}
script: {runtime: "bun"}`)
	if err := os.WriteFile("e2e/example.test.ts", []byte("import {test,expect} from 'bun:test'; test('works',()=>expect(1+1).toBe(2));"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("script/example.ts", []byte("console.log(Bun.argv.slice(2).join('|')); process.exit(9);"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if result := Run([]string{"e2e"}, &out, &diagnostics); result.ExitCode != 0 {
		t.Fatalf("%+v %s", result, &diagnostics)
	}
	out.Reset()
	diagnostics.Reset()
	result := Run([]string{"script", "example.ts", "--", "--flag", "hello world"}, &out, &diagnostics)
	if result.ExitCode != 9 || !strings.Contains(out.String(), "--flag|hello world") {
		t.Fatalf("%+v %s %s", result, &out, &diagnostics)
	}
}

func TestBunDependenciesFailBeforeExecution(t *testing.T) {
	setupTooling(t, `e2e: {runtime: "bun", dependencies: {root: "e2e"}}`)
	old := runToolingCapture
	t.Cleanup(func() { runToolingCapture = old })
	calls := 0
	runToolingCapture = func(name string, args, env []string, dir string) (string, string, error) {
		calls++
		if name != "bun" || !contains(args, "--frozen-lockfile") || !contains(args, "--ignore-scripts") {
			t.Fatalf("unexpected install %s %v", name, args)
		}
		return "", "stale lockfile", fmt.Errorf("install failed")
	}
	var out, diagnostics bytes.Buffer
	if result := Run([]string{"e2e"}, &out, &diagnostics); result.ExitCode == 0 || calls != 0 {
		t.Fatalf("missing lockfile installed dependencies: %+v calls=%d", result, calls)
	}
	for _, name := range []string{"package.json", "bun.lock"} {
		if err := os.WriteFile(filepath.Join("e2e", name), []byte("locked"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	diagnostics.Reset()
	if result := Run([]string{"e2e"}, &out, &diagnostics); result.ExitCode == 0 || calls != 1 || !strings.Contains(diagnostics.String(), "stale lockfile") {
		t.Fatalf("%+v calls=%d %s", result, calls, &diagnostics)
	}
}
