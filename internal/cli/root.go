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
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/buildinfo"
	"github.com/flarebyte/gh-flarebyte/internal/config"
)

// Exit codes aligned with the current design contract.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
	ExitDrift   = 3
	ExitBlockedDeletions = 4
	ExitBlockedVisibility = 5
	ExitBuildFailure = 6
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

type AuditDiff struct {
	Field  string `json:"field"`
	Local  any    `json:"local"`
	Remote any    `json:"remote"`
}

type AuditReport struct {
	Repo      string      `json:"repo"`
	DriftCount int        `json:"driftCount"`
	HasDrift  bool        `json:"hasDrift"`
	Diffs     []AuditDiff `json:"diffs"`
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
	readRepoMetadata = ghReadRepoMetadata
	readReposMine    = ghReadReposMine
	applyRepoSettings = ghApplyRepoSettings
	addRepoTopic      = ghAddRepoTopic
	removeRepoTopic   = ghRemoveRepoTopic
	createRepoLabel   = ghCreateRepoLabel
	updateRepoLabel   = ghUpdateRepoLabel
	deleteRepoLabel   = ghDeleteRepoLabel
	buildTargetBinary = goBuildTargetBinary
	packageBinary     = packageBinaryArchive
	fileExists       = func(path string) bool {
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
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if repo == "" {
			err := errors.New("invalid invocation: --repo owner/name is required")
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if fileExists(config.DefaultPath) && !overwrite {
			err := fmt.Errorf("%s already exists in %s. Re-run with --overwrite if replacing it is intentional.", config.DefaultPath, mustAbs("."))
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		owner, name, err := splitRepo(repo)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}

		meta := defaultRepoMetadata(repo)
		imported := false
		if m, err := readRepoMetadata(repo); err == nil {
			meta = m
			imported = true
		} else {
			fmt.Fprintf(stderr, "warning: unable to import remote metadata for %s, using defaults (%v)\n", repo, err)
		}

		content := renderCueConfig(owner, name, meta)
		if err := writeFile(config.DefaultPath, []byte(content)); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitFailure, Err: err}
		}
		absPath := mustAbs(config.DefaultPath)
		if imported {
			fmt.Fprintf(stdout, "created %s from current GitHub state. Next: review the config, then run gh flarebyte repo update.\n", absPath)
		} else {
			fmt.Fprintf(stdout, "created %s from defaults. Next: review the config, then run gh flarebyte repo update.\n", absPath)
		}
		return Result{ExitCode: ExitOK}
	}

	if len(args) >= 2 && args[0] == "repo" && args[1] == "audit" {
		repo, asJSON, err := parseRepoAuditArgs(args[2:])
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		cfg, err := config.Load("")
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if repo == "" {
			repo = fmt.Sprintf("%s/%s", cfg.Project.Org, cfg.Project.Repo)
		}
		remote, err := readRepoMetadata(repo)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
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
			fmt.Fprintf(stdout, "%d differences found. Review the drift report below, then run gh flarebyte repo update when ready to apply changes.\n", report.DriftCount)
			for _, diff := range report.Diffs {
				fmt.Fprintf(stdout, "- %s local=%v remote=%v\n", diff.Field, diff.Local, diff.Remote)
			}
		} else {
			fmt.Fprintln(stdout, "No drift found. GitHub matches .gh-flarebyte.cue.")
		}
		if report.HasDrift {
			return Result{ExitCode: ExitDrift}
		}
		return Result{ExitCode: ExitOK}
	}

	if len(args) >= 2 && args[0] == "repo" && args[1] == "update" {
		repo, confirmDeletions, acceptVisibility, err := parseRepoUpdateArgs(args[2:])
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		cfg, err := config.Load("")
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if repo == "" {
			repo = fmt.Sprintf("%s/%s", cfg.Project.Org, cfg.Project.Repo)
		}
		remote, err := readRepoMetadata(repo)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitFailure, Err: err}
		}
		plan := buildUpdatePlan(cfg, remote)
		if plan.VisibilityChange && !acceptVisibility {
			err := fmt.Errorf("Visibility would change from %s to %s. Re-run with --accept-visibility-change-consequences if that is intentional.", remote.Visibility, cfg.Repository.Visibility)
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitBlockedVisibility, Err: err}
		}
		if (len(plan.TopicsToRemove) > 0 || len(plan.LabelsToDelete) > 0) && !confirmDeletions {
			err := fmt.Errorf("Update would delete %d labels and %d topics. Re-run with --confirm-deletions if that is intentional.", len(plan.LabelsToDelete), len(plan.TopicsToRemove))
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitBlockedDeletions, Err: err}
		}

		settingsUpdated := 0
		topicsSynced := 0
		labelsReconciled := 0
		appliedSettings := false
		appliedTopics := false

		if plan.SettingsChanged {
			if err := applyRepoSettings(repo, cfg.Repository); err != nil {
				fmt.Fprintln(stderr, err.Error())
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

		fmt.Fprintf(stdout, "Update complete: %d repo settings updated, %d topics synced, %d labels reconciled.\n", settingsUpdated, topicsSynced, labelsReconciled)
		return Result{ExitCode: ExitOK}
	}

	if len(args) >= 2 && args[0] == "repos" && args[1] == "mine" {
		org, asJSON, err := parseReposMineArgs(args[2:])
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if org == "" {
			err := errors.New("invalid invocation: --org is required")
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		contributor, repos, err := readReposMine(org)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
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
		fmt.Fprintf(stdout, "Found %d repositories in org %s for contributor %s.\n", report.Count, report.Org, report.Contributor)
		for _, repo := range report.Repos {
			fmt.Fprintf(stdout, "- %s/%s (%s, default branch: %s)\n", repo.Owner, repo.Name, repo.Visibility, repo.DefaultBranch)
		}
		return Result{ExitCode: ExitOK}
	}

	if len(args) >= 2 && args[0] == "config" && args[1] == "validate" {
		configPath, err := parseConfigPath(args[2:])
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		absPath, err := config.ResolvePath(configPath)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if _, err := config.Load(configPath); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		fmt.Fprintf(stdout, "config is valid: %s\n", absPath)
		return Result{ExitCode: ExitOK}
	}

	if len(args) >= 1 && args[0] == "build" {
		targetFilter, outputDirOverride, err := parseBuildArgs(args[1:])
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		cfg, err := config.Load("")
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if cfg.Build.Language != "go" {
			err := fmt.Errorf("build.language %q is not supported yet. Supported values: go", cfg.Build.Language)
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		targets := cfg.Build.Targets
		if targetFilter != "" {
			if !contains(targets, targetFilter) {
				err := fmt.Errorf("invalid --target %q: not present in build.targets", targetFilter)
				fmt.Fprintln(stderr, err.Error())
				return Result{ExitCode: ExitUsage, Err: err}
			}
			targets = []string{targetFilter}
		}
		outputDir := cfg.Build.OutputDir
		if outputDirOverride != "" {
			outputDir = outputDirOverride
		}
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitFailure, Err: err}
		}
		tmpDir := filepath.Join(outputDir, ".tmp")
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			fmt.Fprintln(stderr, err.Error())
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
				fmt.Fprintln(stderr, err.Error())
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
				fmt.Fprintln(stderr, msg)
				return Result{ExitCode: ExitBuildFailure, Err: err}
			}
			artifactName := binBase + ".tar.gz"
			if goos == "windows" {
				artifactName = binBase + ".zip"
			}
			artifactPath := filepath.Join(outputDir, artifactName)
			if err := packageBinary(binPath, target, artifactPath); err != nil {
				fmt.Fprintf(stderr, "Build failed for %s during packaging. Re-run with --target %s to isolate the failure.\n", target, target)
				return Result{ExitCode: ExitBuildFailure, Err: err}
			}
			sum, err := sha256File(artifactPath)
			if err != nil {
				fmt.Fprintln(stderr, err.Error())
				return Result{ExitCode: ExitBuildFailure, Err: err}
			}
			digests = append(digests, artifactDigest{Name: artifactName, SHA: sum})
		}
		sort.Slice(digests, func(i, j int) bool { return digests[i].Name < digests[j].Name })
		checksumPath := resolveChecksumPath(cfg.Build.ChecksumFile, outputDirOverride)
		if err := os.MkdirAll(filepath.Dir(checksumPath), 0o755); err != nil {
			fmt.Fprintln(stderr, err.Error())
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
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitFailure, Err: err}
		}
		fmt.Fprintf(stdout, "Build complete: %d targets written to %s/ with checksums in %s.\n", len(targets), outputDir, checksumPath)
		return Result{ExitCode: ExitOK}
	}

	hasVersion := contains(args, "--version")
	hasJSON := contains(args, "--json")

	if hasJSON && !hasVersion {
		fmt.Fprintln(stderr, "invalid invocation: --json must be used with --version")
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
		fmt.Fprintf(
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

	fmt.Fprintf(stderr, "unknown arguments: %v\n", args)
	fmt.Fprintln(stderr, "run `gh flarebyte --help` for usage")
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
	fmt.Fprintln(w, "gh flarebyte - manage GitHub repository state from .gh-flarebyte.cue")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gh flarebyte --help")
	fmt.Fprintln(w, "  gh flarebyte --version [--json]")
	fmt.Fprintln(w, "  gh flarebyte build [--target os-arch] [--output-dir path]")
	fmt.Fprintln(w, "  gh flarebyte repo init --repo owner/name [--overwrite]")
	fmt.Fprintln(w, "  gh flarebyte repo update [--repo owner/name] [--confirm-deletions] [--accept-visibility-change-consequences]")
	fmt.Fprintln(w, "  gh flarebyte repo audit [--repo owner/name] [--json]")
	fmt.Fprintln(w, "  gh flarebyte repos mine --org my-org [--json]")
	fmt.Fprintln(w, "  gh flarebyte config validate [--config path]")
}

type UpdatePlan struct {
	SettingsChanged    bool
	SettingsChangeCount int
	VisibilityChange   bool
	TopicsToAdd        []string
	TopicsToRemove     []string
	LabelsToCreate     []LabelState
	LabelsToUpdate     []LabelState
	LabelsToDelete     []LabelState
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
		Labels struct {
			Nodes []struct {
				Name        string `json:"name"`
				Color       string `json:"color"`
				Description string `json:"description"`
			} `json:"nodes"`
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
		Labels:        extractLabelsFromState(payload.Labels.Nodes),
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
	}
	if cfg.Repository.Template != remote.Template {
		settingsChangeCount++
	}
	plan.SettingsChangeCount = settingsChangeCount
	plan.SettingsChanged = settingsChangeCount > 0

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
	fmt.Fprintln(stderr, msg)
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
				Login                      string `json:"login"`
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
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	out, err := os.Create(artifactPath)
	if err != nil {
		return err
	}
	defer out.Close()
	gw := gzip.NewWriter(out)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
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
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
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

func renderCueConfig(owner, name string, meta RepoMetadata) string {
	return fmt.Sprintf(`package ghflarebyte

project: {
	org:  %q
	repo: %q
}

sync: {
	mode: "push"
	managedFields: [
		"description",
		"defaultBranch",
		"homepage",
		"visibility",
		"template",
		"topics",
		"labels",
		"features.issues",
		"features.wiki",
		"features.projects",
		"features.discussions",
		"features.autoMerge",
		"features.mergeCommit",
		"features.rebaseMerge",
		"features.squashMerge",
		"features.squashMergeCommitMessage",
		"features.deleteBranchOnMerge",
		"features.allowForking",
		"features.allowUpdateBranch",
		"features.advancedSecurity",
		"features.secretScanning",
		"features.secretScanningPushProtection",
	]
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

func ghApplyRepoSettings(repo string, desired config.RepositoryConfig) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return nil
	}
	args := []string{
		"repo", "edit", repo,
		"--description", desired.Description,
		"--default-branch", desired.DefaultBranch,
		"--homepage", desired.Homepage,
		"--visibility", desired.Visibility,
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
