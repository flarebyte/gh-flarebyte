// purpose: Guard config loading and validation contracts so cue-backed project/build/release behavior remains deterministic.
// responsibilities: Check valid parsing defaults and reject invalid field combinations, enums, and required-field omissions.
// architecture notes: Uses temp cue fixtures to exercise loader and validator together, mirroring real CLI config consumption.
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
	if cfg.Build.MainPackage != "./cmd/gh-flarebyte" {
		t.Fatalf("expected default build.mainPackage to be ./cmd/gh-flarebyte, got %q", cfg.Build.MainPackage)
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

func TestLoadBuildMainPackageOverride(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "valid-main-package.cue")
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
	language:     "go"
	mainPackage:  "./cmd/flyb"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: ["linux-amd64"]
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
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	if cfg.Build.MainPackage != "./cmd/flyb" {
		t.Fatalf("expected build.mainPackage override, got %q", cfg.Build.MainPackage)
	}
}

func TestLoadDefaultsForDevOutputAndCoverage(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "valid.cue"))
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
	if cfg.DevOutput.Color != "auto" {
		t.Fatalf("expected devOutput.color default auto, got %q", cfg.DevOutput.Color)
	}
	if cfg.DevOutput.Style != "summary" {
		t.Fatalf("expected devOutput.style default summary, got %q", cfg.DevOutput.Style)
	}
	if !cfg.DevOutput.ShowPassed {
		t.Fatalf("expected devOutput.showPassed default true")
	}
	if !cfg.Coverage.FailBelowMin {
		t.Fatalf("expected coverage.enforceMin default true")
	}
	if cfg.Coverage.DefaultMinPercent != nil {
		t.Fatalf("expected nil default coverage.min")
	}
}

func TestLoadRejectsInvalidDevOutputStyle(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "invalid-dev-output-style.cue")
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

devOutput: {
	color: "auto"
	style: "verbose"
}

build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: ["linux-amd64"]
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
		t.Fatalf("expected invalid devOutput.style error")
	}
	if !strings.Contains(err.Error(), "devOutput.style") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsUnknownTopLevelField(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "invalid-unknown-top-level.cue")
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
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: ["linux-amd64"]
}

release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}

extraField: "hello"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config failed: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected unknown top-level field error")
	}
	if !strings.Contains(err.Error(), `unknown field "extraField" in top-level`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsUnknownBuildField(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "invalid-unknown-build-field.cue")
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
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: ["linux-amd64"]
	extraBuild:   true
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
		t.Fatalf("expected unknown build field error")
	}
	if !strings.Contains(err.Error(), `unknown field "extraBuild" in build`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadGoCGOConfig(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "valid-go-cgo.cue")
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

go: {
	toolchain: "local"
	cgo: {
		enabled: true
		cc:      "clang"
		cxx:     "clang++"
	}
}

build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: ["linux-amd64"]
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
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	if !cfg.Go.CGO.Enabled {
		t.Fatalf("expected go.cgo.enabled=true")
	}
	if cfg.Go.CGO.CC != "clang" || cfg.Go.CGO.CXX != "clang++" {
		t.Fatalf("unexpected go.cgo compiler fields: %+v", cfg.Go.CGO)
	}
}

func TestLoadRejectsUnknownGoCGOField(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "invalid-go-cgo-unknown-field.cue")
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

go: {
	cgo: {
		enabled: true
		compiler: "clang"
	}
}

build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: ["linux-amd64"]
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
		t.Fatalf("expected invalid go.cgo unknown field error")
	}
	if !strings.Contains(err.Error(), `unknown field "compiler" in go.cgo`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
