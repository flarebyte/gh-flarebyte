package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/buildinfo"
	"github.com/flarebyte/gh-flarebyte/internal/config"
)

// Exit codes aligned with the current design contract.
const (
	ExitOK                = 0
	ExitFailure           = 1
	ExitUsage             = 2
	ExitDrift             = 3
	ExitBlockedDeletions  = 4
	ExitBlockedVisibility = 5
	ExitBuildFailure      = 6
	ExitReleaseFailure    = 7
)

type VersionInfo struct {
	Version   string `json:"version"`
	CommitID  string `json:"commitId"`
	Date      string `json:"date"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"goVersion"`
}

type Result struct {
	ExitCode int
	Err      error
}

type RepoMetadata struct {
	Description   string
	DefaultBranch string
	Homepage      string
	Visibility    string
	Template      bool
	Topics        []string
	Labels        []LabelState
}

type LabelState struct {
	Name        string
	Color       string
	Description string
}

type RepoSettingsPatch struct {
	Description   string
	DefaultBranch string
	Homepage      string
	Template      bool
	Visibility    string
	SetVisibility bool
}

type AuditDiff struct {
	Field  string `json:"field"`
	Local  any    `json:"local"`
	Remote any    `json:"remote"`
}

type AuditReport struct {
	Repo       string      `json:"repo"`
	DriftCount int         `json:"driftCount"`
	HasDrift   bool        `json:"hasDrift"`
	Diffs      []AuditDiff `json:"diffs"`
}

type ContributedRepo struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Visibility    string `json:"visibility"`
	DefaultBranch string `json:"defaultBranch"`
}

type ReposMineReport struct {
	Org         string            `json:"org"`
	Contributor string            `json:"contributor"`
	Count       int               `json:"count"`
	Repos       []ContributedRepo `json:"repos"`
}

var (
	readRepoMetadata  = ghReadRepoMetadata
	readReposMine     = ghReadReposMine
	applyRepoSettings = ghApplyRepoSettings
	addRepoTopic      = ghAddRepoTopic
	removeRepoTopic   = ghRemoveRepoTopic
	createRepoLabel   = ghCreateRepoLabel
	updateRepoLabel   = ghUpdateRepoLabel
	deleteRepoLabel   = ghDeleteRepoLabel
	buildTargetBinary = goBuildTargetBinary
	packageBinary     = packageBinaryArchive
	findVersion       = resolveVersionFromSource
	tagExists         = ghTagExists
	createRelease     = ghCreateRelease
	fileExists        = func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}
	writeFile = func(path string, data []byte) error {
		return os.WriteFile(path, data, 0o644)
	}
)

func Run(args []string, stdout, stderr io.Writer) Result {
	if len(args) == 0 || contains(args, "--help") {
		printHelp(stdout)
		return Result{ExitCode: ExitOK}
	}

	if len(args) >= 2 && args[0] == "repo" && args[1] == "init" {
		repo, overwrite, err := parseRepoInitArgs(args[2:])
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if repo == "" {
			err := errors.New("invalid invocation: --repo owner/name is required")
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

	if len(args) >= 2 && args[0] == "repo" && args[1] == "audit" {
		repo, asJSON, err := parseRepoAuditArgs(args[2:])
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

	if len(args) >= 2 && args[0] == "repo" && args[1] == "update" {
		repo, confirmDeletions, acceptVisibility, err := parseRepoUpdateArgs(args[2:])
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

	if len(args) >= 2 && args[0] == "repos" && args[1] == "mine" {
		org, asJSON, err := parseReposMineArgs(args[2:])
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if org == "" {
			err := errors.New("invalid invocation: --org is required")
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

	if len(args) >= 2 && args[0] == "config" && args[1] == "validate" {
		configPath, err := parseConfigPath(args[2:])
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

	if len(args) >= 1 && args[0] == "build" {
		targetFilter, outputDirOverride, err := parseBuildArgs(args[1:])
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

	if len(args) >= 1 && args[0] == "release" {
		draft, notesFileOverride, err := parseReleaseArgs(args[1:])
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
				err := errors.New("release notes mode is notes-file but no notes file was provided. Set release.releaseNotesFilePath or pass --notes-file")
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

	hasVersion := contains(args, "--version")
	hasJSON := contains(args, "--json")

	if hasJSON && !hasVersion {
		_, _ = fmt.Fprintln(stderr, "invalid invocation: --json must be used with --version")
		return Result{ExitCode: ExitUsage, Err: errors.New("invalid invocation")}
	}

	if hasVersion {
		v := currentVersionInfo()
		if hasJSON {
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(v); err != nil {
				return Result{ExitCode: ExitFailure, Err: err}
			}
			return Result{ExitCode: ExitOK}
		}
		_, _ = fmt.Fprintf(
			stdout,
			"gh-flarebyte %s commitId=%s date=%s os=%s arch=%s goVersion=%s\n",
			v.Version,
			v.CommitID,
			v.Date,
			v.OS,
			v.Arch,
			v.GoVersion,
		)
		return Result{ExitCode: ExitOK}
	}

	_, _ = fmt.Fprintf(stderr, "unknown arguments: %v\n", args)
	_, _ = fmt.Fprintln(stderr, "run `gh flarebyte --help` for usage")
	return Result{ExitCode: ExitUsage, Err: errors.New("invalid invocation")}
}

func currentVersionInfo() VersionInfo {
	gv := buildinfo.GoVersion
	if gv == "unknown" || gv == "" {
		gv = runtime.Version()
	}
	return VersionInfo{
		Version:   buildinfo.Version,
		CommitID:  buildinfo.CommitID,
		Date:      buildinfo.Date,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: gv,
	}
}

func printHelp(w io.Writer) {
	_, _ = io.WriteString(w, `gh flarebyte - manage GitHub repository state from .gh-flarebyte.cue

Usage:
  gh flarebyte --help
  gh flarebyte --version [--json]
  gh flarebyte build [--target os-arch] [--output-dir path]
  gh flarebyte release [--draft] [--notes-file path]
  gh flarebyte repo init --repo owner/name [--overwrite]
  gh flarebyte repo update [--repo owner/name] [--confirm-deletions] [--accept-visibility-change-consequences]
  gh flarebyte repo audit [--repo owner/name] [--json]
  gh flarebyte repos mine --org my-org [--json]
  gh flarebyte config validate [--config path]
`)
}

type UpdatePlan struct {
	SettingsChanged     bool
	SettingsChangeCount int
	VisibilityChange    bool
	SettingsPatch       RepoSettingsPatch
	TopicsToAdd         []string
	TopicsToRemove      []string
	LabelsToCreate      []LabelState
	LabelsToUpdate      []LabelState
	LabelsToDelete      []LabelState
}

func contains(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
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

func parseRepoInitArgs(args []string) (repo string, overwrite bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--overwrite":
			overwrite = true
		case "--repo":
			if i+1 >= len(args) {
				return "", false, errors.New("invalid invocation: --repo requires owner/name")
			}
			repo = args[i+1]
			i++
		default:
			return "", false, fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return repo, overwrite, nil
}

func parseRepoAuditArgs(args []string) (repo string, asJSON bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--repo":
			if i+1 >= len(args) {
				return "", false, errors.New("invalid invocation: --repo requires owner/name")
			}
			repo = args[i+1]
			i++
		case "--json":
			asJSON = true
		default:
			return "", false, fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return repo, asJSON, nil
}

func parseRepoUpdateArgs(args []string) (repo string, confirmDeletions bool, acceptVisibility bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--repo":
			if i+1 >= len(args) {
				return "", false, false, errors.New("invalid invocation: --repo requires owner/name")
			}
			repo = args[i+1]
			i++
		case "--confirm-deletions":
			confirmDeletions = true
		case "--accept-visibility-change-consequences":
			acceptVisibility = true
		default:
			return "", false, false, fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return repo, confirmDeletions, acceptVisibility, nil
}

func parseReposMineArgs(args []string) (org string, asJSON bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--org":
			if i+1 >= len(args) {
				return "", false, errors.New("invalid invocation: --org requires a value")
			}
			org = args[i+1]
			i++
		case "--json":
			asJSON = true
		default:
			return "", false, fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return org, asJSON, nil
}

func parseBuildArgs(args []string) (target string, outputDir string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--target":
			if i+1 >= len(args) {
				return "", "", errors.New("invalid invocation: --target requires os-arch")
			}
			target = args[i+1]
			i++
		case "--output-dir":
			if i+1 >= len(args) {
				return "", "", errors.New("invalid invocation: --output-dir requires a path")
			}
			outputDir = args[i+1]
			i++
		default:
			return "", "", fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return target, outputDir, nil
}

func parseReleaseArgs(args []string) (draft bool, notesFile string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--draft":
			draft = true
		case "--notes-file":
			if i+1 >= len(args) {
				return false, "", errors.New("invalid invocation: --notes-file requires a path")
			}
			notesFile = args[i+1]
			i++
		default:
			return false, "", fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return draft, notesFile, nil
}

func splitRepo(repo string) (owner string, name string, err error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid invocation: --repo must use owner/name")
	}
	return parts[0], parts[1], nil
}

func ghReadRepoMetadata(repo string) (RepoMetadata, error) {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return defaultRepoMetadata(repo), nil
	}
	cmd := exec.Command(
		"gh", "repo", "view", repo,
		"--json", "description,defaultBranchRef,homepageUrl,isPrivate,isTemplate,repositoryTopics,labels",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return RepoMetadata{}, errors.New(msg)
	}
	var payload struct {
		Description      string `json:"description"`
		HomepageURL      string `json:"homepageUrl"`
		IsPrivate        bool   `json:"isPrivate"`
		IsTemplate       bool   `json:"isTemplate"`
		DefaultBranchRef struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
		RepositoryTopics struct {
			Nodes []struct {
				Topic struct {
					Name string `json:"name"`
				} `json:"topic"`
			} `json:"nodes"`
		} `json:"repositoryTopics"`
		Labels []struct {
			Name        string `json:"name"`
			Color       string `json:"color"`
			Description string `json:"description"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return RepoMetadata{}, err
	}
	visibility := "public"
	if payload.IsPrivate {
		visibility = "private"
	}
	meta := RepoMetadata{
		Description:   payload.Description,
		DefaultBranch: payload.DefaultBranchRef.Name,
		Homepage:      payload.HomepageURL,
		Visibility:    visibility,
		Template:      payload.IsTemplate,
		Topics:        extractTopics(payload.RepositoryTopics.Nodes),
		Labels:        extractLabelsFromState(payload.Labels),
	}
	return meta, nil
}

func extractTopics(nodes []struct {
	Topic struct {
		Name string `json:"name"`
	} `json:"topic"`
}) []string {
	topics := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node.Topic.Name != "" {
			topics = append(topics, node.Topic.Name)
		}
	}
	return topics
}

func extractLabelsFromState(nodes []struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}) []LabelState {
	labels := make([]LabelState, 0, len(nodes))
	for _, node := range nodes {
		labels = append(labels, LabelState{
			Name:        node.Name,
			Color:       node.Color,
			Description: node.Description,
		})
	}
	return labels
}

func defaultRepoMetadata(repo string) RepoMetadata {
	return RepoMetadata{
		Description:   "CLI for landing your git commands right",
		DefaultBranch: "main",
		Homepage:      fmt.Sprintf("https://github.com/%s", repo),
		Visibility:    "public",
		Template:      false,
		Topics:        []string{"gh-extension", "github-cli", "git", "flarebyte"},
		Labels: []LabelState{
			{Name: "bug", Color: "B60205", Description: "Something is broken"},
			{Name: "enhancement", Color: "0E8A16", Description: "New feature"},
		},
	}
}

func buildUpdatePlan(cfg config.Config, remote RepoMetadata) UpdatePlan {
	plan := UpdatePlan{
		TopicsToAdd:    diffStrings(cfg.Repository.Topics, remote.Topics),
		TopicsToRemove: diffStrings(remote.Topics, cfg.Repository.Topics),
	}
	settingsChangeCount := 0
	patch := RepoSettingsPatch{
		Description:   cfg.Repository.Description,
		DefaultBranch: cfg.Repository.DefaultBranch,
		Homepage:      cfg.Repository.Homepage,
		Template:      cfg.Repository.Template,
	}
	if cfg.Repository.Description != remote.Description {
		settingsChangeCount++
	}
	if cfg.Repository.DefaultBranch != remote.DefaultBranch {
		settingsChangeCount++
	}
	if cfg.Repository.Homepage != remote.Homepage {
		settingsChangeCount++
	}
	if cfg.Repository.Visibility != remote.Visibility {
		settingsChangeCount++
		plan.VisibilityChange = true
		patch.Visibility = cfg.Repository.Visibility
		patch.SetVisibility = true
	}
	if cfg.Repository.Template != remote.Template {
		settingsChangeCount++
	}
	plan.SettingsChangeCount = settingsChangeCount
	plan.SettingsChanged = settingsChangeCount > 0
	plan.SettingsPatch = patch

	remoteByName := make(map[string]LabelState, len(remote.Labels))
	for _, r := range remote.Labels {
		remoteByName[r.Name] = r
	}
	localNames := make(map[string]struct{}, len(cfg.Repository.Labels))
	for _, l := range cfg.Repository.Labels {
		localNames[l.Name] = struct{}{}
		desired := LabelState{Name: l.Name, Color: l.Color, Description: l.Description}
		r, ok := remoteByName[l.Name]
		if !ok {
			plan.LabelsToCreate = append(plan.LabelsToCreate, desired)
			continue
		}
		if r.Color != desired.Color || r.Description != desired.Description {
			plan.LabelsToUpdate = append(plan.LabelsToUpdate, desired)
		}
	}
	for _, r := range remote.Labels {
		if _, ok := localNames[r.Name]; !ok {
			plan.LabelsToDelete = append(plan.LabelsToDelete, r)
		}
	}
	return plan
}

func diffStrings(source []string, minus []string) []string {
	minusSet := make(map[string]struct{}, len(minus))
	for _, m := range minus {
		minusSet[m] = struct{}{}
	}
	out := make([]string, 0)
	for _, s := range source {
		if _, ok := minusSet[s]; !ok {
			out = append(out, s)
		}
	}
	return out
}

func partialUpdateFailure(stderr io.Writer, cause error, settingsApplied bool, topicsApplied bool) Result {
	parts := make([]string, 0)
	if settingsApplied {
		parts = append(parts, "repository settings")
	}
	if topicsApplied {
		parts = append(parts, "topics")
	}
	prefix := "Update failed before applying changes."
	if len(parts) > 0 {
		prefix = fmt.Sprintf("Update stopped after %s were applied.", strings.Join(parts, " and "))
	}
	msg := fmt.Sprintf("%s Label sync failed, and no rollback was attempted. Fix the error and rerun gh flarebyte repo update. Cause: %v", prefix, cause)
	_, _ = fmt.Fprintln(stderr, msg)
	return Result{ExitCode: ExitFailure, Err: cause}
}

func buildAuditReport(repo string, cfg config.Config, remote RepoMetadata) AuditReport {
	diffs := make([]AuditDiff, 0)
	if cfg.Repository.Homepage != remote.Homepage {
		diffs = append(diffs, AuditDiff{Field: "repository.homepage", Local: cfg.Repository.Homepage, Remote: remote.Homepage})
	}
	if cfg.Repository.Description != remote.Description {
		diffs = append(diffs, AuditDiff{Field: "repository.description", Local: cfg.Repository.Description, Remote: remote.Description})
	}
	if cfg.Repository.DefaultBranch != remote.DefaultBranch {
		diffs = append(diffs, AuditDiff{Field: "repository.defaultBranch", Local: cfg.Repository.DefaultBranch, Remote: remote.DefaultBranch})
	}
	if cfg.Repository.Visibility != remote.Visibility {
		diffs = append(diffs, AuditDiff{Field: "repository.visibility", Local: cfg.Repository.Visibility, Remote: remote.Visibility})
	}
	if cfg.Repository.Template != remote.Template {
		diffs = append(diffs, AuditDiff{Field: "repository.template", Local: cfg.Repository.Template, Remote: remote.Template})
	}
	if !stringSlicesEqual(cfg.Repository.Topics, remote.Topics) {
		diffs = append(diffs, AuditDiff{Field: "repository.topics", Local: cfg.Repository.Topics, Remote: remote.Topics})
	}
	return AuditReport{
		Repo:       repo,
		DriftCount: len(diffs),
		HasDrift:   len(diffs) > 0,
		Diffs:      diffs,
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func ghReadReposMine(org string) (string, []ContributedRepo, error) {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return "fake-user", []ContributedRepo{
			{Owner: org, Name: "gh-flarebyte", Visibility: "public", DefaultBranch: "main"},
			{Owner: org, Name: "baldrick-seer", Visibility: "public", DefaultBranch: "main"},
		}, nil
	}
	query := `query {
  viewer {
    login
    repositoriesContributedTo(first: 100, includeUserRepositories: true, contributionTypes: [COMMIT, ISSUE, PULL_REQUEST, REPOSITORY]) {
      nodes {
        name
        visibility
        defaultBranchRef { name }
        owner { login }
      }
    }
  }
}`
	cmd := exec.Command("gh", "api", "graphql", "-f", "query="+query)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", nil, errors.New(msg)
	}
	var payload struct {
		Data struct {
			Viewer struct {
				Login                     string `json:"login"`
				RepositoriesContributedTo struct {
					Nodes []struct {
						Name             string `json:"name"`
						Visibility       string `json:"visibility"`
						DefaultBranchRef struct {
							Name string `json:"name"`
						} `json:"defaultBranchRef"`
						Owner struct {
							Login string `json:"login"`
						} `json:"owner"`
					} `json:"nodes"`
				} `json:"repositoriesContributedTo"`
			} `json:"viewer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		return "", nil, err
	}
	repos := make([]ContributedRepo, 0)
	for _, node := range payload.Data.Viewer.RepositoriesContributedTo.Nodes {
		if !strings.EqualFold(node.Owner.Login, org) {
			continue
		}
		repos = append(repos, ContributedRepo{
			Owner:         node.Owner.Login,
			Name:          node.Name,
			Visibility:    strings.ToLower(node.Visibility),
			DefaultBranch: node.DefaultBranchRef.Name,
		})
	}
	return payload.Data.Viewer.Login, repos, nil
}

func goBuildTargetBinary(target string, outputPath string) error {
	goos, goarch, err := splitTarget(target)
	if err != nil {
		return err
	}
	args := []string{
		"build",
		"-ldflags",
		fmt.Sprintf("-X github.com/flarebyte/gh-flarebyte/internal/buildinfo.Version=%s -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.CommitID=%s -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.Date=%s -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.GoVersion=%s",
			buildinfo.Version, buildinfo.CommitID, buildinfo.Date, buildinfo.GoVersion),
		"-o",
		outputPath,
		"./cmd/gh-flarebyte",
	}
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func splitTarget(target string) (goos, goarch string, err error) {
	parts := strings.Split(target, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid target %q: expected os-arch", target)
	}
	return parts[0], parts[1], nil
}

func packageBinaryArchive(binaryPath string, target string, artifactPath string) error {
	goos, _, err := splitTarget(target)
	if err != nil {
		return err
	}
	if goos == "windows" {
		return packageZip(binaryPath, artifactPath)
	}
	return packageTarGz(binaryPath, artifactPath)
}

func packageTarGz(binaryPath string, artifactPath string) error {
	src, err := os.Open(binaryPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	out, err := os.Create(artifactPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	gw := gzip.NewWriter(out)
	defer func() { _ = gw.Close() }()
	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()
	hdr := &tar.Header{
		Name:    filepath.Base(binaryPath),
		Mode:    0o755,
		Size:    info.Size(),
		ModTime: zeroTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.Copy(tw, src); err != nil {
		return err
	}
	return nil
}

func packageZip(binaryPath string, artifactPath string) error {
	content, err := os.ReadFile(binaryPath)
	if err != nil {
		return err
	}
	out, err := os.Create(artifactPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	zw := zip.NewWriter(out)
	defer func() { _ = zw.Close() }()
	hdr := &zip.FileHeader{
		Name:   path.Base(binaryPath),
		Method: zip.Deflate,
	}
	hdr.SetMode(os.FileMode(0o755))
	hdr.Modified = zeroTime()
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	if _, err := w.Write(content); err != nil {
		return err
	}
	return nil
}

func zeroTime() time.Time {
	return time.Unix(0, 0).UTC()
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

func resolveChecksumPath(configPath string, outputOverride string) string {
	if outputOverride == "" {
		return configPath
	}
	return filepath.Join(outputOverride, filepath.Base(configPath))
}

func resolveVersionFromSource(sourcePath string) (string, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("cannot read release.versionSource %s: %w", sourcePath, err)
	}
	ext := strings.ToLower(filepath.Ext(sourcePath))
	switch ext {
	case ".json":
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			return "", fmt.Errorf("cannot parse release.versionSource JSON %s: %w", sourcePath, err)
		}
		v, ok := payload["version"].(string)
		if !ok || strings.TrimSpace(v) == "" {
			return "", errors.New("invalid release.versionSource: top-level string field version is required")
		}
		if !isSemver(v) {
			return "", fmt.Errorf("invalid version %q: semantic version required", v)
		}
		return v, nil
	case ".yaml", ".yml":
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "version:") {
				v := strings.TrimSpace(strings.TrimPrefix(trim, "version:"))
				v = strings.Trim(v, `"'`)
				if v == "" {
					return "", errors.New("invalid release.versionSource: top-level string field version is required")
				}
				if !isSemver(v) {
					return "", fmt.Errorf("invalid version %q: semantic version required", v)
				}
				return v, nil
			}
		}
		return "", errors.New("invalid release.versionSource: top-level string field version is required")
	default:
		return "", fmt.Errorf("unsupported release.versionSource format %q: expected YAML or JSON file", ext)
	}
}

func isSemver(v string) bool {
	// Accepts forms like 1.2.3, 1.2.3-rc.1, 1.2.3+build.7
	re := `^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`
	return regexp.MustCompile(re).MatchString(v)
}

func listReleaseArtifacts(artifactDir string, includeChecksums bool) ([]string, error) {
	entries, err := os.ReadDir(artifactDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read release.artifactDir %s: %w", artifactDir, err)
	}
	artifacts := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip") {
			artifacts = append(artifacts, filepath.Join(artifactDir, name))
		}
		if includeChecksums && (name == "checksums.txt" || strings.HasSuffix(name, "checksums.txt")) {
			artifacts = append(artifacts, filepath.Join(artifactDir, name))
		}
	}
	sort.Strings(artifacts)
	if len(artifacts) == 0 {
		return nil, fmt.Errorf("no release artifacts found in %s", artifactDir)
	}
	return artifacts, nil
}

func ghTagExists(tag string) (bool, error) {
	if os.Getenv("GH_FLAREBYTE_FAKE_RELEASE") == "1" {
		return false, nil
	}
	cmd := exec.Command("gh", "release", "view", tag)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	msg := strings.TrimSpace(stderr.String())
	// treat "not found" as clean false
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "not found") || strings.Contains(lower, "http 404") {
		return false, nil
	}
	if msg == "" {
		msg = err.Error()
	}
	return false, errors.New(msg)
}

func ghCreateRelease(tag string, artifacts []string, notesMode string, notesFile string, draft bool) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_RELEASE") == "1" {
		return nil
	}
	args := []string{"release", "create", tag}
	args = append(args, artifacts...)
	switch notesMode {
	case "generate-notes", "notes-from-tag":
		args = append(args, "--generate-notes")
	case "notes-file":
		args = append(args, "--notes-file", notesFile)
	}
	if draft {
		args = append(args, "--draft")
	}
	return runGH(args...)
}

func ternary(cond bool, yes string, no string) string {
	if cond {
		return yes
	}
	return no
}

func renderCueConfig(owner, name string, meta RepoMetadata) string {
	return fmt.Sprintf(`package ghflarebyte

project: {
	org:  %q
	repo: %q
}

sync: {
	mode: "push"
}

repository: {
	description:   %q
	defaultBranch: %q
	homepage:      %q
	visibility:    %q
	template:      %v
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
	features: {
		issues:                        true
		wiki:                          false
		projects:                      false
		discussions:                   false
		autoMerge:                     true
		mergeCommit:                   false
		rebaseMerge:                   false
		squashMerge:                   true
		squashMergeCommitMessage:      "pr-title"
		deleteBranchOnMerge:           true
		allowForking:                  false
		allowUpdateBranch:             false
		advancedSecurity:              true
		secretScanning:                true
		secretScanningPushProtection:  true
	}
}

build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
		"darwin-arm64",
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
`, owner, name, meta.Description, meta.DefaultBranch, meta.Homepage, meta.Visibility, meta.Template)
}

func ghApplyRepoSettings(repo string, desired RepoSettingsPatch) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return nil
	}
	args := []string{
		"repo", "edit", repo,
		"--description", desired.Description,
		"--default-branch", desired.DefaultBranch,
		"--homepage", desired.Homepage,
	}
	if desired.SetVisibility {
		args = append(args, "--visibility", desired.Visibility)
	}
	if desired.Template {
		args = append(args, "--template")
	} else {
		args = append(args, "--template=false")
	}
	cmd := exec.Command("gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func ghAddRepoTopic(repo string, topic string) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return nil
	}
	return runGH("repo", "edit", repo, "--add-topic", topic)
}

func ghRemoveRepoTopic(repo string, topic string) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return nil
	}
	return runGH("repo", "edit", repo, "--remove-topic", topic)
}

func ghCreateRepoLabel(repo string, label LabelState) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return nil
	}
	return runGH("label", "create", label.Name, "--repo", repo, "--color", label.Color, "--description", label.Description, "--force")
}

func ghUpdateRepoLabel(repo string, label LabelState) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return nil
	}
	return runGH("label", "edit", label.Name, "--repo", repo, "--color", label.Color, "--description", label.Description)
}

func ghDeleteRepoLabel(repo string, labelName string) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return nil
	}
	return runGH("label", "delete", labelName, "--repo", repo, "--yes")
}

func runGH(args ...string) error {
	cmd := exec.Command("gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
