package cli

import (
	"os"
	"testing"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func setupTempWorkdirWithConfig(t *testing.T, cfg string) string {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if cfg != "" {
		if err := os.WriteFile(".gh-flarebyte.cue", []byte(cfg), 0o644); err != nil {
			t.Fatalf("write config failed: %v", err)
		}
	}
	return tmpDir
}

func stubBuildArtifacts(t *testing.T) {
	t.Helper()
	oldBuildTargetBinary := buildTargetBinary
	oldPackageBinary := packageBinary
	t.Cleanup(func() {
		buildTargetBinary = oldBuildTargetBinary
		packageBinary = oldPackageBinary
	})
	buildTargetBinary = func(target string, outputPath string, mainPackage string, _ config.CGOConfig) error {
		return os.WriteFile(outputPath, []byte("bin"), 0o755)
	}
	packageBinary = func(binaryPath, target, artifactPath, archiveBinaryName string) error {
		return os.WriteFile(artifactPath, []byte("pkg"), 0o644)
	}
}

func stubReleaseFlow(t *testing.T, version string, tagAlreadyExists bool) {
	t.Helper()
	oldFindVersion := findVersion
	oldTagExists := tagExists
	oldCreateRelease := createRelease
	t.Cleanup(func() {
		findVersion = oldFindVersion
		tagExists = oldTagExists
		createRelease = oldCreateRelease
	})
	stubBuildArtifacts(t)
	findVersion = func(sourcePath string) (string, error) { return version, nil }
	tagExists = func(tag string) (bool, error) { return tagAlreadyExists, nil }
	createRelease = func(tag string, artifacts []string, notesMode string, notesFile string, draft bool) error {
		return nil
	}
}

func baseRepoMetadata() RepoMetadata {
	return defaultRepoMetadata("flarebyte/gh-flarebyte")
}

func stubRepoInitIO(t *testing.T, exists bool, readFn func(repo string) (RepoMetadata, error)) {
	t.Helper()
	oldReadRepoMetadata := readRepoMetadata
	oldFileExists := fileExists
	oldWriteFile := writeFile
	t.Cleanup(func() {
		readRepoMetadata = oldReadRepoMetadata
		fileExists = oldFileExists
		writeFile = oldWriteFile
	})
	readRepoMetadata = readFn
	fileExists = func(path string) bool { return exists }
	writeFile = func(path string, data []byte) error {
		return os.WriteFile(path, data, 0o644)
	}
}

func testConfigCue() string {
	return `package ghflarebyte

project: {
	org:  "flarebyte"
	repo: "gh-flarebyte"
}

sync: {
	mode: "push"
	}

repository: {
	description:   "CLI for landing your git commands right"
	defaultBranch: "main"
	homepage:      "https://github.com/flarebyte/gh-flarebyte"
	visibility:    "public"
	template:      false
	topics: [
		"gh-extension",
		"github-cli",
		"git",
		"flarebyte",
	]
	labels: [
		{
			name:        "bug"
			color:       "B60205"
			description: "Something is broken"
		},
		{
			name:        "enhancement"
			color:       "0E8A16"
			description: "New feature"
		},
	]
}

build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
	]
}

release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}
`
}
