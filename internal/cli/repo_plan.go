package cli

import (
	"fmt"
	"io"
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
	artifactTargetSuffix: true
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
