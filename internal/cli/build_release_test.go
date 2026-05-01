package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	oldBuildTargetBinary := buildTargetBinary
	oldPackageBinary := packageBinary
	t.Cleanup(func() {
		buildTargetBinary = oldBuildTargetBinary
		packageBinary = oldPackageBinary
	})
	buildTargetBinary = func(target string, outputPath string) error {
		return os.WriteFile(outputPath, []byte("binary-"+target), 0o755)
	}
	packageBinary = packageBinaryArchive
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
	oldBuildTargetBinary := buildTargetBinary
	oldPackageBinary := packageBinary
	t.Cleanup(func() {
		buildTargetBinary = oldBuildTargetBinary
		packageBinary = oldPackageBinary
	})
	buildTargetBinary = func(target string, outputPath string) error {
		return os.WriteFile(outputPath, []byte("binary-"+target), 0o755)
	}
	packageBinary = packageBinaryArchive
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build", "--output-dir", "dist"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (%s)", ExitOK, result.ExitCode, errOut.String())
	}
	if _, err := os.Stat(filepath.Join("dist", "linux-amd64", "gh-flarebyte.tar.gz")); err != nil {
		t.Fatalf("expected linux artifact in target subdir: %v", err)
	}
	if _, err := os.Stat(filepath.Join("dist", "darwin-arm64", "gh-flarebyte.tar.gz")); err != nil {
		t.Fatalf("expected darwin artifact in target subdir: %v", err)
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

func TestRunReleaseFailsWithDuplicateArtifactBasenames(t *testing.T) {
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
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"release"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected usage failure for duplicate basenames, got %d", result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "duplicate filename") {
		t.Fatalf("expected duplicate filename message, got: %s", errOut.String())
	}
}
