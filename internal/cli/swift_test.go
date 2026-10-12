// purpose: Verify SwiftPM command contracts, complete test reports, scoped coverage, and fresh releases.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

const swiftXCTest = "Test Case 'MathTests.testAdds' passed (0.001 seconds).\nTest Case 'MathTests.testSkipped' skipped (0.001 seconds).\nExecuted 2 tests, with 1 test skipped and 0 failures (0 unexpected) in 0.002 seconds\n"
const swiftEvents = `{"version":0,"kind":"test","payload":{"kind":"function","name":"modernAdds()","id":"Module.modernAdds()"}}
{"version":0,"kind":"test","payload":{"kind":"function","name":"modernSkipped()","id":"Module.modernSkipped()"}}
{"version":0,"kind":"event","payload":{"kind":"runStarted"}}
{"version":0,"kind":"event","payload":{"kind":"testStarted","testID":"Module.modernAdds()"}}
{"version":0,"kind":"event","payload":{"kind":"testEnded","testID":"Module.modernAdds()"}}
{"version":0,"kind":"event","payload":{"kind":"testSkipped","testID":"Module.modernSkipped()"}}
{"version":0,"kind":"event","payload":{"kind":"runEnded"}}
`

func swiftConfigCue(mode string) string {
	s := strings.Replace(rustConfigCue(mode), `language: "rust"`, `language: "swift"`, 1)
	s = strings.Replace(s, "Cargo.toml", "main.project.yaml", 1)
	s = strings.Replace(s, "linux-amd64", "darwin-arm64", 1)
	return s
}

func setupSwift(t *testing.T, mode string) swiftProject {
	t.Helper()
	root := setupTempWorkdirWithConfig(t, swiftConfigCue(mode))
	for name, body := range map[string]string{"Package.swift": "// swift-tools-version: 6.0\n", "main.project.yaml": "version: 1.2.3\n", "Sources/Math/Math.swift": "public func add() {}\n", "Tests/MathTests/Tests.swift": "import Testing\n"} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return swiftProject{Path: root, Targets: []swiftTarget{{Name: "Math", Type: "library", Path: "Sources/Math", Sources: []string{"Math.swift"}}, {Name: "MathTests", Type: "test", Path: "Tests/MathTests", Sources: []string{"Tests.swift"}}}, Products: []swiftProduct{{Name: "demo", Type: map[string]json.RawMessage{"executable": json.RawMessage("null")}}}}
}

func stubSwift(t *testing.T, project swiftProject) *[][]string {
	t.Helper()
	old := runCommandCapture
	t.Cleanup(func() { runCommandCapture = old })
	var calls [][]string
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		if name != "swift" {
			t.Fatalf("unexpected tool %s", name)
		}
		calls = append(calls, append([]string(nil), args...))
		flag := func(key string) string {
			for i, arg := range args {
				if arg == key && i+1 < len(args) {
					return args[i+1]
				}
			}
			return ""
		}
		switch args[0] {
		case "--version":
			return "Apple Swift version 6.0.3", "", nil
		case "-print-target-info":
			return `{"target":{"triple":"arm64-apple-macosx14.0"}}`, "", nil
		case "package":
			if contains(args, "dump-package") {
				return `{"dependencies":[],"toolsVersion":{"_version":"6.0.0"}}`, "", nil
			}
			data, err := json.Marshal(project)
			return string(data), "", err
		case "test":
			if path := flag("--event-stream-output-path"); path != "" {
				if err := os.WriteFile(path, []byte(swiftEvents), 0o644); err != nil {
					return "", "", err
				}
			}
			if contains(args, "--enable-code-coverage") {
				path := filepath.Join(flag("--scratch-path"), "coverage.json")
				body := fmt.Sprintf(`{"type":"llvm.coverage.json.export","data":[{"files":[{"filename":%q,"summary":{"lines":{"count":10,"covered":7}}}]}]}`, filepath.Join(project.Path, "Sources/Math/Math.swift"))
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					return "", "", err
				}
				if contains(args, "--show-codecov-path") {
					return path + "\n", "", nil
				}
			}
			return swiftXCTest, "", nil
		case "build":
			if contains(args, "--configuration") {
				dir := flag("--scratch-path")
				if err := os.WriteFile(filepath.Join(dir, "demo"), []byte("fresh executable"), 0o755); err != nil {
					return "", "", err
				}
				if contains(args, "--show-bin-path") {
					return dir + "\n", "", nil
				}
			}
		}
		return "", "", nil
	}
	return &calls
}

func TestSwiftDeveloperCommands(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"format"}, "FORMAT PASS"}, {[]string{"lint"}, "LINT PASS"},
		{[]string{"test", "--style", "per_test"}, "✓ Module.modernAdds()"},
		{[]string{"test", "--failed-only"}, ""}, {[]string{"cov", "--min", "0"}, "total=70.00%"},
	} {
		t.Run(tc.args[0]+tc.want, func(t *testing.T) {
			project := setupSwift(t, "library")
			calls := stubSwift(t, project)
			var out, diag bytes.Buffer
			result := Run(tc.args, &out, &diag)
			if result.ExitCode != 0 || !strings.Contains(out.String(), tc.want) {
				t.Fatalf("result=%+v out=%s err=%s", result, out.String(), diag.String())
			}
			if tc.want == "" && out.Len() != 0 {
				t.Fatal(out.String())
			}
			for _, args := range *calls {
				if (args[0] == "test" || args[0] == "build" || contains(args, "describe")) && !contains(args, "--force-resolved-versions") {
					t.Fatalf("unlocked %v", args)
				}
			}
		})
	}
}

func TestSwiftTestReports(t *testing.T) {
	report, err := parseSwiftTests(swiftXCTest, swiftEvents)
	if err != nil || report.Tests != 4 || report.Skipped != 2 {
		t.Fatalf("%+v %v", report, err)
	}
	failed := strings.Replace(swiftEvents, `{"version":0,"kind":"event","payload":{"kind":"testEnded"`, `{"version":0,"kind":"event","payload":{"kind":"issueRecorded","testID":"Module.modernAdds()","issue":{"isKnown":false}}}`+"\n"+`{"version":0,"kind":"event","payload":{"kind":"testEnded"`, 1)
	report, err = parseSwiftTests(swiftXCTest, failed)
	if err != nil || report.Failed != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	known := strings.Replace(failed, `"isKnown":false`, `"isKnown":true`, 1)
	report, err = parseSwiftTests(swiftXCTest, known)
	if err != nil || report.Failed != 0 {
		t.Fatalf("%+v %v", report, err)
	}
	for _, stream := range []string{"", "not json", strings.Replace(swiftEvents, `"version":0`, `"version":1`, 1), strings.Replace(swiftEvents, `"runEnded"`, `"unknown"`, 1), strings.Replace(swiftEvents, `"testEnded"`, `"unknown"`, 1)} {
		if _, err := parseSwiftTests(swiftXCTest, stream); err == nil {
			t.Fatalf("accepted incomplete %s", stream)
		}
	}
	if _, err := parseSwiftTests(strings.Replace(swiftXCTest, "Executed 2", "Executed 3", 1), swiftEvents); err == nil {
		t.Fatal("accepted mismatched XCTest totals")
	}
	linux := "Test Case 'MathTests.testAdds' passed (0.001 seconds).\nExecuted 1 test, with 0 failures (0 unexpected) in 0.001 seconds\n"
	if _, err := parseSwiftTests(linux, swiftEvents); err != nil {
		t.Fatal(err)
	}
}

func TestSwiftBuildAndFreshRelease(t *testing.T) {
	for _, mode := range []string{"library", "binary"} {
		t.Run(mode, func(t *testing.T) {
			project := setupSwift(t, mode)
			stubSwift(t, project)
			var out, diag bytes.Buffer
			result := Run([]string{"build", "--output-dir", "output with spaces"}, &out, &diag)
			if result.ExitCode != 0 {
				t.Fatalf("%+v %s", result, diag.String())
			}
			if mode == "binary" {
				checksums, err := os.ReadFile("output with spaces/checksums.txt")
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(checksums), "gh-flarebyte-darwin-arm64.tar.gz") {
					t.Fatal(string(checksums))
				}
				if err := os.MkdirAll("build", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile("build/stale.tar.gz", []byte("stale"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("GH_FLAREBYTE_FAKE_RELEASE", "1")
			out.Reset()
			diag.Reset()
			result = Run([]string{"release", "--draft"}, &out, &diag)
			if result.ExitCode != 0 || strings.Contains(out.String(), "stale.tar.gz") {
				t.Fatalf("%+v %s %s", result, out.String(), diag.String())
			}
		})
	}
}

func TestSwiftSelectionAndTargets(t *testing.T) {
	project := setupSwift(t, "library")
	stubSwift(t, project)
	if product, err := swiftExecutable(project, ""); err != nil || product != "demo" {
		t.Fatalf("%s %v", product, err)
	}
	project.Products = append(project.Products, swiftProduct{Name: "other", Type: map[string]json.RawMessage{"executable": json.RawMessage("null")}})
	if _, err := swiftExecutable(project, ""); err == nil {
		t.Fatal("ambiguous product accepted")
	}
	if _, err := swiftExecutable(project, "other"); err != nil {
		t.Fatal(err)
	}
	for _, selection := range [][]string{{"missing"}, {"MathTests"}, {"./...", "Math"}} {
		if _, err := swiftSelectedTargets(project, selection); err == nil {
			t.Fatal(selection)
		}
	}
	if selected, err := swiftSelectedTargets(project, []string{"Math"}); err != nil || len(selected) != 1 {
		t.Fatalf("%v %v", selected, err)
	}
	var out, diag bytes.Buffer
	if result := Run([]string{"build", "--target", "linux-amd64"}, &out, &diag); result.ExitCode != ExitUsage {
		t.Fatalf("%+v %s", result, diag.String())
	}
	if result := Run([]string{"build", "--target", "darwin-arm64"}, &out, &diag); result.ExitCode != 0 {
		t.Fatalf("%+v %s", result, diag.String())
	}
}

func TestSwiftCoverageScopeAndPolicy(t *testing.T) {
	project := setupSwift(t, "library")
	stubSwift(t, project)
	allowed, err := swiftSourceFiles(project, true)
	if err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(`{"type":"llvm.coverage.json.export","data":[{"files":[{"filename":%q,"summary":{"lines":{"count":10,"covered":7}}},{"filename":%q,"summary":{"lines":{"count":1000,"covered":0}}}]}]}`, filepath.Join(project.Path, "Sources/Math/Math.swift"), filepath.Join(project.Path, "Tests/MathTests/Tests.swift"))
	percent, details, err := parseSwiftCoverage([]byte(data), allowed)
	if err != nil || percent != 70 || len(details) != 1 {
		t.Fatalf("%v %v %v", percent, details, err)
	}
	for _, body := range []string{`{}`, `not json`, strings.Replace(data, `"covered":7`, `"covered":11`, 1)} {
		if _, _, err := parseSwiftCoverage([]byte(body), allowed); err == nil {
			t.Fatal(body)
		}
	}
	var out, diag bytes.Buffer
	if result := Run([]string{"cov", "--min", "80"}, &out, &diag); result.ExitCode != 1 || !strings.Contains(diag.String(), "coverage below minimum") {
		t.Fatalf("%+v %s", result, diag.String())
	}
	out.Reset()
	diag.Reset()
	if result := Run([]string{"cov", "--min", "0", "--failed-only"}, &out, &diag); result.ExitCode != 0 || out.Len() != 0 {
		t.Fatalf("%+v %s %s", result, out.String(), diag.String())
	}
}

func TestSwiftDependencyAndToolFailures(t *testing.T) {
	project := setupSwift(t, "library")
	stubSwift(t, project)
	original := runCommandCapture
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		if contains(args, "dump-package") {
			return `{"dependencies":[{"sourceControl":[]}],"toolsVersion":{"_version":"6.0.0"}}`, "", nil
		}
		return original(name, args, env)
	}
	if _, err := loadSwiftProject(os.Environ()); err == nil || !strings.Contains(err.Error(), "Package.resolved") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat("Package.resolved"); !os.IsNotExist(err) {
		t.Fatal("lock created")
	}
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		return "", "missing Swift", fmt.Errorf("tool unavailable")
	}
	for _, args := range [][]string{{"test"}, {"format"}, {"lint"}, {"cov"}, {"build"}} {
		var out, diag bytes.Buffer
		result := Run(args, &out, &diag)
		if result.ExitCode == 0 || !strings.Contains(diag.String(), "missing Swift") {
			t.Fatalf("%+v %s", result, diag.String())
		}
	}
}

func TestSwiftBuildStopsBeforePackaging(t *testing.T) {
	project := setupSwift(t, "binary")
	calls := stubSwift(t, project)
	original := runCommandCapture
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(strings.Replace(swiftConfigCue("binary"), "language:", "runTests: true\n language:", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		if args[0] == "test" {
			return "assertion failed", "fixture build tests failed", fmt.Errorf("tests failed")
		}
		return original(name, args, env)
	}
	var out, diag bytes.Buffer
	result := Run([]string{"build"}, &out, &diag)
	if result.ExitCode != ExitBuildFailure || !strings.Contains(diag.String(), "fixture build tests failed") {
		t.Fatalf("%+v %s", result, diag.String())
	}
	for _, args := range *calls {
		if args[0] == "build" {
			t.Fatalf("build ran after failing tests: %v", args)
		}
	}
}

func TestSwiftRejectsIncompleteBinaryAssets(t *testing.T) {
	project := setupSwift(t, "binary")
	stubSwift(t, project)
	original := runCommandCapture
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		out, diag, err := original(name, args, env)
		if contains(args, "--show-bin-path") && err == nil {
			err = os.Mkdir(filepath.Join(strings.TrimSpace(out), "FixtureResources.bundle"), 0o755)
		}
		return out, diag, err
	}
	var out, diag bytes.Buffer
	result := Run([]string{"build"}, &out, &diag)
	if result.ExitCode != ExitBuildFailure || !strings.Contains(diag.String(), "resource bundles") {
		t.Fatalf("%+v %s", result, diag.String())
	}
	if _, err := os.Stat("build/checksums.txt"); !os.IsNotExist(err) {
		t.Fatalf("packaged incomplete binary: %v", err)
	}
}

func TestSwiftWeightedCoverage(t *testing.T) {
	project := setupSwift(t, "library")
	second := filepath.Join(project.Path, "Sources/Math/Second.swift")
	if err := os.WriteFile(second, []byte("public func second() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	project.Targets[0].Sources = append(project.Targets[0].Sources, "Second.swift")
	allowed, err := swiftSourceFiles(project, true)
	if err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(`{"type":"llvm.coverage.json.export","data":[{"files":[{"filename":%q,"summary":{"lines":{"count":10,"covered":5}}},{"filename":%q,"summary":{"lines":{"count":2,"covered":1}}}]}]}`, filepath.Join(project.Path, "Sources/Math/Math.swift"), second)
	percent, details, err := parseSwiftCoverage([]byte(data), allowed)
	if err != nil || percent != 50 || len(details) != 2 {
		t.Fatalf("%v %v %v", percent, details, err)
	}
	// Unequal file coverage must be weighted by source lines, rather than averaging percentages.
	data = strings.Replace(data, `"count":2,"covered":1`, `"count":2,"covered":2`, 1)
	percent, _, err = parseSwiftCoverage([]byte(data), allowed)
	if err != nil || percent < 58.33 || percent > 58.34 {
		t.Fatalf("%v %v", percent, err)
	}
}

func TestSwiftMinimumToolchainAndManifest(t *testing.T) {
	project := setupSwift(t, "library")
	stubSwift(t, project)
	original := runCommandCapture
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		if args[0] == "--version" {
			return "Apple Swift version 5.9.2", "", nil
		}
		return original(name, args, env)
	}
	if _, err := loadSwiftProject(os.Environ()); err == nil || !strings.Contains(err.Error(), "Swift 6+") {
		t.Fatalf("%v", err)
	}
	runCommandCapture = original
	if err := os.Remove("Package.swift"); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSwiftProject(os.Environ()); err == nil || !strings.Contains(err.Error(), "Package.swift") {
		t.Fatalf("%v", err)
	}
}

func TestSwiftHostTargetMapping(t *testing.T) {
	original := runCommandCapture
	t.Cleanup(func() { runCommandCapture = original })
	for _, tc := range []struct{ triple, target string }{
		{"arm64-apple-macosx14.0", "darwin-arm64"}, {"x86_64-apple-macosx14.0", "darwin-amd64"},
		{"aarch64-unknown-linux-gnu", "linux-arm64"}, {"x86_64-unknown-linux-gnu", "linux-amd64"},
		{"arm64-apple-ios18.0", ""}, {"x86_64-unknown-windows-msvc", ""}, {"x86_64-unknown-linux-musl", ""},
	} {
		t.Run(tc.triple, func(t *testing.T) {
			runCommandCapture = func(name string, args, env []string) (string, string, error) {
				return fmt.Sprintf(`{"target":{"triple":%q}}`, tc.triple), "", nil
			}
			target, err := swiftHostTarget(nil)
			if tc.target == "" {
				if err == nil {
					t.Fatalf("accepted %s", tc.triple)
				}
			} else if err != nil || target != tc.target {
				t.Fatalf("%s %v", target, err)
			}
		})
	}
}

func TestSwiftConfigModesAndDefaults(t *testing.T) {
	setupSwift(t, "binary")
	cfg, err := config.Load(".gh-flarebyte.cue")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Build.MainPackage != "" {
		t.Fatalf("default Swift product should be inferred: %s", cfg.Build.MainPackage)
	}
	if cfg.Build.Mode != "binary" || cfg.Build.Language != "swift" {
		t.Fatalf("%+v", cfg.Build)
	}
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(swiftConfigCue("package")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(".gh-flarebyte.cue"); err == nil {
		t.Fatal("Swift package mode accepted")
	}
}
