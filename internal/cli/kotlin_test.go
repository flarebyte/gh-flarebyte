// purpose: Exercise Kotlin's shared developer contract, fresh report policy, and JVM release assets.
package cli

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const kotlinJUnit = `<testsuite name="fixture.MathTest" tests="2" failures="0" errors="0" skipped="1"><testcase classname="fixture.MathTest" name="adds"/><testcase name="skipped"><skipped/></testcase></testsuite>`
const kotlinJacoco = `<report name="fixture"><package name="fixture"><sourcefile name="Math.kt"><counter type="LINE" missed="3" covered="7"/></sourcefile></package><counter type="LINE" missed="3" covered="7"/></report>`

func setupKotlin(t *testing.T, mode string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "e2e", "fixtures", "kotlin", ".gh-flarebyte.cue"))
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "coverage: { min: 0, enforceMin: true }", "", 1))
	cfg := strings.Replace(string(data), `mode: "library"`, `mode: "`+mode+`"`, 1)
	if mode == "binary" {
		cfg = strings.Replace(cfg, "includeArtifacts: false", "includeArtifacts: true", 1)
	}
	root := setupTempWorkdirWithConfig(t, cfg)
	for name, data := range map[string]string{
		"gradlew": "#!/bin/sh\n", "gradle/wrapper/gradle-wrapper.jar": "wrapper", "gradle/wrapper/gradle-wrapper.properties": "distributionUrl=pinned", "gradle.lockfile": "locked", "build.gradle.kts": "plugins {}", "main.project.yaml": "version: 1.2.3\n",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func kotlinTestZip(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := zip.NewWriter(f)
	for _, name := range []string{"fixture/bin/fixture", "fixture/lib/fixture.jar"} {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o755)
		entry, err := w.CreateHeader(h)
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, err := entry.Write([]byte("fixture")); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func stubKotlin(t *testing.T) *[][]string {
	t.Helper()
	old := runCommandCapture
	t.Cleanup(func() { runCommandCapture = old })
	var calls [][]string
	runCommandCapture = func(name string, args, env []string) (string, string, error) {
		if name != "./gradlew" {
			t.Fatalf("unexpected command %s %v", name, args)
		}
		calls = append(calls, append([]string(nil), args...))
		dir := ""
		for _, arg := range args {
			if strings.HasPrefix(arg, "-Dgh.flarebyte.output=") {
				dir = strings.TrimPrefix(arg, "-Dgh.flarebyte.output=")
			}
		}
		if dir == "" {
			t.Fatal("missing isolated output")
		}
		if contains(args, "test") {
			path := filepath.Join(dir, "tests", "test", "TEST-fixture.xml")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(kotlinJUnit), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if contains(args, "jacocoTestReport") {
			if err := os.WriteFile(filepath.Join(dir, "jacoco.xml"), []byte(kotlinJacoco), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if contains(args, "distZip") {
			if err := kotlinTestZip(filepath.Join(dir, "distribution", "application.zip")); err != nil {
				t.Fatal(err)
			}
		}
		return "", "", nil
	}
	return &calls
}

func TestKotlinReports(t *testing.T) {
	report, err := parseKotlinTests([]byte(kotlinJUnit))
	if err != nil || report.Tests != 2 || report.Skipped != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	failed := strings.Replace(kotlinJUnit, `failures="0"`, `failures="1"`, 1)
	failed = strings.Replace(failed, `name="adds"/>`, `name="adds"><failure>wrong sum</failure></testcase>`, 1)
	report, err = parseKotlinTests([]byte(failed))
	if err != nil || report.Failed != 1 || report.Events[0].Action != "fail" {
		t.Fatalf("%+v %v", report, err)
	}
	for _, data := range []string{"", "<invalid/>", strings.Replace(kotlinJUnit, `tests="2"`, `tests="3"`, 1)} {
		if _, err := parseKotlinTests([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	percent, entries, err := parseKotlinCoverage([]byte(kotlinJacoco))
	if err != nil || percent != 70 || len(entries) != 1 || entries[0].Label != "fixture/Math.kt" {
		t.Fatalf("%v %v %v", percent, entries, err)
	}
	for _, data := range []string{"", "<report/>", strings.Replace(kotlinJacoco, `covered="7"`, `covered="8"`, 1), strings.ReplaceAll(kotlinJacoco, `missed="3"`, `missed="-1"`), strings.ReplaceAll(kotlinJacoco, `type="LINE"`, `type="BRANCH"`)} {
		if _, _, err := parseKotlinCoverage([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

func TestKotlinFreshReports(t *testing.T) {
	for _, command := range []string{"test", "cov", "build"} {
		t.Run(command, func(t *testing.T) {
			setupKotlin(t, "library")
			stubKotlin(t)
			// Existing repository reports must never satisfy a new invocation.
			if err := os.MkdirAll("build/test-results/test", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("build/test-results/test/TEST-stale.xml", []byte(kotlinJUnit), 0o644); err != nil {
				t.Fatal(err)
			}
			runCommandCapture = func(string, []string, []string) (string, string, error) { return "", "", nil }
			var out, diag bytes.Buffer
			result := Run([]string{command}, &out, &diag)
			if result.ExitCode == ExitOK || !strings.Contains(diag.String(), "JUnit reports missing") {
				t.Fatalf("%+v %s", result, &diag)
			}
		})
	}
}

func TestKotlinBuildAndRelease(t *testing.T) {
	for _, mode := range []string{"library", "binary"} {
		t.Run(mode, func(t *testing.T) {
			setupKotlin(t, mode)
			calls := stubKotlin(t)
			var out, diag bytes.Buffer
			result := Run([]string{"build", "--output-dir", "output with spaces"}, &out, &diag)
			if result.ExitCode != ExitOK {
				t.Fatalf("%+v %s", result, &diag)
			}
			if mode == "binary" {
				data, err := os.ReadFile("output with spaces/checksums.txt")
				if err != nil {
					t.Fatal(err)
				}
				sum, err := sha256File("output with spaces/kotlin-fixture-1.2.3.zip")
				if err != nil || string(data) != sum+"  kotlin-fixture-1.2.3.zip\n" {
					t.Fatalf("checksum: %s %v", data, err)
				}
			} else if _, err := os.Stat("output with spaces"); !os.IsNotExist(err) {
				t.Fatal("library created assets")
			}
			if !contains((*calls)[0], "test") {
				t.Fatal("build.runTests ignored")
			}
			t.Setenv("GH_FLAREBYTE_FAKE_RELEASE", "1")
			out.Reset()
			diag.Reset()
			if result := Run([]string{"release", "--draft"}, &out, &diag); result.ExitCode != ExitOK {
				t.Fatalf("%+v %s", result, &diag)
			}
			if !strings.Contains(out.String(), "v1.2.3") {
				t.Fatalf("%s", &out)
			}
		})
	}
}

func TestKotlinInvalidRequests(t *testing.T) {
	for _, field := range []string{`targets: ["linux-amd64"]`, `mainPackage: "MainKt"`, `packages: [":lib"]`, `mode: "package"`, `artifactTargetSuffix: false`} {
		t.Run(field, func(t *testing.T) {
			setupKotlin(t, "library")
			data, err := os.ReadFile(".gh-flarebyte.cue")
			if err != nil {
				t.Fatal(err)
			}
			text := strings.ReplaceAll(string(data), `packages: ["./..."]`, "")
			if strings.HasPrefix(field, "mode:") {
				text = strings.Replace(text, `mode: "library"`, field, 1)
			} else {
				text = strings.Replace(text, "build: {", "build: {\n"+field, 1)
			}
			if err := os.WriteFile(".gh-flarebyte.cue", []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			var out, diag bytes.Buffer
			if result := Run([]string{"config", "validate"}, &out, &diag); result.ExitCode != ExitUsage {
				t.Fatalf("%+v %s", result, &diag)
			}
		})
	}
	setupKotlin(t, "binary")
	calls := stubKotlin(t)
	var out, diag bytes.Buffer
	if result := Run([]string{"build", "--target", "linux-amd64"}, &out, &diag); result.ExitCode != ExitUsage || len(*calls) != 0 {
		t.Fatalf("%+v %s", result, &diag)
	}
}

func TestKotlinMissingTooling(t *testing.T) {
	for _, path := range []string{"gradlew", "gradle/wrapper/gradle-wrapper.jar", "gradle/wrapper/gradle-wrapper.properties", "gradle.lockfile"} {
		t.Run(path, func(t *testing.T) {
			setupKotlin(t, "library")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if _, err := prepareKotlin(); err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestKotlinBuildRunTestsFalse(t *testing.T) {
	setupKotlin(t, "library")
	data, _ := os.ReadFile(".gh-flarebyte.cue")
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(strings.Replace(string(data), "runTests: true", "runTests: false", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := stubKotlin(t)
	var out, diag bytes.Buffer
	if result := Run([]string{"build"}, &out, &diag); result.ExitCode != ExitOK {
		t.Fatalf("%+v %s", result, &diag)
	}
	if len(*calls) != 1 || contains((*calls)[0], "test") {
		t.Fatal(fmt.Sprint(*calls))
	}
}

func TestKotlinReleaseStagesOnlyFreshAssets(t *testing.T) {
	for _, checksums := range []bool{false, true} {
		t.Run(fmt.Sprint(checksums), func(t *testing.T) {
			setupKotlin(t, "binary")
			stubKotlin(t)
			data, err := os.ReadFile(".gh-flarebyte.cue")
			if err != nil {
				t.Fatal(err)
			}
			text := strings.Replace(string(data), `artifactDir: "dist"`, `artifactDir: "release-assets"`, 1)
			if !checksums {
				text = strings.Replace(text, "includeChecksums: true", "includeChecksums: false", 1)
			}
			if err := os.WriteFile(".gh-flarebyte.cue", []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, dir := range []string{"dist", "release-assets"} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "stale.zip"), []byte("stale"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			oldTag, oldRelease := tagExists, createRelease
			t.Cleanup(func() { tagExists, createRelease = oldTag, oldRelease })
			tagExists = func(string) (bool, error) { return false, nil }
			createRelease = func(tag string, assets []string, notesMode, notesFile string, draft bool) error {
				if tag != "v1.2.3" || !draft {
					t.Fatalf("tag=%s draft=%v", tag, draft)
				}
				want := []string{filepath.Join("release-assets", "kotlin-fixture-1.2.3.zip")}
				if checksums {
					want = append(want, filepath.Join("release-assets", "checksums.txt"))
				}
				if fmt.Sprint(assets) != fmt.Sprint(want) {
					t.Fatalf("assets=%v want=%v", assets, want)
				}
				for _, path := range assets {
					if _, err := os.Stat(path); err != nil {
						t.Fatal(err)
					}
				}
				return nil
			}
			var out, diag bytes.Buffer
			if result := Run([]string{"release", "--draft"}, &out, &diag); result.ExitCode != ExitOK {
				t.Fatalf("%+v %s", result, &diag)
			}
		})
	}
}

func TestKotlinDistributionValidation(t *testing.T) {
	for _, names := range [][]string{{"../escape"}, {"/absolute"}, {"app/lib/only.jar"}, {"app/bin/only"}, {"app/bin/../escape"}} {
		t.Run(fmt.Sprint(names), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.zip")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			w := zip.NewWriter(f)
			for _, name := range names {
				h := &zip.FileHeader{Name: name}
				h.SetMode(0o755)
				if _, err := w.CreateHeader(h); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if err := validateKotlinDistribution(path); err == nil {
				t.Fatal("accepted invalid distribution")
			}
		})
	}
}
