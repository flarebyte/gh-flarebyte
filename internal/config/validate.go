// purpose: Enforce semantic and structural validity rules for loaded gh-flarebyte configuration.
// responsibilities: Validate required fields/enums; enforce cross-field constraints; return user-actionable error messages.
// architecture notes: Validation is intentionally centralized so every command using config.Load shares identical rule enforcement and failure wording.
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
	switch cfg.DevOutput.Color {
	case "auto", "true", "false":
	default:
		return fmt.Errorf("invalid dev_output.color %q: expected true, false, or auto", cfg.DevOutput.Color)
	}
	switch cfg.DevOutput.Style {
	case "summary", "per_test":
	default:
		return fmt.Errorf("invalid dev_output.style %q: expected summary or per_test", cfg.DevOutput.Style)
	}
	if cfg.Coverage.DefaultMinPercent != nil {
		if *cfg.Coverage.DefaultMinPercent < 0 || *cfg.Coverage.DefaultMinPercent > 100 {
			return fmt.Errorf("invalid coverage.default_min_percent %.2f: expected between 0 and 100", *cfg.Coverage.DefaultMinPercent)
		}
	}
	switch cfg.Build.Language {
	case "go", "dart":
	default:
		return fmt.Errorf("invalid build.language %q: expected go or dart", cfg.Build.Language)
	}
	switch cfg.Build.Mode {
	case "binary":
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
	case "library":
		if len(cfg.Build.Packages) == 0 {
			return errors.New("invalid build.packages: at least one package pattern is required in library mode")
		}
	default:
		return fmt.Errorf("invalid build.mode %q: expected binary or library", cfg.Build.Mode)
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
	if cfg.Release.IncludeArtifacts {
		if cfg.Release.ArtifactDir == "" {
			return errors.New("invalid release.artifactDir: value is required when release.includeArtifacts is true")
		}
		if cfg.Build.Mode == "library" {
			return errors.New(`invalid config: build.mode is "library" and release.includeArtifacts is true. Use build.mode: "library" with release.includeArtifacts: false`)
		}
	}
	return nil
}
