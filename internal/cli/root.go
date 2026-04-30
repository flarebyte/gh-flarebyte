package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/buildinfo"
	"github.com/flarebyte/gh-flarebyte/internal/config"
)

// Exit codes aligned with the current design contract.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
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
}

var (
	readRepoMetadata = ghReadRepoMetadata
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
	fmt.Fprintln(w, "  gh flarebyte repo init --repo owner/name [--overwrite]")
	fmt.Fprintln(w, "  gh flarebyte config validate [--config path]")
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

func splitRepo(repo string) (owner string, name string, err error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid invocation: --repo must use owner/name")
	}
	return parts[0], parts[1], nil
}

func ghReadRepoMetadata(repo string) (RepoMetadata, error) {
	cmd := exec.Command(
		"gh", "repo", "view", repo,
		"--json", "description,defaultBranchRef,homepageUrl,isPrivate,isTemplate",
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
	}
	return meta, nil
}

func defaultRepoMetadata(repo string) RepoMetadata {
	return RepoMetadata{
		Description:   "CLI for landing your git commands right",
		DefaultBranch: "main",
		Homepage:      fmt.Sprintf("https://github.com/%s", repo),
		Visibility:    "public",
		Template:      false,
	}
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

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
