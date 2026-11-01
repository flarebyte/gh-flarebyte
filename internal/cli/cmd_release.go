// purpose: Implement release publishing by composing build outputs, version resolution, and GitHub release creation.
// responsibilities: Parse release flags; trigger build; resolve tag/version; collect artifacts; create release with selected notes mode.
// architecture notes: Release always shells through the build command first so artifact shape and checksum behavior stay consistent with standalone build execution.
package cli

import (
	"fmt"
	"io"
)

func runRelease(draft bool, notesFileOverride string, stdout, stderr io.Writer) Result {
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	if cfg.Build.Language == "dart" && cfg.Release.IncludeArtifacts {
		err := fmt.Errorf(`invalid config: build.language is "dart" and release.includeArtifacts is true. Use release.includeArtifacts: false for Dart projects`)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	var freshArtifacts []string
	if cfg.Build.Language == "python" || cfg.Build.Language == "rust" || cfg.Build.Language == "swift" {
		var err error
		if cfg.Build.Language == "swift" {
			freshArtifacts, err = buildSwift(cfg, "", "", stderr)
		} else if cfg.Build.Language == "rust" {
			freshArtifacts, err = buildRust(cfg, "", "", stderr)
		} else {
			freshArtifacts, err = buildPythonPackage(cfg, "", stderr)
		}
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
	} else {
		buildRes := Run([]string{"build"}, io.Discard, stderr)
		if buildRes.ExitCode != ExitOK {
			if buildRes.ExitCode == ExitBuildFailure {
				return buildRes
			}
			return Result{ExitCode: ExitReleaseFailure, Err: buildRes.Err}
		}
	}
	version, err := findVersion(cfg.Release.VersionSource)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	tag := cfg.Release.TagPrefix + version
	exists, err := tagExists(tag)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitReleaseFailure, Err: err}
	}
	if exists {
		err := fmt.Errorf("release tag %s already exists. refusing to mutate existing release implicitly", tag)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitReleaseFailure, Err: err}
	}
	artifacts := make([]string, 0)
	if cfg.Release.IncludeArtifacts {
		if cfg.Build.Language == "python" || cfg.Build.Language == "rust" || cfg.Build.Language == "swift" {
			artifacts, err = stageFreshRelease(freshArtifacts, cfg.Build.OutputDir, cfg.Release.ArtifactDir, cfg.Release.IncludeChecksums)
		} else {
			artifacts, err = listReleaseArtifacts(cfg.Release.ArtifactDir, cfg.Release.IncludeChecksums, cfg.Project.Repo, cfg.Build.ArtifactTargetSuffix)
		}
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitReleaseFailure, Err: err}
		}
		if err := ensureUniqueArtifactBasenames(artifacts); err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
	}
	notesFile := ""
	switch cfg.Release.NotesMode {
	case "generate-notes":
	case "notes-from-tag":
	case "notes-file":
		notesFile = cfg.Release.ReleaseNotesFilePath
		if notesFileOverride != "" {
			notesFile = notesFileOverride
		}
		if notesFile == "" {
			err := fmt.Errorf("release notes mode is notes-file but no notes file was provided. Set release.releaseNotesFilePath or pass --notes-file")
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
	default:
		err := fmt.Errorf("invalid release.notesMode %q", cfg.Release.NotesMode)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	if err := createRelease(tag, artifacts, cfg.Release.NotesMode, notesFile, draft); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitReleaseFailure, Err: err}
	}
	if cfg.Release.IncludeArtifacts {
		_, _ = fmt.Fprintf(stdout, "Release %s published from %s with checksums %s.\n", tag, cfg.Release.ArtifactDir, ternary(cfg.Release.IncludeChecksums, "attached", "skipped"))
	} else {
		_, _ = fmt.Fprintf(stdout, "Release %s published without artifacts.\n", tag)
	}
	return Result{ExitCode: ExitOK}
}
