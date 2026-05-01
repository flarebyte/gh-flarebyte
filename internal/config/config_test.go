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

func TestLoadInvalidArtifactTargetSuffixWithoutUniqueTarget(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "invalid-artifact-suffix.cue")
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
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config failed: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected invalid artifactTargetSuffix error")
	}
	if !strings.Contains(err.Error(), "artifactTargetSuffix") {
		t.Fatalf("expected artifactTargetSuffix validation error, got: %v", err)
	}
}
