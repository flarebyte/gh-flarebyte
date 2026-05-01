package config

import (
	"errors"
	"fmt"
)

func Validate(cfg Config) error {
	if cfg.Project.Org == "" {
		return errors.New("invalid project.org: value is required")
	}
	if cfg.Project.Repo == "" {
		return errors.New("invalid project.repo: value is required")
	}
	if cfg.Repository.DefaultBranch == "" {
		return errors.New("invalid repository.defaultBranch: value is required")
	}
	switch cfg.Repository.Visibility {
	case "public", "private", "internal":
	default:
		return fmt.Errorf("invalid repository.visibility %q: expected public, private, or internal", cfg.Repository.Visibility)
	}
	if len(cfg.Repository.Topics) == 0 {
		return errors.New("invalid repository.topics: at least one topic is required")
	}
	if len(cfg.Repository.Labels) == 0 {
		return errors.New("invalid repository.labels: at least one label is required")
	}
	for _, label := range cfg.Repository.Labels {
		if label.Name == "" {
			return errors.New("invalid repository.labels.name: value is required")
		}
		if label.Color == "" {
			return fmt.Errorf("invalid repository.labels[%s].color: value is required", label.Name)
		}
	}
	if cfg.Sync.Mode != "push" {
		return fmt.Errorf("invalid sync.mode %q: expected push", cfg.Sync.Mode)
	}
	switch cfg.Build.Language {
	case "go", "dart":
	default:
		return fmt.Errorf("invalid build.language %q: expected go or dart", cfg.Build.Language)
	}
	if cfg.Build.OutputDir == "" {
		return errors.New("invalid build.outputDir: value is required")
	}
	if cfg.Build.ChecksumFile == "" {
		return errors.New("invalid build.checksumFile: value is required")
	}
	if len(cfg.Build.Targets) == 0 {
		return errors.New("invalid build.targets: at least one target is required")
	}
	for _, target := range cfg.Build.Targets {
		if !targetPattern.MatchString(target) {
			return fmt.Errorf("invalid build.targets entry %q: expected os-arch format such as linux-amd64 or windows-amd64", target)
		}
	}
	switch cfg.Release.NotesMode {
	case "generate-notes", "notes-from-tag":
	case "notes-file":
		if cfg.Release.ReleaseNotesFilePath == "" {
			return errors.New("invalid release.releaseNotesFilePath: value is required when release.notesMode is notes-file")
		}
	default:
		return fmt.Errorf("invalid release.notesMode %q: expected generate-notes, notes-from-tag, or notes-file", cfg.Release.NotesMode)
	}
	if cfg.Release.VersionSource == "" {
		return errors.New("invalid release.versionSource: value is required")
	}
	if cfg.Release.TagPrefix == "" {
		return errors.New("invalid release.tagPrefix: value is required")
	}
	if cfg.Release.ArtifactDir == "" {
		return errors.New("invalid release.artifactDir: value is required")
	}
	return nil
}
