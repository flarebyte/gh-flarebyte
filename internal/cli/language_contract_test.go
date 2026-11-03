package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the same user-facing policy through every language dispatcher.
// Tool-specific command contracts remain in each language's tests.
func setupLanguageContract(t *testing.T, language string) {
	t.Helper()
	switch language {
	case "typescript":
		setupTypeScript(t, "library")
		stubTypeScript(t)
	case "swift":
		stubSwift(t, setupSwift(t, "library"))
	case "rust":
		stubRust(t, setupRust(t, "library"))
	case "python":
		setupPython(t, pythonConfigCue())
		stubPython(t)
	default:
		cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language: "`+language+`"`, 1)
		setupTempWorkdirWithConfig(t, cfg)
		if err := os.WriteFile("main.go", []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		old := runCommandCapture
		t.Cleanup(func() { runCommandCapture = old })
		runCommandCapture = func(name string, args, env []string) (string, string, error) {
			if name == "go" && args[0] == "tool" {
				return "main.go:1: main 70.0%\ntotal: (statements) 70.0%\n", "", nil
			}
			if name == "go" && args[0] == "test" {
				return "{\"Action\":\"pass\",\"Test\":\"TestAdds\"}\n{\"Action\":\"skip\",\"Test\":\"TestSkipped\"}\n", "", nil
			}
			if name == "dart" && args[0] == "test" {
				if contains(args, "--coverage") {
					path := filepath.Join(".dart_tool", "coverage", "lcov.info")
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						return "", "", err
					}
					return "", "", os.WriteFile(path, []byte("SF:lib/main.dart\nLF:10\nLH:7\nend_of_record\n"), 0o644)
				}
				return "{\"type\":\"testDone\",\"test\":{\"id\":1,\"name\":\"adds\"},\"result\":\"success\"}\n{\"type\":\"testDone\",\"test\":{\"id\":2,\"name\":\"skipped\",\"skip\":true},\"result\":\"success\"}\n", "", nil
			}
			return "", "", nil
		}
		data, err := os.ReadFile(".gh-flarebyte.cue")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(".gh-flarebyte.cue", append(data, []byte("\ndevOutput: { color: \"false\" }\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLanguageDeveloperContract(t *testing.T) {
	for _, language := range []string{"go", "dart", "python", "rust", "swift", "typescript"} {
		for _, tc := range []struct {
			name, extraConfig string
			devFields         string
			args              []string
			wantCode          int
			want              string
			quiet             bool
			failTool          bool
		}{
			{name: "test summary", args: []string{"test"}, want: "TEST PASS"},
			{name: "test per test", args: []string{"test", "--style", "per_test"}, want: "✓ "},
			{name: "test failures only", args: []string{"test", "--failed-only"}, quiet: true},
			{name: "test per test failures only", args: []string{"test", "--style", "per_test", "--failed-only"}, quiet: true},
			{name: "hide passing summary", devFields: "showPassed: false", args: []string{"test"}, quiet: true},
			{name: "keep skipped tests", devFields: "showPassed: false", args: []string{"test", "--style", "per_test"}, want: "↷ "},
			{name: "color override", args: []string{"test", "--color", "true"}, want: "\x1b[32mPASS\x1b[0m"},
			{name: "test failure", args: []string{"test"}, failTool: true, wantCode: ExitFailure, want: "fixture failure"},
			{name: "format", args: []string{"format"}, want: "FORMAT PASS"},
			{name: "lint", args: []string{"lint"}, want: "LINT PASS"},
			{name: "lint failures only", args: []string{"lint", "--failed-only"}, quiet: true},
			{name: "lint failure", args: []string{"lint", "--failed-only"}, failTool: true, wantCode: ExitFailure, want: "fixture failure"},
			{name: "coverage override", extraConfig: "coverage: { min: 90 }", args: []string{"cov", "--min", "60"}, want: "min=60.00%"},
			{name: "coverage enforced", extraConfig: "coverage: { min: 100 }", args: []string{"cov"}, wantCode: ExitFailure, want: "COV FAIL"},
			{name: "coverage advisory", extraConfig: "coverage: { min: 100, enforceMin: false }", args: []string{"cov"}, want: "COV PASS"},
			{name: "coverage failures only", args: []string{"cov", "--min", "0", "--failed-only"}, quiet: true},
			{name: "coverage entries", devFields: `style: "per_test"`, args: []string{"cov", "--min", "0"}, want: "✓ "},
			{name: "coverage failure", args: []string{"cov"}, failTool: true, wantCode: ExitFailure, want: "fixture failure"},
			{name: "invalid color", args: []string{"test", "--color", "invalid"}, wantCode: ExitUsage, want: "--color"},
			{name: "invalid style", args: []string{"test", "--style", "invalid"}, wantCode: ExitUsage, want: "--style"},
			{name: "invalid minimum", args: []string{"cov", "--min", "101"}, wantCode: ExitUsage, want: "--min"},
		} {
			t.Run(language+"/"+tc.name, func(t *testing.T) {
				setupLanguageContract(t, language)
				t.Setenv("NO_COLOR", "")
				if tc.devFields != "" {
					data, err := os.ReadFile(".gh-flarebyte.cue")
					if err != nil {
						t.Fatal(err)
					}
					cfg := strings.Replace(string(data), "devOutput: {", "devOutput: {"+tc.devFields+",", 1)
					if err := os.WriteFile(".gh-flarebyte.cue", []byte(cfg), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				if tc.failTool || tc.wantCode == ExitUsage {
					runCommandCapture = func(string, []string, []string) (string, string, error) {
						if !tc.failTool {
							t.Fatal("invalid flags reached the toolchain")
						}
						return "", "fixture failure", fmt.Errorf("tool failed")
					}
				}
				file, err := os.OpenFile(".gh-flarebyte.cue", os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatal(err)
				}
				_, writeErr := fmt.Fprintln(file, tc.extraConfig)
				closeErr := file.Close()
				if writeErr != nil || closeErr != nil {
					t.Fatalf("config write: %v %v", writeErr, closeErr)
				}
				var out, diag bytes.Buffer
				result := Run(tc.args, &out, &diag)
				if result.ExitCode != tc.wantCode || !strings.Contains(out.String()+diag.String(), tc.want) {
					t.Fatalf("result=%+v stdout=%s stderr=%s", result, &out, &diag)
				}
				if tc.quiet && (out.Len() != 0 || diag.Len() != 0) {
					t.Fatalf("expected quiet success: stdout=%s stderr=%s", &out, &diag)
				}
			})
		}
	}
}
