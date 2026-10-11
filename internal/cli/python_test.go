package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

const pythonJUnit = `<testsuites><testsuite><testcase classname="tests.test_math" name="test_add"/><testcase name="test_skip"><skipped/></testcase></testsuite></testsuites>`
const pythonCoverage = `{"totals":{"percent_covered":75},"files":{"src/z.py":{"summary":{"percent_covered":50}},"src/a.py":{"summary":{"percent_covered":100}}}}`

func pythonConfigCue() string {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, "language:     \"python\"\n\tmode: \"package\"", 1)
	cfg = strings.Replace(cfg, "targets: [\n\t\t\"linux-amd64\",\n\t]", "", 1)
	// Keep plain-text assertions independent of the caller's terminal color support.
	return strings.Replace(cfg, "main.project.yaml", "pyproject.toml", 1) + `

devOutput: { color: "false" }
`
}

func setupPython(t *testing.T, cfg string) {
	t.Helper()
	setupTempWorkdirWithConfig(t, cfg)
	for name, text := range map[string]string{
		"pyproject.toml": "[project]\nname = 'python-demo'\nversion = '1.2.3'\nrequires-python = '>=3.10'\n",
		"uv.lock":        "version = 1\n",
	} {
		if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func stubPython(t *testing.T) *[][]string {
	t.Helper()
	old := runCommandCapture
	t.Cleanup(func() { runCommandCapture = old })
	var calls [][]string
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		if name != "uv" {
			t.Fatalf("unexpected tool: %s", name)
		}
		calls = append(calls, append([]string(nil), args...))
		if args[0] == "run" && args[1] != "--locked" {
			t.Fatal("unlocked uv run")
		}
		for i, arg := range args {
			if strings.HasPrefix(arg, "--junit-xml=") {
				if err := os.WriteFile(strings.TrimPrefix(arg, "--junit-xml="), []byte(pythonJUnit), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if arg == "-o" {
				if envValue(env, "COVERAGE_FILE") == "" {
					t.Fatal("coverage data is not isolated")
				}
				if err := os.WriteFile(args[i+1], []byte(pythonCoverage), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if arg == "--out-dir" {
				for _, name := range []string{"python_demo-1.2.3-py3-none-any.whl", "python_demo-1.2.3.tar.gz"} {
					if err := os.WriteFile(filepath.Join(args[i+1], name), []byte(name), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		return "", "", nil
	}
	return &calls
}

func TestPythonDevCommands(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		call string
	}{
		{[]string{"test", "--style", "per_test"}, "✓ tests.test_math::test_add", "python -m pytest"},
		{[]string{"test", "--failed-only"}, "", "python -m pytest"},
		{[]string{"format"}, "FORMAT PASS", "ruff format ."},
		{[]string{"lint"}, "LINT PASS", "ruff check ."},
		{[]string{"cov", "--min", "70"}, "total=75.00% min=70.00%", "coverage json --fail-under=0"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			setupPython(t, pythonConfigCue())
			calls := stubPython(t)
			var out, errOut bytes.Buffer
			result := Run(tc.args, &out, &errOut)
			if result.ExitCode != ExitOK || !strings.Contains(out.String(), tc.want) {
				t.Fatalf("%+v stdout=%s stderr=%s", result, &out, &errOut)
			}
			if tc.want == "" && out.Len() != 0 {
				t.Fatalf("failed-only output: %s", &out)
			}
			found := false
			for _, call := range *calls {
				found = found || strings.Contains(strings.Join(call, " "), tc.call)
			}
			if !found {
				t.Fatalf("missing call %s in %v", tc.call, *calls)
			}
		})
	}
}

func TestPythonFailures(t *testing.T) {
	for _, command := range []string{"test", "format", "lint", "cov", "build"} {
		t.Run(command, func(t *testing.T) {
			setupPython(t, pythonConfigCue())
			old := runCommandCapture
			t.Cleanup(func() { runCommandCapture = old })
			runCommandCapture = func(string, []string, []string) (string, string, error) {
				return "stdout diagnostic", "stderr diagnostic", fmt.Errorf("missing uv / stale lock / Python 3 required")
			}
			var out, errOut bytes.Buffer
			result := Run([]string{command}, &out, &errOut)
			if result.ExitCode == ExitOK || !strings.Contains(errOut.String(), "stdout diagnostic") || !strings.Contains(errOut.String(), "stderr diagnostic") {
				t.Fatalf("%+v %s", result, &errOut)
			}
		})
	}
}

func TestPythonCoverageThreshold(t *testing.T) {
	for _, enforce := range []bool{true, false} {
		t.Run(fmt.Sprint(enforce), func(t *testing.T) {
			setupPython(t, pythonConfigCue()+fmt.Sprintf("\ncoverage: { min: 80, enforceMin: %t }\n", enforce))
			stubPython(t)
			var out, errOut bytes.Buffer
			result := Run([]string{"cov"}, &out, &errOut)
			if (result.ExitCode != ExitOK) != enforce {
				t.Fatalf("%+v %s", result, &errOut)
			}
		})
	}
}

func TestPythonReports(t *testing.T) {
	report, err := parsePythonTests([]byte(`<testsuite><testcase name="ok"/><testcase name="bad"><failure>assertion</failure></testcase><testcase name="setup"><error/></testcase><testcase name="skip"><skipped/></testcase></testsuite>`))
	if err != nil || report.Tests != 4 || report.Failed != 2 || report.Skipped != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	for _, text := range []string{"", "<testsuite>", "<wrong/>", "<testsuites><testcase"} {
		if _, err := parsePythonTests([]byte(text)); err == nil {
			t.Fatalf("accepted invalid XML %q", text)
		}
	}
	percent, details, err := parsePythonCoverage([]byte(pythonCoverage))
	if err != nil || percent != 75 || len(details) != 2 || details[0].Label != "src/a.py" {
		t.Fatalf("%v %v %v", percent, details, err)
	}
	for _, text := range []string{"{", "{}", `{"totals":{"percent_covered":101}}`, `{"totals":{"percent_covered":50},"files":{"a":{"summary":{}}}}`} {
		if _, _, err := parsePythonCoverage([]byte(text)); err == nil {
			t.Fatalf("accepted invalid coverage %s", text)
		}
	}
}

func TestPythonProjectVersion(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"[project]\nversion = '1.2.3' # comment\n[tool.other]\nversion='9.9.9'", "1.2.3"},
		{"[project]\nversion='1.2.3'\ndynamic=['version']", ""},
		{"[tool.other]\nversion='1.2.3'", ""},
		{"[project]\nversion='bad'", ""},
		{"[project\n", ""},
	} {
		version, err := pythonProjectVersion([]byte(tc.text))
		if version != tc.want || (err != nil) != (tc.want == "") {
			t.Fatalf("%q: %q %v", tc.text, version, err)
		}
	}
}

func TestPythonBuildAndRelease(t *testing.T) {
	setupPython(t, pythonConfigCue())
	stubPython(t)
	var out, errOut bytes.Buffer
	result := Run([]string{"build", "--output-dir", "output with spaces"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("%+v %s", result, &errOut)
	}
	data, err := os.ReadFile("output with spaces/checksums.txt")
	if err != nil || strings.Count(string(data), "\n") != 2 {
		t.Fatalf("checksums: %s %v", data, err)
	}
	if err := os.MkdirAll("build", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("build/stale-0.0.1.whl", []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTag, oldCreate := tagExists, createRelease
	t.Cleanup(func() { tagExists, createRelease = oldTag, oldCreate })
	tagExists = func(string) (bool, error) { return false, nil }
	var uploaded []string
	createRelease = func(tag string, artifacts []string, notesMode, notesFile string, draft bool) error {
		if tag != "v1.2.3" || !draft {
			t.Fatalf("tag=%s draft=%v", tag, draft)
		}
		uploaded = artifacts
		return nil
	}
	result = Run([]string{"release", "--draft"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("%+v %s", result, &errOut)
	}
	want := []string{"build/python_demo-1.2.3-py3-none-any.whl", "build/python_demo-1.2.3.tar.gz", "build/checksums.txt"}
	if !reflect.DeepEqual(uploaded, want) {
		t.Fatalf("uploaded=%v", uploaded)
	}
}

func TestPythonConfigValidation(t *testing.T) {
	setupPython(t, pythonConfigCue())
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"binary", "library"} {
		cfg.Build.Mode = mode
		if err := config.Validate(cfg); err == nil {
			t.Fatalf("accepted mode %s", mode)
		}
	}
	cfg.Build.Mode = "package"
	cfg.Build.Targets = []string{"linux-amd64"}
	if err := config.Validate(cfg); err == nil {
		t.Fatal("accepted target matrix")
	}
	cfg.Build.Targets = nil
	cfg.Build.Language = "go"
	if err := config.Validate(cfg); err == nil {
		t.Fatal("accepted Go package mode")
	}
}

func TestPythonMissingProjectFiles(t *testing.T) {
	for _, name := range []string{"pyproject.toml", "uv.lock"} {
		t.Run(name, func(t *testing.T) {
			setupPython(t, pythonConfigCue())
			if err := os.Remove(name); err != nil {
				t.Fatal(err)
			}
			if err := preparePython(nil); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestPythonTestReportFailures(t *testing.T) {
	for _, report := range []string{"", "<testsuites/>", "<testsuites>", `<testsuite><testcase name="bad"><error/></testcase></testsuite>`} {
		t.Run(report, func(t *testing.T) {
			setupPython(t, pythonConfigCue())
			stubPython(t)
			delegate := runCommandCapture
			runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
				out, errText, err := delegate(name, args, env)
				for _, arg := range args {
					if strings.HasPrefix(arg, "--junit-xml=") {
						if err := os.WriteFile(strings.TrimPrefix(arg, "--junit-xml="), []byte(report), 0o644); err != nil {
							t.Fatal(err)
						}
					}
				}
				return out, errText, err
			}
			var out, errOut bytes.Buffer
			if result := Run([]string{"test"}, &out, &errOut); result.ExitCode == ExitOK || strings.Contains(out.String(), "TEST PASS") {
				t.Fatalf("%+v %s %s", result, &out, &errOut)
			}
		})
	}
}

func TestPythonBuildStopsOnTestFailure(t *testing.T) {
	setupPython(t, strings.Replace(pythonConfigCue(), `mode: "package"`, "mode: \"package\"\n runTests: true", 1))
	calls := stubPython(t)
	delegate := runCommandCapture
	runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
		out, errText, err := delegate(name, args, env)
		if strings.Contains(strings.Join(args, " "), "-m pytest") {
			return "test failed", "", fmt.Errorf("pytest exit 1")
		}
		return out, errText, err
	}
	var out, errOut bytes.Buffer
	if result := Run([]string{"build"}, &out, &errOut); result.ExitCode != ExitBuildFailure {
		t.Fatalf("%+v %s", result, &errOut)
	}
	for _, call := range *calls {
		if call[0] == "build" {
			t.Fatal("packaged after tests failed")
		}
	}
}

func TestPythonReleaseStaging(t *testing.T) {
	setupPython(t, pythonConfigCue())
	stubPython(t)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Build.ChecksumFile = "manifests/custom.sha256"
	built, err := buildPythonPackage(cfg, "", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cfg.Build.ChecksumFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range built[:len(built)-1] {
		sum, err := sha256File(artifact)
		if err != nil || !strings.Contains(string(data), sum+"  "+filepath.Base(artifact)) {
			t.Fatalf("incorrect checksum for %s", artifact)
		}
	}
	for _, checksums := range []bool{true, false} {
		assets, err := stagePythonRelease(built, "release assets", checksums)
		if err != nil || len(assets) != 2+boolInt(checksums) {
			t.Fatalf("%v %v", assets, err)
		}
		for _, asset := range assets {
			if _, err := os.Stat(asset); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := stagePythonRelease([]string{"a/same.whl", "b/same.whl", "checksums.txt"}, "release assets", true); err == nil {
		t.Fatal("accepted duplicate staged filenames")
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
