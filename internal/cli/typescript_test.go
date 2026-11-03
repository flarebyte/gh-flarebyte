// purpose: Verify TypeScript command policies, strict reports, library/CLI boundaries, and fresh artifacts.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

const typescriptJUnitFixture = `<testsuites tests="2" failures="0" skipped="1"><testsuite name="math" file="test/math.test.ts" tests="2" failures="0" skipped="1"><testcase name="adds"/><testcase name="skipped"><skipped/></testcase></testsuite></testsuites>`

func typescriptConfigCue(mode string) string {
	cfg := strings.Replace(rustConfigCue(mode), `language: "rust"`, `language: "typescript"`, 1)
	cfg = strings.Replace(cfg, "Cargo.toml", "package.json", 1)
	if mode == "binary" {
		cfg = strings.Replace(cfg, "language:", "mainPackage: \"src/cli.ts\"\n language:", 1)
	}
	return cfg
}

func setupTypeScript(t *testing.T, mode string) string {
	t.Helper()
	root := setupTempWorkdirWithConfig(t, typescriptConfigCue(mode))
	for name, data := range map[string]string{
		"package.json": `{"name":"fixture","version":"1.2.3"}`, "package-lock.json": "locked", "tsconfig.json": "{}", "biome.json": "{}",
		"src/math.ts": "export const add = 1;", "src/cli.ts": "console.log(1);", "test/math.test.ts": "test('adds', () => {});",
		typescriptCompiler: "", typescriptBiome: "", "node_modules/c8/bin/c8.js": "",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func typescriptLCOV(name string, count, hit int) string {
	var out strings.Builder
	_, _ = fmt.Fprintf(&out, "SF:%s\nLF:%d\nLH:%d\n", name, count, hit)
	for line := 1; line <= count; line++ {
		value := 0
		if line <= hit {
			value = 1
		}
		_, _ = fmt.Fprintf(&out, "DA:%d,%d\n", line, value)
	}
	return out.String() + "end_of_record\n"
}

func stubTypeScript(t *testing.T) *[][]string {
	t.Helper()
	old := runCommandCapture
	t.Cleanup(func() { runCommandCapture = old })
	var calls [][]string
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		if name != "bun" && name != "node" && name != "npm" {
			t.Fatalf("unexpected TypeScript command: %s %v", name, args)
		}
		if name == "bun" {
			args = args[1:]
		}
		calls = append(calls, append([]string(nil), args...))
		flag := func(key string) string {
			for i, value := range args {
				if strings.HasPrefix(value, key+"=") {
					return strings.TrimPrefix(value, key+"=")
				}
				if value == key && i+1 < len(args) {
					return args[i+1]
				}
			}
			return ""
		}
		switch args[0] {
		case "--version":
			return "1.3.6", "", nil
		case "ci":
			if !contains(args, "--ignore-scripts") {
				t.Fatalf("unlocked or script-enabled installation: %v", args)
			}
		case typescriptCompiler:
			if contains(args, "--showConfig") {
				return `{"files":["./src/math.ts","./src/cli.ts","./test/math.test.ts"]}`, "", nil
			}
		case typescriptBiome:
		case "--eval":
			return "linux-x64", "", nil
		case "test", "./node_modules/c8/bin/c8.js":
			reportPath := ""
			for _, value := range env {
				if strings.HasPrefix(value, "NODE_OPTIONS=") {
					re := regexp.MustCompile(`--test-reporter-destination="([^"]+)"`)
					match := re.FindStringSubmatch(value)
					if len(match) > 1 {
						reportPath = match[1]
					}
				}
			}
			if err := os.WriteFile(reportPath, []byte(typescriptJUnitFixture), 0o644); err != nil {
				return "", "", err
			}
			if args[0] != "test" {
				data := typescriptLCOV("src/math.ts", 10, 7) + typescriptLCOV("src/cli.ts", 1, 1)
				return "", "", os.WriteFile(filepath.Join(flag("--reports-dir"), "lcov.info"), []byte(data), 0o644)
			}

		case "build":
			if !contains(args, "--compile") {
				t.Fatal("CLI build did not compile an executable")
			}
			return "", "", os.WriteFile(flag("--outfile"), []byte("fresh CLI"), 0o755)
		default:
			t.Fatalf("unexpected Bun arguments: %v", args)
		}
		return "", "", nil
	}
	return &calls
}

func TestTypeScriptLibraryAndCLI(t *testing.T) {
	for _, mode := range []string{"library", "binary"} {
		t.Run(mode, func(t *testing.T) {
			setupTypeScript(t, mode)
			calls := stubTypeScript(t)
			var out, diagnostics bytes.Buffer
			result := Run([]string{"build", "--output-dir", "output with spaces"}, &out, &diagnostics)
			if result.ExitCode != ExitOK {
				t.Fatalf("%+v %s", result, &diagnostics)
			}
			compiled := false
			for _, call := range *calls {
				compiled = compiled || call[0] == "build"
			}
			if compiled != (mode == "binary") {
				t.Fatalf("library/CLI boundary violated: %v", *calls)
			}
			if mode == "library" {
				if _, err := os.Stat("output with spaces"); !os.IsNotExist(err) {
					t.Fatal("library verification wrote release artifacts")
				}
			} else {
				data, err := os.ReadFile("output with spaces/checksums.txt")
				if err != nil {
					t.Fatal(err)
				}
				entries := strings.Split(strings.TrimSpace(string(data)), "\n")
				if len(entries) != 2 {
					t.Fatalf("expected archive and executable: %s", data)
				}
				for _, entry := range entries {
					parts := strings.SplitN(entry, "  ", 2)
					if len(parts) != 2 {
						t.Fatal(entry)
					}
					sum, err := sha256File(filepath.Join("output with spaces", parts[1]))
					if err != nil || sum != parts[0] {
						t.Fatalf("incorrect checksum: %s %v", entry, err)
					}
				}
			}
			if err := os.MkdirAll("build", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("build/stale.tar.gz", []byte("stale"), 0o644); err != nil {
				t.Fatal(err)
			}
			var assets []string
			stubReleaseCaptureArtifacts(t, "1.2.3", false, &assets)
			result = Run([]string{"release", "--draft"}, &out, &diagnostics)
			if result.ExitCode != ExitOK || (mode == "library" && len(assets) != 0) || (mode == "binary" && len(assets) != 3) {
				t.Fatalf("%+v assets=%v %s", result, assets, &diagnostics)
			}
			for _, path := range assets {
				if strings.Contains(path, "stale") {
					t.Fatal("stale asset was released")
				}
			}
		})
	}
}

func TestTypeScriptConfigBoundaries(t *testing.T) {
	for _, mode := range []string{"library", "binary", "package"} {
		t.Run(mode, func(t *testing.T) {
			setupTypeScript(t, mode)
			cfg, err := config.Load(".gh-flarebyte.cue")
			if mode == "package" {
				if err == nil {
					t.Fatal("TypeScript package mode accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "binary" {
				cfg.Build.MainPackage = ""
			} else {
				cfg.Build.MainPackage = "src/cli.ts"
			}
			if err := config.Validate(cfg); err == nil {
				t.Fatal("library/CLI configuration boundary not enforced")
			}
		})
	}
}

func TestTypeScriptReports(t *testing.T) {
	report, err := parseTypeScriptTests([]byte(typescriptJUnitFixture))
	if err != nil || report.Tests != 2 || report.Skipped != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	for _, data := range []string{"", "<invalid/>", strings.Replace(typescriptJUnitFixture, `tests="2"`, `tests="3"`, 1)} {
		if _, err := parseTypeScriptTests([]byte(data)); err == nil {
			t.Fatalf("accepted incomplete report: %s", data)
		}
	}
	root := t.TempDir()
	allowed := map[string]string{filepath.Join(root, "a.ts"): "a.ts", filepath.Join(root, "b.ts"): "b.ts"}
	data := typescriptLCOV("a.ts", 10, 5) + typescriptLCOV("b.ts", 2, 2) + typescriptLCOV("test/test.ts", 100, 100)
	percent, entries, err := parseTypeScriptCoverage(data, root, allowed)
	if err != nil || percent < 58.33 || percent > 58.34 || len(entries) != 2 {
		t.Fatalf("%v %v %v", percent, entries, err)
	}
	for _, invalid := range []string{
		typescriptLCOV("a.ts", 10, 5),
		data + typescriptLCOV("a.ts", 10, 5),
		strings.Replace(data, "LF:10", "LF:11", 1),
		strings.Replace(data, "LH:5", "LH:6", 1),
		strings.TrimSuffix(data, "end_of_record\n"),
	} {
		if _, _, err := parseTypeScriptCoverage(invalid, root, allowed); err == nil {
			t.Fatalf("accepted invalid coverage: %s", invalid)
		}
	}
}

func TestTypeScriptFailures(t *testing.T) {
	for _, command := range []string{"format", "lint", "test", "cov", "build"} {
		t.Run(command, func(t *testing.T) {
			setupTypeScript(t, "library")
			stubTypeScript(t)
			runCommandCapture = func(string, []string, []string) (string, string, error) {
				return "stdout diagnostic", "stderr diagnostic", fmt.Errorf("tool failed")
			}
			var out, diagnostics bytes.Buffer
			result := Run([]string{command}, &out, &diagnostics)
			if result.ExitCode == ExitOK || !strings.Contains(diagnostics.String(), "stdout diagnostic") || !strings.Contains(diagnostics.String(), "stderr diagnostic") {
				t.Fatalf("%+v %s", result, &diagnostics)
			}
		})
	}
}

func TestTypeScriptStopsBeforePackagingOnTypeErrors(t *testing.T) {
	setupTypeScript(t, "binary")
	calls := stubTypeScript(t)
	base := runCommandCapture
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		if contains(args, "--noEmit") {
			return "type error", "", fmt.Errorf("type check failed")
		}
		return base(name, args, env)
	}
	var out, diagnostics bytes.Buffer
	result := Run([]string{"build"}, &out, &diagnostics)
	if result.ExitCode != ExitBuildFailure {
		t.Fatalf("%+v %s", result, &diagnostics)
	}
	for _, call := range *calls {
		if call[0] == "build" {
			t.Fatal("Bun compiled a CLI despite type errors")
		}
	}
	if _, err := os.Stat("build/checksums.txt"); !os.IsNotExist(err) {
		t.Fatal("type error produced packaged assets")
	}
}

func TestTypeScriptMissingLockAndInvalidTarget(t *testing.T) {
	setupTypeScript(t, "library")
	stubTypeScript(t)
	runCommandCapture = func(string, []string, []string) (string, string, error) {
		t.Fatal("invalid request reached Bun")
		return "", "", nil
	}
	var out, diagnostics bytes.Buffer
	if result := Run([]string{"build", "--target", "linux-amd64"}, &out, &diagnostics); result.ExitCode != ExitUsage {
		t.Fatalf("%+v %s", result, &diagnostics)
	}
	if err := os.Remove("package-lock.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareTypeScript(os.Environ()); err == nil || !strings.Contains(err.Error(), "package-lock.json") {
		t.Fatalf("missing lock accepted: %v", err)
	}
}

func TestTypeScriptManifestRejectsWorkspaces(t *testing.T) {
	setupTypeScript(t, "library")
	data, _ := json.Marshal(map[string]any{"workspaces": []string{"packages/*"}})
	if err := os.WriteFile("package.json", data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareTypeScript(os.Environ()); err == nil || !strings.Contains(err.Error(), "workspaces") {
		t.Fatalf("workspace accepted: %v", err)
	}
}

func TestTypeScriptCustomToolingCoverageRoots(t *testing.T) {
	project := typescriptProject{Files: map[string]string{
		"/repo/src/main.ts":       "src/main.ts",
		"/repo/checks/test.ts":    "checks/test.ts",
		"/repo/tools/generate.ts": "tools/generate.ts",
	}, ToolingRoots: []string{"checks", "tools"}}
	files, err := project.coverageFiles(nil)
	if err != nil || len(files) != 1 || files["/repo/src/main.ts"] != "src/main.ts" {
		t.Fatalf("tooling included in production coverage: %v %v", files, err)
	}
}
