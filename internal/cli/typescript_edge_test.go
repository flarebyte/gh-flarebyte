// purpose: Cover TypeScript validation, reporting, and build failures that must fail closed.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTypeScriptProjectValidation(t *testing.T) {
	for _, tc := range []struct {
		name, path, data, command, reply, want string
	}{
		{name: "invalid manifest", path: "package.json", data: "{", want: "invalid TypeScript package.json"},
		{name: "missing compiler", path: typescriptCompiler, want: "project-local tool"},
		{name: "invalid compiler output", command: typescriptCompiler, reply: "{", want: "invalid TypeScript project configuration"},
		{name: "references", command: typescriptCompiler, reply: `{"files":["src/math.ts"],"references":[{"path":"../other"}]}`, want: "project references"},
		{name: "empty project", command: typescriptCompiler, reply: `{"files":[]}`, want: "contains no source files"},
		{name: "missing source", command: typescriptCompiler, reply: `{"files":["src/missing.ts"]}`, want: "missing.ts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTypeScript(t, "library")
			stubTypeScript(t)
			if tc.path != "" {
				if tc.data == "" {
					if err := os.Remove(tc.path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(tc.path, []byte(tc.data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			base := runCommandCapture
			runCommandCapture = func(name string, args, env []string) (string, string, error) {
				if len(args) > 0 && args[0] == tc.command {
					return tc.reply, "", nil
				}
				return base(name, args, env)
			}
			if _, err := prepareTypeScript(os.Environ()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
	t.Run("source symlink outside package", func(t *testing.T) {
		setupTypeScript(t, "library")
		stubTypeScript(t)
		outside := filepath.Join(t.TempDir(), "external.ts")
		if err := os.WriteFile(outside, []byte("export {};"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove("src/math.ts"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, "src/math.ts"); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareTypeScript(os.Environ()); err == nil || !strings.Contains(err.Error(), "outside the root package") {
			t.Fatalf("external source accepted: %v", err)
		}
	})
}

func TestTypeScriptCoverageSourceSelection(t *testing.T) {
	for name, want := range map[string]bool{
		"src/code.ts": true, "src/view.tsx": true, "src/module.mts": true, "src/plain.js": true,
		"src/types.d.ts": false, "src/types.d.mts": false, "src/types.d.cts": false,
		"test/a.ts": false, "src/a.test.ts": false, "src/a.spec.ts": false,
		"node_modules/pkg/a.ts": false, "dist/a.js": false, "generated/a.ts": false, "README.md": false,
	} {
		if got := typescriptProductionFile(name); got != want {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
}

func TestTypeScriptJUnitFailureAndTotals(t *testing.T) {
	data := `<testsuites tests="3" failures="1" errors="1" skipped="1"><testsuite file="outer.ts" tests="3" failures="1" errors="1" skipped="1"><testcase name="assertion" classname="nested" file="inner.ts"><failure>bad assertion</failure></testcase><testcase name="runtime"><error>exception</error></testcase><testcase name="pending"><skipped/></testcase></testsuite></testsuites>`
	report, err := parseTypeScriptTests([]byte(data))
	if err != nil || report.Tests != 3 || report.Failed != 2 || report.Skipped != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	if report.Events[0].Test != "inner.ts::nested::assertion" || report.Events[0].Action != "fail" || report.Events[1].Test != "outer.ts::runtime" {
		t.Fatalf("incorrect events: %+v", report.Events)
	}
	for _, invalid := range []string{
		strings.Replace(data, `name="runtime"`, `name=""`, 1),
		strings.Replace(data, `tests="3"`, `tests="4"`, 2),
		strings.Replace(data, `errors="1"`, `errors="0"`, 2),
		strings.Replace(data, ` skipped="1"`, "", 2),
	} {
		if _, err := parseTypeScriptTests([]byte(invalid)); err == nil {
			t.Fatalf("accepted invalid JUnit: %s", invalid)
		}
	}
}

func TestTypeScriptRejectsMalformedCoverage(t *testing.T) {
	root := t.TempDir()
	allowed := map[string]string{filepath.Join(root, "a.ts"): "a.ts"}
	valid := typescriptLCOV("a.ts", 2, 1)
	for name, data := range map[string]string{
		"nested record": "SF:a.ts\n" + valid,
		"orphan totals": "LF:1\n", "negative total": "SF:a.ts\nLF:-1\n", "invalid total": "SF:a.ts\nLH:x\n",
		"duplicate LF": strings.Replace(valid, "LF:2", "LF:2\nLF:2", 1),
		"duplicate LH": strings.Replace(valid, "LH:1", "LH:1\nLH:1", 1),
		"orphan line":  "DA:1,1\n", "incomplete line": "SF:a.ts\nDA:1\n",
		"negative line":       strings.Replace(valid, "DA:1,1", "DA:-1,1", 1),
		"invalid hit count":   strings.Replace(valid, "DA:1,1", "DA:1,no", 1),
		"duplicate line":      strings.Replace(valid, "DA:1,1", "DA:1,1\nDA:1,1", 1),
		"hits exceed lines":   strings.Replace(valid, "LH:1", "LH:3", 1),
		"no executable lines": typescriptLCOV("a.ts", 0, 0),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseTypeScriptCoverage(data, root, allowed); err == nil {
				t.Fatalf("accepted invalid coverage: %s", data)
			}
		})
	}
}

func TestTypeScriptBuildFailureGates(t *testing.T) {
	for _, tc := range []struct {
		name, command, reply, want string
		target                     string
		code                       int
	}{
		{name: "foreign host", command: "--eval", reply: "darwin-arm64", want: "differs from the active Bun host", code: ExitUsage},
		{name: "missing entry", want: "must be a source file", code: ExitBuildFailure},
		{name: "compiler fails", command: "build", want: "compile failed", code: ExitBuildFailure},
		{name: "compiler produces no executable", command: "build", reply: "no output", want: "did not produce", code: ExitBuildFailure},
		{name: "tests fail", command: "test", want: "compile failed", code: ExitBuildFailure},
		{name: "unsupported target", target: "windows-amd64", want: "unsupported TypeScript target", code: ExitUsage},
		{name: "unknown target", target: "darwin-amd64", want: "not present in build.targets", code: ExitUsage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTypeScript(t, "binary")
			calls := stubTypeScript(t)
			if tc.name == "tests fail" {
				data, err := os.ReadFile(".gh-flarebyte.cue")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(".gh-flarebyte.cue", bytes.Replace(data, []byte("build: {"), []byte("build: {\n runTests: true"), 1), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name == "missing entry" {
				if err := os.Remove("src/cli.ts"); err != nil {
					t.Fatal(err)
				}
				// Keep the compiler's resolved source list valid so entry validation runs.
			}
			base := runCommandCapture
			runCommandCapture = func(name string, args, env []string) (string, string, error) {
				if tc.name == "missing entry" && args[0] == typescriptCompiler && contains(args, "--showConfig") {
					return `{"files":["src/math.ts"]}`, "", nil
				}
				command := args[0]
				if name == "bun" {
					command = args[1]
				}
				if command == tc.command {
					if tc.reply == "" {
						return "", "compile failed", fmt.Errorf("tool failed")
					}
					return tc.reply, "", nil
				}
				return base(name, args, env)
			}
			if tc.name == "unsupported target" {
				data, err := os.ReadFile(".gh-flarebyte.cue")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(".gh-flarebyte.cue", bytes.ReplaceAll(data, []byte("linux-amd64"), []byte(tc.target)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"build"}
			if tc.target != "" {
				args = append(args, "--target", tc.target)
			}
			var out, diagnostics bytes.Buffer
			result := Run(args, &out, &diagnostics)
			if result.ExitCode != tc.code || !strings.Contains(diagnostics.String(), tc.want) {
				t.Fatalf("%+v %s", result, &diagnostics)
			}
			if _, err := os.Stat("build/checksums.txt"); !os.IsNotExist(err) {
				t.Fatal("failed build packaged assets")
			}
			for _, call := range *calls {
				if call[0] == "build" {
					t.Fatal("build proceeded after a validation failure")
				}
			}
		})
	}
}

func TestTypeScriptTestReportGates(t *testing.T) {
	for _, tc := range []struct{ name, data, want string }{
		{name: "missing report", want: "JUnit report missing"},
		{name: "malformed report", data: "<", want: "invalid Node JUnit report"},
		{name: "zero tests", data: `<testsuite tests="0" failures="0" skipped="0"/>`, want: "expected passing tests"},
		{name: "failure despite successful exit", data: `<testsuite tests="1" failures="1" skipped="0"><testcase name="failed"><failure/></testcase></testsuite>`, want: "expected passing tests"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTypeScript(t, "library")
			stubTypeScript(t)
			project, err := prepareTypeScript(os.Environ())
			if err != nil {
				t.Fatal(err)
			}
			runCommandCapture = func(name string, args, env []string) (string, string, error) {
				if tc.data != "" {
					for _, value := range env {
						if strings.HasPrefix(value, "NODE_OPTIONS=") {
							re := regexp.MustCompile(`--test-reporter-destination="([^"]+)"`)
							match := re.FindStringSubmatch(value)
							if len(match) > 1 {
								if err := os.WriteFile(match[1], []byte(tc.data), 0o600); err != nil {
									t.Fatal(err)
								}
							}
						}
					}
				}
				return "", "", nil
			}
			if _, err := typescriptTest(project, os.Environ(), false, t.TempDir()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}
