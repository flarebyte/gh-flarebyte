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

const rustPrettyTests = "running 3 tests\ntest tests::adds ... ok\ntest tests::ignored ... ignored, fixture\ntest integration ... ok\n\ntest result: ok. 2 passed; 0 failed; 1 ignored; 0 measured; 0 filtered out; finished in 0.00s\n"

func rustConfigCue(mode string) string {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, "language: \"rust\"\n\tmode: \""+mode+"\"", 1)
	if mode != "binary" {
		cfg = strings.Replace(cfg, "targets: [\n\t\t\"linux-amd64\",\n\t]", "", 1)
	}
	cfg = strings.Replace(cfg, "main.project.yaml", "Cargo.toml", 1)
	if mode == "library" {
		cfg = strings.Replace(cfg, "includeChecksums: true", "includeChecksums: true\n includeArtifacts: false", 1)
	}
	return cfg + "\ndevOutput: { color: \"false\" }\n"
}

func setupRust(t *testing.T, mode string) cargoProject {
	t.Helper()
	root := setupTempWorkdirWithConfig(t, rustConfigCue(mode))
	for name, text := range map[string]string{"Cargo.toml": "[package]\nname='rust-demo'\nversion='1.2.3'\n", "Cargo.lock": "version=4\n"} {
		if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := cargoPackage{ID: "demo", Name: "rust-demo", Version: "1.2.3", ManifestPath: filepath.Join(root, "Cargo.toml"), Targets: []cargoTarget{{Name: "rust-demo", Kind: []string{"bin"}}}}
	return cargoProject{Root: root, Members: []string{"demo"}, Packages: []cargoPackage{p}}
}

func stubRust(t *testing.T, project cargoProject) *[][]string {
	t.Helper()
	old := runCommandCapture
	t.Cleanup(func() { runCommandCapture = old })
	var calls [][]string
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		if name == "rustc" {
			return "rustc 1.99.0\nhost: aarch64-apple-darwin\n", "", nil
		}
		if name != "cargo" {
			t.Fatalf("unexpected Rust tool: %s", name)
		}
		calls = append(calls, append([]string(nil), args...))
		if args[0] != "fmt" && !contains(args, "--locked") {
			t.Fatalf("unlocked Cargo command: %v", args)
		}
		switch args[0] {
		case "metadata":
			data, err := json.Marshal(project)
			return string(data), "", err
		case "test":
			return rustPrettyTests, "", nil
		case "llvm-cov":
			path := ""
			for i, arg := range args {
				if arg == "--output-path" {
					path = args[i+1]
				}
			}
			if envValue(env, "CARGO_LLVM_COV_TARGET_DIR") == "" {
				t.Fatal("coverage artifacts are not isolated")
			}
			return "", "", os.WriteFile(path, []byte("SF:"+filepath.Join(project.Root, "src", "lib.rs")+"\nLF:10\nLH:7\nend_of_record\n"), 0o644)
		case "package":
			dir := filepath.Join(envValue(env, "CARGO_TARGET_DIR"), "package")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", "", err
			}
			for _, p := range project.Packages {
				if err := os.WriteFile(filepath.Join(dir, p.Name+"-"+p.Version+".crate"), []byte("fresh crate"), 0o644); err != nil {
					return "", "", err
				}
			}
		case "build":
			if contains(args, "--message-format") {
				dir := envValue(env, "CARGO_TARGET_DIR")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return "", "", err
				}
				binary := filepath.Join(dir, "rust-demo")
				if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
					return "", "", err
				}
				data, err := json.Marshal(map[string]any{"reason": "compiler-artifact", "target": map[string]any{"name": "rust-demo", "kind": []string{"bin"}}, "executable": binary})
				return string(data), "", err
			}
		}
		return "", "", nil
	}
	return &calls
}

func TestRustDevCommands(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		want, command string
	}{
		{[]string{"test", "--style", "per_test"}, "✓ tests::adds", "test"},
		{[]string{"test", "--failed-only"}, "", "test"},
		{[]string{"format"}, "FORMAT PASS", "fmt"},
		{[]string{"lint"}, "LINT PASS", "clippy"},
		{[]string{"lint", "--failed-only"}, "", "clippy"},
		{[]string{"cov", "--min", "60"}, "total=70.00% min=60.00%", "llvm-cov"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			project := setupRust(t, "library")
			calls := stubRust(t, project)
			var out, errOut bytes.Buffer
			res := Run(tc.args, &out, &errOut)
			if res.ExitCode != ExitOK || !strings.Contains(out.String(), tc.want) {
				t.Fatalf("%+v stdout=%s stderr=%s", res, &out, &errOut)
			}
			if tc.want == "" && out.Len() != 0 {
				t.Fatalf("failed-only output: %s", &out)
			}
			found := false
			for _, call := range *calls {
				found = found || call[0] == tc.command
			}
			if !found {
				t.Fatalf("missing %s call: %v", tc.command, *calls)
			}
		})
	}
}

func TestRustBuildModes(t *testing.T) {
	for _, mode := range []string{"library", "binary", "package"} {
		t.Run(mode, func(t *testing.T) {
			project := setupRust(t, mode)
			stubRust(t, project)
			var out, errOut bytes.Buffer
			res := Run([]string{"build", "--output-dir", "output with spaces"}, &out, &errOut)
			if res.ExitCode != ExitOK {
				t.Fatalf("%+v %s", res, &errOut)
			}
			if mode == "library" {
				if _, err := os.Stat("output with spaces"); !os.IsNotExist(err) {
					t.Fatal("library build produced assets")
				}
				return
			}
			data, err := os.ReadFile("output with spaces/checksums.txt")
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				parts := strings.SplitN(line, "  ", 2)
				if len(parts) != 2 {
					t.Fatalf("invalid checksum: %s", line)
				}
				sum, err := sha256File(filepath.Join("output with spaces", parts[1]))
				if err != nil || sum != parts[0] {
					t.Fatalf("incorrect checksum: %s %v", line, err)
				}
			}
		})
	}
}

func TestRustReports(t *testing.T) {
	report, err := parseRustTests(rustPrettyTests)
	if err != nil || report.Tests != 3 || report.Skipped != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	report, err = parseRustTests("test failing ... FAILED\ntest result: FAILED. 0 passed; 1 failed; 0 ignored;\n")
	if err != nil || report.Failed != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	for _, text := range []string{"", "custom harness", "test result: ok. 1 passed; 0 failed; 0 ignored;"} {
		if _, err := parseRustTests(text); err == nil {
			t.Fatalf("accepted report %q", text)
		}
	}
	root := t.TempDir()
	lcov := "SF:" + filepath.Join(root, "src", "a.rs") + "\nLF:10\nLH:10\nend_of_record\nSF:" + filepath.Join(root, "src", "b.rs") + "\nLF:30\nLH:10\nend_of_record\nSF:" + filepath.Join(root, "target", "generated.rs") + "\nLF:100\nLH:0\nend_of_record\nSF:" + filepath.Join(t.TempDir(), "dependency.rs") + "\nLF:100\nLH:0\nend_of_record\n"
	percent, details, err := parseRustCoverage(lcov, root)
	if err != nil || percent != 50 || len(details) != 2 || details[0].Label != "src/a.rs" {
		t.Fatalf("%v %v %v", percent, details, err)
	}
	for _, text := range []string{"", "SF:src/a.rs\nLF:10\nLH:11", "SF:src/a.rs\nLF:-1\nLH:0", "SF:src/a.rs\nLF:NaN\nLH:0", "SF:src/a.rs\nLF:10"} {
		if _, _, err := parseRustCoverage(text, root); err == nil {
			t.Fatalf("accepted LCOV %q", text)
		}
	}
}

func TestRustSelectionAndVersions(t *testing.T) {
	project := setupRust(t, "library")
	stubRust(t, project)
	if version, err := resolveVersionFromSource("Cargo.toml"); err != nil || version != "1.2.3" {
		t.Fatalf("version=%s err=%v", version, err)
	}
	if _, err := rustPackages(project, []string{"missing"}); err == nil {
		t.Fatal("accepted missing crate")
	}
	if _, err := rustPackages(project, []string{"./...", "rust-demo"}); err == nil {
		t.Fatal("accepted mixed selections")
	}
	if _, err := rustPackages(project, []string{"rust-demo"}); err != nil {
		t.Fatal(err)
	}
	project.Packages[0].Targets = append(project.Packages[0].Targets, cargoTarget{Name: "second", Kind: []string{"bin"}})
	if _, _, err := rustBinaryPackage(project, "."); err == nil {
		t.Fatal("accepted ambiguous binaries")
	}
	project.Packages[0].DefaultRun = "second"
	if _, bin, err := rustBinaryPackage(project, "."); err != nil || bin != "second" {
		t.Fatalf("%s %v", bin, err)
	}
	for _, target := range []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64"} {
		if _, err := rustTargetTriple(target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rustTargetTriple("unknown"); err == nil {
		t.Fatal("accepted unknown target")
	}
}

func TestRustReleaseFreshAssets(t *testing.T) {
	for _, mode := range []string{"library", "binary", "package"} {
		t.Run(mode, func(t *testing.T) {
			project := setupRust(t, mode)
			stubRust(t, project)
			if err := os.MkdirAll("build", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("build/stale.crate", []byte("stale"), 0o644); err != nil {
				t.Fatal(err)
			}
			oldTag, oldCreate := tagExists, createRelease
			t.Cleanup(func() { tagExists, createRelease = oldTag, oldCreate })
			tagExists = func(string) (bool, error) { return false, nil }
			var uploaded []string
			createRelease = func(tag string, assets []string, notesMode, notesFile string, draft bool) error {
				if tag != "v1.2.3" || !draft {
					t.Fatalf("tag=%s draft=%v", tag, draft)
				}
				uploaded = assets
				return nil
			}
			var out, errOut bytes.Buffer
			res := Run([]string{"release", "--draft"}, &out, &errOut)
			if res.ExitCode != ExitOK {
				t.Fatalf("%+v %s", res, &errOut)
			}
			if mode == "library" && len(uploaded) != 0 {
				t.Fatal("library uploaded assets")
			}
			if mode != "library" && len(uploaded) < 2 {
				t.Fatalf("missing artifacts: %v", uploaded)
			}
			for _, path := range uploaded {
				if strings.Contains(path, "stale") {
					t.Fatal("uploaded stale artifact")
				}
			}
		})
	}
}

func TestRustFailures(t *testing.T) {
	for _, command := range []string{"test", "format", "lint", "cov", "build"} {
		t.Run(command, func(t *testing.T) {
			project := setupRust(t, "library")
			stubRust(t, project)
			delegate := runCommandCapture
			runCommandCapture = func(name string, args, env []string) (string, string, error) {
				if name == "cargo" && args[0] != "metadata" {
					return "stdout diagnostic", "stderr diagnostic", fmt.Errorf("tool failure")
				}
				return delegate(name, args, env)
			}
			var out, errOut bytes.Buffer
			res := Run([]string{command}, &out, &errOut)
			if res.ExitCode == ExitOK || !strings.Contains(errOut.String(), "stdout diagnostic") || !strings.Contains(errOut.String(), "stderr diagnostic") {
				t.Fatalf("%+v %s", res, &errOut)
			}
		})
	}
}

func TestRustCustomHarness(t *testing.T) {
	project := setupRust(t, "library")
	stubRust(t, project)
	if err := os.WriteFile("Cargo.toml", []byte("[package]\nname='rust-demo'\nversion='1.2.3'\n[[test]]\nname='custom'\nharness=false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	res := Run([]string{"test", "--style", "per_test"}, &out, &errOut)
	if res.ExitCode != ExitFailure || !strings.Contains(errOut.String(), "harness=false") {
		t.Fatalf("%+v %s", res, &errOut)
	}
	if res := Run([]string{"test", "--style", "summary"}, &out, &errOut); res.ExitCode != ExitOK {
		t.Fatalf("%+v %s", res, &errOut)
	}
}

func TestRustConfigAndTargetValidation(t *testing.T) {
	setupRust(t, "binary")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Build.MainPackage != "." {
		t.Fatalf("Rust mainPackage default=%s", cfg.Build.MainPackage)
	}
	if err := validateRustBuildTarget(cfg, "darwin-arm64"); err == nil {
		t.Fatal("accepted unconfigured binary target")
	}
	cfg.Build.Mode = "package"
	if err := config.Validate(cfg); err == nil {
		t.Fatal("accepted package target matrix")
	}
	cfg.Build.Targets = nil
	if err := config.Validate(cfg); err != nil {
		t.Fatal(err)
	}
	if err := validateRustBuildTarget(cfg, "linux-amd64"); err == nil {
		t.Fatal("accepted package --target")
	}
}

func TestRustMissingProjectFiles(t *testing.T) {
	for _, name := range []string{"Cargo.toml", "Cargo.lock"} {
		t.Run(name, func(t *testing.T) {
			project := setupRust(t, "library")
			stubRust(t, project)
			if err := os.Remove(name); err != nil {
				t.Fatal(err)
			}
			if _, err := cargoMetadata("Cargo.toml", nil); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("missing %s: %v", name, err)
			}
			if _, err := os.Stat(name); !os.IsNotExist(err) {
				t.Fatalf("recreated %s", name)
			}
		})
	}
}

func TestRustCoveragePolicy(t *testing.T) {
	for _, enforce := range []bool{true, false} {
		t.Run(fmt.Sprint(enforce), func(t *testing.T) {
			project := setupRust(t, "library")
			stubRust(t, project)
			cfg := rustConfigCue("library") + fmt.Sprintf("\ncoverage: { min: 80, enforceMin: %t }\n", enforce)
			if err := os.WriteFile(".gh-flarebyte.cue", []byte(cfg), 0o644); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			res := Run([]string{"cov"}, &out, &errOut)
			if (res.ExitCode != ExitOK) != enforce {
				t.Fatalf("%+v stdout=%s stderr=%s", res, &out, &errOut)
			}
			out.Reset()
			errOut.Reset()
			res = Run([]string{"cov", "--min", "60", "--failed-only"}, &out, &errOut)
			if res.ExitCode != ExitOK || out.Len() != 0 {
				t.Fatalf("%+v stdout=%s stderr=%s", res, &out, &errOut)
			}
		})
	}
}

func TestRustMultiTargetStaging(t *testing.T) {
	project := setupRust(t, "binary")
	stubRust(t, project)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Build.Targets = []string{"linux-amd64", "windows-amd64"}
	cfg.Build.ArtifactTargetSuffix = false
	cfg.Build.ChecksumFile = "manifests/custom.sha256"
	built, err := buildRust(cfg, "", "", &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, include := range []bool{true, false} {
		staged, err := stageFreshRelease(built, cfg.Build.OutputDir, "release assets", include)
		if err != nil {
			t.Fatal(err)
		}
		want := 4
		if include {
			want++
		}
		if len(staged) != want {
			t.Fatalf("assets=%v", staged)
		}
		for _, path := range staged {
			if _, err := os.Stat(path); err != nil {
				t.Fatal(err)
			}
		}
	}
	info, err := os.Stat("release assets/linux-amd64/gh-flarebyte-linux-amd64")
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("executable permissions: %v %v", info, err)
	}
	data, err := os.ReadFile("release assets/custom.sha256")
	if err != nil || !strings.Contains(string(data), "windows-amd64/gh-flarebyte-windows-amd64.zip") {
		t.Fatalf("manifest=%s err=%v", data, err)
	}
}

func TestRustBuildStopsOnTestFailure(t *testing.T) {
	project := setupRust(t, "package")
	calls := stubRust(t, project)
	delegate := runCommandCapture
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		if name == "cargo" && args[0] == "test" {
			if !contains(args, "aarch64-apple-darwin") {
				t.Fatalf("tests did not select compiler host: %v", args)
			}
			return "test failed", "", fmt.Errorf("tests failed")
		}
		return delegate(name, args, env)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Build.RunTests = true
	if _, err := buildRust(cfg, "", "", &bytes.Buffer{}); err == nil {
		t.Fatal("ignored test failure")
	}
	for _, call := range *calls {
		if call[0] == "package" {
			t.Fatal("packaged after test failure")
		}
	}
}
