package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func handleConfigValidate(args []string, stdout, stderr io.Writer) Result {
	configPath, err := parseConfigPath(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	absPath, err := config.ResolvePath(configPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	if _, err := config.Load(configPath); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	_, _ = fmt.Fprintf(stdout, "config is valid: %s\n", absPath)
	return Result{ExitCode: ExitOK}
}

func parseConfigPath(args []string) (string, error) {
	configPath := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" {
			if i+1 >= len(args) {
				return "", errors.New("invalid invocation: --config requires a path")
			}
			configPath = args[i+1]
			i++
			continue
		}
		return "", fmt.Errorf("invalid invocation: unknown argument %q", arg)
	}
	return configPath, nil
}

func handleRepoInit(args []string, stdout, stderr io.Writer) Result {
	repo, overwrite, err := parseRepoInitArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	if repo == "" {
		err := fmt.Errorf("invalid invocation: --repo owner/name is required")
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	if fileExists(config.DefaultPath) && !overwrite {
		err := fmt.Errorf("%s already exists in %s. Re-run with --overwrite if replacing it is intentional", config.DefaultPath, mustAbs("."))
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	owner, name, err := splitRepo(repo)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}

	meta := defaultRepoMetadata(repo)
	imported := false
	if m, err := readRepoMetadata(repo); err == nil {
		meta = m
		imported = true
	} else {
		_, _ = fmt.Fprintf(stderr, "warning: unable to import remote metadata for %s, using defaults (%v)\n", repo, err)
	}

	content := renderCueConfig(owner, name, meta)
	if err := writeFile(config.DefaultPath, []byte(content)); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	absPath := mustAbs(config.DefaultPath)
	if imported {
		_, _ = fmt.Fprintf(stdout, "created %s from current GitHub state. Next: review the config, then run gh flarebyte repo update.\n", absPath)
	} else {
		_, _ = fmt.Fprintf(stdout, "created %s from defaults. Next: review the config, then run gh flarebyte repo update.\n", absPath)
	}
	return Result{ExitCode: ExitOK}
}

func handleRepoAudit(args []string, stdout, stderr io.Writer) Result {
	repo, asJSON, err := parseRepoAuditArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	cfg, err := config.Load("")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	if repo == "" {
		repo = fmt.Sprintf("%s/%s", cfg.Project.Org, cfg.Project.Repo)
	}
	remote, err := readRepoMetadata(repo)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	report := buildAuditReport(repo, cfg, remote)
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(report); err != nil {
			return Result{ExitCode: ExitFailure, Err: err}
		}
	} else if report.HasDrift {
		_, _ = fmt.Fprintf(stdout, "%d differences found. Review the drift report below, then run gh flarebyte repo update when ready to apply changes.\n", report.DriftCount)
		for _, diff := range report.Diffs {
			_, _ = fmt.Fprintf(stdout, "- %s local=%v remote=%v\n", diff.Field, diff.Local, diff.Remote)
		}
	} else {
		_, _ = fmt.Fprintln(stdout, "No drift found. GitHub matches .gh-flarebyte.cue.")
	}
	if report.HasDrift {
		return Result{ExitCode: ExitDrift}
	}
	return Result{ExitCode: ExitOK}
}

func handleRepoUpdate(args []string, stdout, stderr io.Writer) Result {
	repo, confirmDeletions, acceptVisibility, err := parseRepoUpdateArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	cfg, err := config.Load("")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	if repo == "" {
		repo = fmt.Sprintf("%s/%s", cfg.Project.Org, cfg.Project.Repo)
	}
	remote, err := readRepoMetadata(repo)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	plan := buildUpdatePlan(cfg, remote)
	if plan.VisibilityChange && !acceptVisibility {
		err := fmt.Errorf("visibility would change from %s to %s. Re-run with --accept-visibility-change-consequences if that is intentional", remote.Visibility, cfg.Repository.Visibility)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitBlockedVisibility, Err: err}
	}
	if (len(plan.TopicsToRemove) > 0 || len(plan.LabelsToDelete) > 0) && !confirmDeletions {
		err := fmt.Errorf("update would delete %d labels and %d topics. Re-run with --confirm-deletions if that is intentional", len(plan.LabelsToDelete), len(plan.TopicsToRemove))
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitBlockedDeletions, Err: err}
	}

	settingsUpdated := 0
	topicsSynced := 0
	labelsReconciled := 0
	appliedSettings := false
	appliedTopics := false

	if plan.SettingsChanged {
		if err := applyRepoSettings(repo, plan.SettingsPatch); err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitFailure, Err: err}
		}
		settingsUpdated = plan.SettingsChangeCount
		appliedSettings = true
	}
	for _, topic := range plan.TopicsToAdd {
		if err := addRepoTopic(repo, topic); err != nil {
			return partialUpdateFailure(stderr, err, appliedSettings, appliedTopics)
		}
	}
	for _, topic := range plan.TopicsToRemove {
		if err := removeRepoTopic(repo, topic); err != nil {
			return partialUpdateFailure(stderr, err, appliedSettings, appliedTopics)
		}
	}
	if len(plan.TopicsToAdd) > 0 || len(plan.TopicsToRemove) > 0 {
		appliedTopics = true
	}
	topicsSynced = len(cfg.Repository.Topics)

	for _, label := range plan.LabelsToCreate {
		if err := createRepoLabel(repo, label); err != nil {
			return partialUpdateFailure(stderr, err, appliedSettings, appliedTopics)
		}
	}
	for _, label := range plan.LabelsToUpdate {
		if err := updateRepoLabel(repo, label); err != nil {
			return partialUpdateFailure(stderr, err, appliedSettings, appliedTopics)
		}
	}
	for _, label := range plan.LabelsToDelete {
		if err := deleteRepoLabel(repo, label.Name); err != nil {
			return partialUpdateFailure(stderr, err, appliedSettings, appliedTopics)
		}
	}
	labelsReconciled = len(cfg.Repository.Labels)
	_, _ = fmt.Fprintf(stdout, "Update complete: %d repo settings updated, %d topics synced, %d labels reconciled.\n", settingsUpdated, topicsSynced, labelsReconciled)
	return Result{ExitCode: ExitOK}
}

func handleReposMine(args []string, stdout, stderr io.Writer) Result {
	org, asJSON, err := parseReposMineArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	if org == "" {
		err := fmt.Errorf("invalid invocation: --org is required")
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	contributor, repos, err := readReposMine(org)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	report := ReposMineReport{
		Org:         org,
		Contributor: contributor,
		Count:       len(repos),
		Repos:       repos,
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(report); err != nil {
			return Result{ExitCode: ExitFailure, Err: err}
		}
		return Result{ExitCode: ExitOK}
	}
	_, _ = fmt.Fprintf(stdout, "Found %d repositories in org %s for contributor %s.\n", report.Count, report.Org, report.Contributor)
	for _, repo := range report.Repos {
		_, _ = fmt.Fprintf(stdout, "- %s/%s (%s, default branch: %s)\n", repo.Owner, repo.Name, repo.Visibility, repo.DefaultBranch)
	}
	return Result{ExitCode: ExitOK}
}

func handleBuild(args []string, stdout, stderr io.Writer) Result {
	targetFilter, outputDirOverride, err := parseBuildArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	cfg, err := config.Load("")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	if cfg.Build.Language != "go" {
		err := fmt.Errorf("build.language %q is not supported yet. Supported values: go", cfg.Build.Language)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	targets := cfg.Build.Targets
	if targetFilter != "" {
		if !contains(targets, targetFilter) {
			err := fmt.Errorf("invalid --target %q: not present in build.targets", targetFilter)
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		targets = []string{targetFilter}
	}
	outputDir := cfg.Build.OutputDir
	if outputDirOverride != "" {
		outputDir = outputDirOverride
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	tmpDir := filepath.Join(outputDir, ".tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	type artifactDigest struct {
		Name string
		SHA  string
	}
	digests := make([]artifactDigest, 0, len(targets))
	for _, target := range targets {
		goos, goarch, err := splitTarget(target)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		binBase := fmt.Sprintf("%s-%s-%s", cfg.Project.Repo, goos, goarch)
		binName := binBase
		if goos == "windows" {
			binName += ".exe"
		}
		binPath := filepath.Join(tmpDir, binName)
		if err := buildTargetBinary(target, binPath); err != nil {
			msg := fmt.Sprintf("Build failed for %s during go build. Re-run with --target %s to isolate the failure.", target, target)
			_, _ = fmt.Fprintln(stderr, msg)
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		artifactName := binBase + ".tar.gz"
		if goos == "windows" {
			artifactName = binBase + ".zip"
		}
		artifactPath := filepath.Join(outputDir, artifactName)
		if err := packageBinary(binPath, target, artifactPath); err != nil {
			_, _ = fmt.Fprintf(stderr, "Build failed for %s during packaging. Re-run with --target %s to isolate the failure.\n", target, target)
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		sum, err := sha256File(artifactPath)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		digests = append(digests, artifactDigest{Name: artifactName, SHA: sum})
	}
	sort.Slice(digests, func(i, j int) bool { return digests[i].Name < digests[j].Name })
	checksumPath := resolveChecksumPath(cfg.Build.ChecksumFile, outputDirOverride)
	if err := os.MkdirAll(filepath.Dir(checksumPath), 0o755); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	var b strings.Builder
	for _, d := range digests {
		b.WriteString(d.SHA)
		b.WriteString("  ")
		b.WriteString(d.Name)
		b.WriteString("\n")
	}
	if err := os.WriteFile(checksumPath, []byte(b.String()), 0o644); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	_, _ = fmt.Fprintf(stdout, "Build complete: %d targets written to %s/ with checksums in %s.\n", len(targets), outputDir, checksumPath)
	return Result{ExitCode: ExitOK}
}

func handleRelease(args []string, stdout, stderr io.Writer) Result {
	draft, notesFileOverride, err := parseReleaseArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	cfg, err := config.Load("")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
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
	artifacts, err := listReleaseArtifacts(cfg.Release.ArtifactDir, cfg.Release.IncludeChecksums)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitReleaseFailure, Err: err}
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
	_, _ = fmt.Fprintf(stdout, "Release %s published from %s with checksums %s.\n", tag, cfg.Release.ArtifactDir, ternary(cfg.Release.IncludeChecksums, "attached", "skipped"))
	return Result{ExitCode: ExitOK}
}
