package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

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
