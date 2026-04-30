package config

import (
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
