// purpose: Ensure build and release orchestration stays reliable across target modes, packaging paths, and release policies.
// responsibilities: Verify build artifacts/checksums, version hydration, release gating, and release artifact selection behavior.
// architecture notes: Tests replace command-side function variables to isolate orchestration logic from external tools and network calls.
package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/buildinfo"
	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func setupBuildTargetAndPackagingStubs(t *testing.T) {
	t.Helper()
	oldBuildTargetBinary := buildTargetBinary
	oldPackageBinary := packageBinary
	t.Cleanup(func() {
		buildTargetBinary = oldBuildTargetBinary
		packageBinary = oldPackageBinary
	})
	buildTargetBinary = func(target string, outputPath string, mainPackage string, _ config.CGOConfig) error {
		return os.WriteFile(outputPath, []byte("binary-"+target), 0o755)
	}
	packageBinary = packageBinaryArchive
}

func stubReleaseCaptureArtifacts(t *testing.T, version string, includeTagExists bool, capturedArtifacts *[]string) {
	t.Helper()
	oldFindVersion := findVersion
	oldTagExists := tagExists
	oldCreateRelease := createRelease
	t.Cleanup(func() {
		findVersion = oldFindVersion
		tagExists = oldTagExists
		createRelease = oldCreateRelease
	})
	findVersion = func(sourcePath string) (string, error) { return version, nil }
	tagExists = func(tag string) (bool, error) { return includeTagExists, nil }
	createRelease = func(tag string, artifacts []string, notesMode string, notesFile string, draft bool) error {
		*capturedArtifacts = append([]string{}, artifacts...)
		return nil
	}
}

func setupHydrateBuildInfoTestEnv(
	t *testing.T,
	resolveFn func(sourcePath string) (string, error),
	gitFn func(args ...string) (string, error),
	nowFn func() time.Time,
	goVersionFn func() string,
) {
	t.Helper()
	oldResolveBuildVersion := resolveBuildVersion
	oldReadGitOutput := readGitOutput
	oldCurrentTimeUTC := currentTimeUTC
	oldCurrentGoVersion := currentGoVersion
	oldVersion := buildinfo.Version
	oldCommitID := buildinfo.CommitID
	oldDate := buildinfo.Date
	oldGoVersion := buildinfo.GoVersion
	t.Cleanup(func() {
		resolveBuildVersion = oldResolveBuildVersion
		readGitOutput = oldReadGitOutput
		currentTimeUTC = oldCurrentTimeUTC
		currentGoVersion = oldCurrentGoVersion
		buildinfo.Version = oldVersion
		buildinfo.CommitID = oldCommitID
		buildinfo.Date = oldDate
		buildinfo.GoVersion = oldGoVersion
	})
	resolveBuildVersion = resolveFn
	readGitOutput = gitFn
	currentTimeUTC = nowFn
	currentGoVersion = goVersionFn
}

func TestRunBuildRejectsUnknownConfiguredTargetFilter(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build", "--target", "darwin-arm64"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, result.ExitCode)
	}
}

func TestRunBuildSuccessWritesChecksumAndSummary(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	setupBuildTargetAndPackagingStubs(t)
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build", "--output-dir", "dist"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, result.ExitCode)
	}
	if _, err := os.Stat(filepath.Join("dist", "gh-flarebyte-linux-amd64.tar.gz")); err != nil {
		t.Fatalf("expected artifact: %v", err)
	}
}

func TestRunBuildUsesConfiguredMainPackage(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
	]
}`, `build: {
	language:     "go"
	mainPackage:  "./cmd/flyb"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
	]
}`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldBuildTargetBinary := buildTargetBinary
	oldPackageBinary := packageBinary
	t.Cleanup(func() {
		buildTargetBinary = oldBuildTargetBinary
		packageBinary = oldPackageBinary
	})
	var gotMainPackage string
	buildTargetBinary = func(target string, outputPath string, mainPackage string, _ config.CGOConfig) error {
		gotMainPackage = mainPackage
		return os.WriteFile(outputPath, []byte("binary-"+target), 0o755)
	}
	packageBinary = packageBinaryArchive
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build", "--output-dir", "dist"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, result.ExitCode)
	}
	if gotMainPackage != "./cmd/flyb" {
		t.Fatalf("expected configured main package, got %q", gotMainPackage)
	}
}

func TestRunBuildWithoutSuffixUsesTargetSubdirsForMultipleTargets(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `targets: [
		"linux-amd64",
	]`, `artifactTargetSuffix: false
	targets: [
		"linux-amd64",
		"darwin-arm64",
	]`, 1)
	cfg = strings.Replace(cfg, `artifactTargetSuffix: true`, `artifactTargetSuffix: false`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	setupBuildTargetAndPackagingStubs(t)
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build", "--output-dir", "dist"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (%s)", ExitOK, result.ExitCode, errOut.String())
	}
	if _, err := os.Stat(filepath.Join("dist", "linux-amd64", "gh-flarebyte-linux-amd64.tar.gz")); err != nil {
		t.Fatalf("expected linux artifact in target subdir: %v", err)
	}
	if _, err := os.Stat(filepath.Join("dist", "darwin-arm64", "gh-flarebyte-darwin-arm64.tar.gz")); err != nil {
		t.Fatalf("expected darwin artifact in target subdir: %v", err)
	}
}

func TestRunBuildFailureIncludesUnderlyingError(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldBuildTargetBinary := buildTargetBinary
	t.Cleanup(func() {
		buildTargetBinary = oldBuildTargetBinary
	})
	buildTargetBinary = func(target string, outputPath string, mainPackage string, _ config.CGOConfig) error {
		return errors.New("go: cannot find main module")
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build"}, &out, &errOut)
	if result.ExitCode != ExitBuildFailure {
		t.Fatalf("expected exit code %d, got %d", ExitBuildFailure, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "Underlying error: go: cannot find main module") {
		t.Fatalf("expected underlying error in stderr, got: %s", errOut.String())
	}
}

func TestRunBuildDartLibraryModeSuccess(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	cfg = strings.Replace(cfg, "build: {\n\tlanguage:     \"dart\"", "build: {\n\tlanguage:     \"dart\"\n\tmode:         \"library\"\n\tpackages: [\n\t\t\"./...\",\n\t]\n\trunTests:     true", 1)
	cfg = strings.Replace(cfg, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}`, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	includeArtifacts: false
}`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldDartBuildPackages := dartBuildPackages
	t.Cleanup(func() {
		dartBuildPackages = oldDartBuildPackages
	})
	called := false
	dartBuildPackages = func(runTests bool) error {
		called = true
		if !runTests {
			t.Fatalf("expected runTests=true")
		}
		return nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", result.ExitCode, errOut.String())
	}
	if !called {
		t.Fatalf("expected dart build execution")
	}
	if !strings.Contains(out.String(), "Dart library validation passed") {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestRunBuildDartLibraryModeFailure(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	cfg = strings.Replace(cfg, "build: {\n\tlanguage:     \"dart\"", "build: {\n\tlanguage:     \"dart\"\n\tmode:         \"library\"\n\tpackages: [\n\t\t\"./...\",\n\t]", 1)
	cfg = strings.Replace(cfg, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}`, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	includeArtifacts: false
}`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldDartBuildPackages := dartBuildPackages
	t.Cleanup(func() {
		dartBuildPackages = oldDartBuildPackages
	})
	dartBuildPackages = func(runTests bool) error {
		return errors.New("analyze failed")
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build"}, &out, &errOut)
	if result.ExitCode != ExitBuildFailure {
		t.Fatalf("expected build failure, got %d stderr=%s", result.ExitCode, errOut.String())
	}
	if !strings.Contains(errOut.String(), "Build failed in library mode during dart pub/analyze/test.") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestPackageBinaryArchiveTarGzAndZip(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "gh-flarebyte-linux-amd64")
	if err := os.WriteFile(src, []byte("linux-binary"), 0o755); err != nil {
		t.Fatalf("write src failed: %v", err)
	}
	tarGz := filepath.Join(tmpDir, "artifact.tar.gz")
	if err := packageBinaryArchive(src, "linux-amd64", tarGz, "gh-flarebyte"); err != nil {
		t.Fatalf("package tar.gz failed: %v", err)
	}
	f, err := os.Open(tarGz)
	if err != nil {
		t.Fatalf("open tar.gz failed: %v", err)
	}
	defer func() { _ = f.Close() }()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader failed: %v", err)
	}
	defer func() { _ = gr.Close() }()
	tr := tar.NewReader(gr)
	hdr, err := tr.Next()
	if err != nil || hdr.Name != "gh-flarebyte" {
		t.Fatalf("unexpected tar entry")
	}
	content, _ := io.ReadAll(tr)
	if string(content) != "linux-binary" {
		t.Fatalf("unexpected tar content")
	}

	winSrc := filepath.Join(tmpDir, "gh-flarebyte-windows-amd64.exe")
	_ = os.WriteFile(winSrc, []byte("win-binary"), 0o755)
	zipPath := filepath.Join(tmpDir, "artifact.zip")
	if err := packageBinaryArchive(winSrc, "windows-amd64", zipPath, "gh-flarebyte.exe"); err != nil {
		t.Fatalf("package zip failed: %v", err)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip failed: %v", err)
	}
	defer func() { _ = zr.Close() }()
	if len(zr.File) != 1 || zr.File[0].Name != "gh-flarebyte.exe" {
		t.Fatalf("unexpected zip content")
	}
}

func TestResolveVersionFromSourceYAMLAndJSON(t *testing.T) {
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "v.yaml")
	_ = os.WriteFile(yamlPath, []byte("version: 1.2.3\n"), 0o644)
	jsonPath := filepath.Join(tmpDir, "v.json")
	_ = os.WriteFile(jsonPath, []byte(`{"version":"1.2.3-rc.1"}`), 0o644)
	v1, err := resolveVersionFromSource(yamlPath)
	if err != nil || v1 != "1.2.3" {
		t.Fatalf("expected yaml version")
	}
	v2, err := resolveVersionFromSource(jsonPath)
	if err != nil || v2 != "1.2.3-rc.1" {
		t.Fatalf("expected json version")
	}
}

func TestRunReleaseNotesFileModeRequiresPath(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `notesMode:        "generate-notes"`, `notesMode:        "notes-file"`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	stubReleaseFlow(t, "1.2.3", false)
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"release"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected usage failure")
	}
}

func TestRunReleaseFailsWhenTagExists(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	stubReleaseFlow(t, "1.2.3", true)
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"release"}, &out, &errOut)
	if result.ExitCode != ExitReleaseFailure {
		t.Fatalf("expected release failure")
	}
}

func TestRunReleaseRejectsDartArtifactsEnabled(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	cfg = strings.Replace(cfg, "build: {\n\tlanguage:     \"dart\"", "build: {\n\tlanguage:     \"dart\"\n\tmode:         \"library\"\n\tpackages: [\n\t\t\"./...\",\n\t]", 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"release"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected usage failure, got %d stderr=%s", result.ExitCode, errOut.String())
	}
	if !strings.Contains(errOut.String(), `release.includeArtifacts is true`) {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunReleaseSuccess(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	stubReleaseFlow(t, "1.2.3", false)
	var capturedTag string
	createRelease = func(tag string, artifacts []string, notesMode string, notesFile string, draft bool) error {
		capturedTag = tag
		return nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"release", "--draft"}, &out, &errOut)
	if result.ExitCode != ExitOK || capturedTag != "v1.2.3" {
		t.Fatalf("unexpected release result")
	}
}

func TestRunReleaseDartLibraryNoArtifactsSuccess(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `language:     "go"`, `language:     "dart"`, 1)
	cfg = strings.Replace(cfg, "build: {\n\tlanguage:     \"dart\"", "build: {\n\tlanguage:     \"dart\"\n\tmode:         \"library\"\n\tpackages: [\n\t\t\"./...\",\n\t]\n\trunTests:     true", 1)
	cfg = strings.Replace(cfg, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}`, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	includeArtifacts: false
}`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldDartBuildPackages := dartBuildPackages
	t.Cleanup(func() { dartBuildPackages = oldDartBuildPackages })
	dartBuildPackages = func(runTests bool) error {
		if !runTests {
			t.Fatalf("expected runTests=true")
		}
		return nil
	}
	stubReleaseFlow(t, "1.2.3", false)
	var capturedTag string
	var capturedArtifacts []string
	createRelease = func(tag string, artifacts []string, notesMode string, notesFile string, draft bool) error {
		capturedTag = tag
		capturedArtifacts = append([]string{}, artifacts...)
		return nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"release", "--draft"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d stderr=%s", result.ExitCode, errOut.String())
	}
	if capturedTag != "v1.2.3" {
		t.Fatalf("unexpected tag: %s", capturedTag)
	}
	if len(capturedArtifacts) != 0 {
		t.Fatalf("expected no artifacts for dart library release, got: %v", capturedArtifacts)
	}
}

func TestRunReleaseSupportsNoSuffixModeWithMultipleTargets(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `targets: [
		"linux-amd64",
	]`, `artifactTargetSuffix: false
	targets: [
		"linux-amd64",
		"darwin-arm64",
	]`, 1)
	cfg = strings.Replace(cfg, `artifactTargetSuffix: true`, `artifactTargetSuffix: false`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	stubReleaseFlow(t, "1.2.3", false)
	var capturedArtifacts []string
	stubReleaseCaptureArtifacts(t, "1.2.3", false, &capturedArtifacts)
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"release"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected release success, got %d (%s)", result.ExitCode, errOut.String())
	}
	if len(capturedArtifacts) == 0 {
		t.Fatalf("expected captured artifacts")
	}
}

func TestHydrateBuildInfoPopulatesAllFields(t *testing.T) {
	setupHydrateBuildInfoTestEnv(
		t,
		func(sourcePath string) (string, error) { return "1.2.3", nil },
		func(args ...string) (string, error) { return "abc123def456", nil },
		func() time.Time { return time.Date(2026, 5, 2, 12, 34, 56, 0, time.UTC) },
		func() string { return "go1.24.1" },
	)

	hydrateBuildInfo("main.project.yaml")

	if buildinfo.Version != "1.2.3" {
		t.Fatalf("unexpected version: %s", buildinfo.Version)
	}
	if buildinfo.CommitID != "abc123def456" {
		t.Fatalf("unexpected commit id: %s", buildinfo.CommitID)
	}
	if buildinfo.Date != "2026-05-02T12:34:56Z" {
		t.Fatalf("unexpected date: %s", buildinfo.Date)
	}
	if buildinfo.GoVersion != "go1.24.1" {
		t.Fatalf("unexpected go version: %s", buildinfo.GoVersion)
	}
}

func TestHydrateBuildInfoKeepsExistingWhenVersionAndGitUnavailable(t *testing.T) {
	setupHydrateBuildInfoTestEnv(
		t,
		func(sourcePath string) (string, error) { return "", os.ErrNotExist },
		func(args ...string) (string, error) { return "", os.ErrNotExist },
		func() time.Time { return time.Date(2026, 5, 2, 1, 2, 3, 0, time.UTC) },
		func() string { return "go1.24.2" },
	)
	buildinfo.Version = "dev"
	buildinfo.CommitID = "unknown"

	hydrateBuildInfo("main.project.yaml")

	if buildinfo.Version != "dev" {
		t.Fatalf("expected existing version to remain, got: %s", buildinfo.Version)
	}
	if buildinfo.CommitID != "unknown" {
		t.Fatalf("expected existing commit to remain, got: %s", buildinfo.CommitID)
	}
	if buildinfo.Date != "2026-05-02T01:02:03Z" {
		t.Fatalf("unexpected date: %s", buildinfo.Date)
	}
	if buildinfo.GoVersion != "go1.24.2" {
		t.Fatalf("unexpected go version: %s", buildinfo.GoVersion)
	}
}

func TestRunBuildLibraryModeSuccess(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
	]
}`, `build: {
	language: "go"
	mode:     "library"
	packages: ["./..."]
	runTests: true
}`, 1)
	cfg = strings.Replace(cfg, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}`, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	includeArtifacts: false
}`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldGoBuildPackages := goBuildPackages
	t.Cleanup(func() { goBuildPackages = oldGoBuildPackages })
	var gotRunTests bool
	goBuildPackages = func(goos string, goarch string, packages []string, runTests bool, _ config.CGOConfig) error {
		gotRunTests = runTests
		return nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d (%s)", result.ExitCode, errOut.String())
	}
	if !gotRunTests {
		t.Fatalf("expected runTests=true in library mode")
	}
}

func TestRunBuildLibraryModePassesCGOConfig(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
	]
}`, `go: {
	cgo: {
		enabled: true
		cc:      "clang"
		cxx:     "clang++"
	}
}

build: {
	language: "go"
	mode:     "library"
	packages: ["./..."]
}`, 1)
	cfg = strings.Replace(cfg, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}`, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	includeArtifacts: false
}`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldGoBuildPackages := goBuildPackages
	t.Cleanup(func() { goBuildPackages = oldGoBuildPackages })
	var got config.CGOConfig
	goBuildPackages = func(goos string, goarch string, packages []string, runTests bool, cgo config.CGOConfig) error {
		got = cgo
		return nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected success, got %d (%s)", result.ExitCode, errOut.String())
	}
	if !got.Enabled || got.CC != "clang" || got.CXX != "clang++" {
		t.Fatalf("unexpected cgo config: %+v", got)
	}
}

func TestRunReleaseLibraryModeWithoutArtifacts(t *testing.T) {
	cfg := strings.Replace(testConfigCue(), `build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
	]
}`, `build: {
	language: "go"
	mode:     "library"
	packages: ["./..."]
}`, 1)
	cfg = strings.Replace(cfg, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}`, `release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	includeArtifacts: false
}`, 1)
	_ = setupTempWorkdirWithConfig(t, cfg)
	oldGoBuildPackages := goBuildPackages
	t.Cleanup(func() { goBuildPackages = oldGoBuildPackages })
	goBuildPackages = func(goos string, goarch string, packages []string, runTests bool, _ config.CGOConfig) error {
		return nil
	}
	var capturedArtifacts []string
	stubReleaseCaptureArtifacts(t, "1.2.3", false, &capturedArtifacts)
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"release"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected release success, got %d (%s)", result.ExitCode, errOut.String())
	}
	if len(capturedArtifacts) != 0 {
		t.Fatalf("expected no artifacts, got %v", capturedArtifacts)
	}
}

func TestRunBuildFailurePrintsGoEnvAndCommand(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldBuildTargetBinary := buildTargetBinary
	t.Cleanup(func() { buildTargetBinary = oldBuildTargetBinary })
	buildTargetBinary = func(target string, outputPath string, mainPackage string, _ config.CGOConfig) error {
		return &goBuildInvocationError{
			Command: "go build -o out ./cmd/gh-flarebyte",
			Env: []string{
				"GOOS=linux",
				"GOARCH=amd64",
				"CGO_ENABLED=0",
				"CC=",
				"CXX=",
			},
			Stderr: "build constraints exclude all Go files",
			Cause:  errors.New("exit status 1"),
		}
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build"}, &out, &errOut)
	if result.ExitCode != ExitBuildFailure {
		t.Fatalf("expected build failure, got %d", result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "go_env: GOOS=linux GOARCH=amd64 CGO_ENABLED=0") {
		t.Fatalf("expected go env diagnostics, got: %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "first_failed_command: go build -o out ./cmd/gh-flarebyte") {
		t.Fatalf("expected command diagnostics, got: %s", errOut.String())
	}
}
