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
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

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
