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
	buildRes := Run([]string{"build"}, io.Discard, stderr)
	if buildRes.ExitCode != ExitOK {
		if buildRes.ExitCode == ExitBuildFailure {
			return buildRes
		}
		return Result{ExitCode: ExitReleaseFailure, Err: buildRes.Err}
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
		artifacts, err = listReleaseArtifacts(cfg.Release.ArtifactDir, cfg.Release.IncludeChecksums, cfg.Project.Repo, cfg.Build.ArtifactTargetSuffix)
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
		_, _ = fmt.Fprintf(stdout, "Release %s published without binary artifacts.\n", tag)
	}
	return Result{ExitCode: ExitOK}
}
