// purpose: Define the CLI domain model and top-level command router for gh-flarebyte.
// responsibilities: Declare shared CLI types/exit codes; dispatch subcommands; render help/version output.
// architecture notes: Command handlers are wired through package-level function variables to keep command orchestration testable with lightweight stubs.
package cli

import (
	"io"
	"os"
	"runtime"

	"github.com/flarebyte/gh-flarebyte/internal/buildinfo"
)

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
	Description         string
	DefaultBranch       string
	Homepage            string
	Visibility          string
	Template            bool
	MergeCommit         bool
	RebaseMerge         bool
	SquashMerge         bool
	DeleteBranchOnMerge bool
	Topics              []string
	Labels              []LabelState
}

type LabelState struct {
	Name        string
	Color       string
	Description string
}

type RepoSettingsPatch struct {
	Description            string
	DefaultBranch          string
	Homepage               string
	Template               bool
	Visibility             string
	SetVisibility          bool
	MergeCommit            bool
	SetMergeCommit         bool
	RebaseMerge            bool
	SetRebaseMerge         bool
	SquashMerge            bool
	SetSquashMerge         bool
	DeleteBranchOnMerge    bool
	SetDeleteBranchOnMerge bool
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
	return runWithCobra(args, stdout, stderr)
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
  gh flarebyte test
  gh flarebyte format
  gh flarebyte lint
  gh flarebyte cov [--min percent]
  gh flarebyte release [--draft] [--notes-file path]
  gh flarebyte repo init --repo owner/name [--overwrite]
  gh flarebyte repo update [--repo owner/name] [--confirm-deletions] [--accept-visibility-change-consequences]
  gh flarebyte repo audit [--repo owner/name] [--json]
  gh flarebyte repos mine --org my-org [--json]
  gh flarebyte config validate [--config path]
`)
}

func contains(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}
