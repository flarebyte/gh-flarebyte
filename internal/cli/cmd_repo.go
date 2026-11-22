// purpose: Implement user-facing repository workflows (init, audit, update, mine) on top of config and GitHub adapters.
// responsibilities: Parse repo command intent; enforce safety gates; orchestrate plan execution; format repo workflow output.
// architecture notes: Update applies settings/topics/labels sequentially and intentionally avoids rollback, surfacing partial-progress failures for deterministic reruns.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func runRepoInit(repo string, overwrite bool, stdout, stderr io.Writer) Result {
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

func runRepoAudit(repo string, asJSON bool, stdout, stderr io.Writer) Result {
	repo, cfg, remote, fail := loadRepoContext(repo, stderr)
	if fail != nil {
		return *fail
	}
	report := buildAuditReport(repo, cfg, remote)
	changes, rulesets, err := loadBranchProtectionPlan(repo, cfg.Repository.BranchProtection)
	if err != nil {
		return branchProtectionFailure(stderr, err)
	}
	appendBranchProtectionAudit(&report, changes, rulesets)
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

func runRepoUpdate(repo string, confirmDeletions, acceptVisibility bool, stdout, stderr io.Writer) Result {
	repo, cfg, remote, fail := loadRepoContext(repo, stderr)
	if fail != nil {
		return *fail
	}
	plan := buildUpdatePlan(cfg, remote)
	branchChanges, _, err := loadBranchProtectionPlan(repo, cfg.Repository.BranchProtection)
	if err != nil {
		return branchProtectionFailure(stderr, err)
	}
	rulesetsToDelete := 0
	for _, change := range branchChanges {
		if change.Operation == "delete" {
			rulesetsToDelete++
		}
	}
	if plan.VisibilityChange && !acceptVisibility {
		err := fmt.Errorf("visibility would change from %s to %s. Re-run with --accept-visibility-change-consequences if that is intentional", remote.Visibility, cfg.Repository.Visibility)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitBlockedVisibility, Err: err}
	}
	if (len(plan.TopicsToRemove) > 0 || len(plan.LabelsToDelete) > 0 || rulesetsToDelete > 0) && !confirmDeletions {
		err := fmt.Errorf("update would delete %d labels, %d topics, and %d branch rulesets. Re-run with --confirm-deletions if that is intentional", len(plan.LabelsToDelete), len(plan.TopicsToRemove), rulesetsToDelete)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitBlockedDeletions, Err: err}
	}

	var settingsUpdated, topicsChanged, labelsChanged, branchUpdated int
	incomplete := func(phase string, err error) Result {
		headline := "UPDATE INCOMPLETE"
		color := cfg.DevOutput.Color
		if color == "" || color == "auto" {
			color = "false"
			if file, ok := stderr.(*os.File); ok {
				if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
					color = "auto"
				}
			}
		}
		if useColor(color) {
			headline = "\x1b[31m" + headline + "\x1b[0m"
		}
		_, _ = fmt.Fprintln(stderr, headline)
		_, _ = fmt.Fprintf(stderr, "Failed during %s: %v\n", phase, err)
		_, _ = fmt.Fprintf(stderr, "Confirmed changes: %d repo settings, %d topic changes, %d label changes, %d branch ruleset changes.\n", settingsUpdated, topicsChanged, labelsChanged, branchUpdated)
		_, _ = fmt.Fprintf(stderr, "Changes may remain because no rollback was attempted. Run gh flarebyte repo audit --repo %s before retrying.\n", repo)
		return Result{ExitCode: ExitFailure, Err: err}
	}

	if plan.SettingsChanged {
		if err := applyRepoSettings(repo, plan.SettingsPatch); err != nil {
			return incomplete("repository settings update", err)
		}
		settingsUpdated = plan.SettingsChangeCount
	}
	for _, topic := range plan.TopicsToAdd {
		if err := addRepoTopic(repo, topic); err != nil {
			return incomplete("topic update", fmt.Errorf("add topic %q: %w", topic, err))
		}
		topicsChanged++
	}
	for _, topic := range plan.TopicsToRemove {
		if err := removeRepoTopic(repo, topic); err != nil {
			return incomplete("topic update", fmt.Errorf("remove topic %q: %w", topic, err))
		}
		topicsChanged++
	}
	for _, label := range plan.LabelsToCreate {
		if err := createRepoLabel(repo, label); err != nil {
			return incomplete("label update", fmt.Errorf("create label %q: %w", label.Name, err))
		}
		labelsChanged++
	}
	for _, label := range plan.LabelsToUpdate {
		if err := updateRepoLabel(repo, label); err != nil {
			return incomplete("label update", fmt.Errorf("update label %q: %w", label.Name, err))
		}
		labelsChanged++
	}
	for _, label := range plan.LabelsToDelete {
		if err := deleteRepoLabel(repo, label.Name); err != nil {
			return incomplete("label update", fmt.Errorf("delete label %q: %w", label.Name, err))
		}
		labelsChanged++
	}
	branchUpdated, err = applyBranchProtection(repo, branchChanges, stdout)
	if err != nil {
		return incomplete("branch ruleset update", err)
	}
	if branchUpdated > 0 {
		if err := verifyBranchProtection(repo, cfg.Repository.BranchProtection); err != nil {
			return incomplete("branch ruleset verification", err)
		}
	}

	if cfg.Repository.BranchProtection != nil {
		_, _ = fmt.Fprintf(stdout, "Branch protection: %d ruleset changes applied.\n", branchUpdated)
	}
	_, _ = fmt.Fprintf(stdout, "Update complete: %d repo settings updated, %d topics synced, %d labels reconciled.\n", settingsUpdated, len(cfg.Repository.Topics), len(cfg.Repository.Labels))
	return Result{ExitCode: ExitOK}
}

func runReposMine(org string, asJSON bool, stdout, stderr io.Writer) Result {
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
