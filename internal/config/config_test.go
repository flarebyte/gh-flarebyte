package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "valid.cue"))
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
	if cfg.Project.Org != "flarebyte" {
		t.Fatalf("unexpected org: %s", cfg.Project.Org)
	}
	if len(cfg.Build.Targets) != 2 {
		t.Fatalf("expected two build targets, got %d", len(cfg.Build.Targets))
	}
	if !cfg.Build.ArtifactTargetSuffix {
		t.Fatalf("expected default build.artifactTargetSuffix=true")
	}
}

func TestLoadInvalidBuildTarget(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "invalid-target.cue"))
	if err == nil {
		t.Fatalf("expected invalid build target error")
	}
	if !strings.Contains(err.Error(), "invalid build.targets entry") {
		t.Fatalf("expected target validation error, got: %v", err)
	}
}

func TestLoadInvalidNotesFilePath(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "invalid-notes-file-path.cue"))
	if err == nil {
		t.Fatalf("expected invalid notes file path error")
	}
	if !strings.Contains(err.Error(), "release.releaseNotesFilePath") {
		t.Fatalf("expected notes-file validation error, got: %v", err)
	}
}

func TestLoadBuildArtifactTargetSuffixFalseWithMultipleTargets(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "valid-artifact-suffix.cue")
	content := `package ghflarebyte

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
	topics: ["gh-extension"]
	labels: [{name: "bug", color: "B60205"}]
}

build: {
	language:             "go"
	outputDir:            "build"
	checksumFile:         "build/checksums.txt"
	artifactTargetSuffix: false
	targets: [
		"linux-amd64",
		"windows-amd64",
	]
}

release: {
	versionSource:        "main.project.yaml"
	tagPrefix:            "v"
	notesMode:            "generate-notes"
	artifactDir:          "build"
	includeChecksums:     true
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config failed: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	if cfg.Build.ArtifactTargetSuffix {
		t.Fatalf("expected build.artifactTargetSuffix=false")
	}
}

func TestLoadLibraryModeConfig(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "valid-library.cue")
	content := `package ghflarebyte

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
	topics: ["gh-extension"]
	labels: [{name: "bug", color: "B60205"}]
}

build: {
	language: "go"
	mode:     "library"
	packages: ["./..."]
	runTests: true
}

release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	includeArtifacts: false
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config failed: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	if cfg.Build.Mode != "library" {
		t.Fatalf("expected build.mode=library, got %s", cfg.Build.Mode)
	}
	if !cfg.Build.RunTests {
		t.Fatalf("expected build.runTests=true")
	}
	if cfg.Release.IncludeArtifacts {
		t.Fatalf("expected release.includeArtifacts=false")
	}
}

func TestLoadRejectsLibraryModeWithArtifactsEnabled(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "invalid-library-artifacts.cue")
	content := `package ghflarebyte

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
	topics: ["gh-extension"]
	labels: [{name: "bug", color: "B60205"}]
}

build: {
	language: "go"
	mode:     "library"
	packages: ["./..."]
}

release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	includeArtifacts: true
	artifactDir:      "build"
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config failed: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected invalid config error")
	}
	if !strings.Contains(err.Error(), `build.mode is "library"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
