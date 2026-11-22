package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func loadBranchProtectionPlan(repo string, policy *config.BranchProtectionConfig) ([]branchRulesetChange, []branchRuleset, error) {
	if policy == nil {
		return nil, nil, nil
	}
	remote, err := readBranchRulesets(repo)
	if err != nil {
		return nil, nil, err
	}
	var appID int64
	if policy.Enabled {
		appID, err = readActionsAppID()
		if err != nil {
			return nil, nil, err
		}
	}
	changes, err := planBranchProtection(policy, repo, remote, appID)
	return changes, remote, err
}

func appendBranchProtectionAudit(report *AuditReport, changes []branchRulesetChange, remote []branchRuleset) {
	for _, change := range changes {
		var local, actual any
		if change.Operation != "delete" {
			local = change.Definition
		}
		for _, rule := range remote {
			if rule.ID == change.ID {
				actual = rule.Definition
				break
			}
		}
		field := "repository.branchProtection.protection"
		if change.Definition.Name == branchCIRulesetName {
			field = "repository.branchProtection.ci"
		}
		report.Diffs = append(report.Diffs, AuditDiff{Field: field, Local: local, Remote: actual})
	}
	report.DriftCount = len(report.Diffs)
	report.HasDrift = report.DriftCount > 0
}

func branchProtectionFailure(stderr io.Writer, err error) Result {
	_, _ = fmt.Fprintln(stderr, err)
	return Result{ExitCode: ExitFailure, Err: err}
}

func applyBranchProtection(repo string, changes []branchRulesetChange, stdout io.Writer) (int, error) {
	completed := 0
	for _, change := range changes {
		if err := applyBranchRulesetChange(repo, change); err != nil {
			return completed, fmt.Errorf("%s ruleset %s: %w", change.Operation, change.Definition.Name, err)
		}
		completed++
		_, _ = fmt.Fprintf(stdout, "Ruleset %s: GitHub accepted %s.\n", change.Definition.Name, change.Operation)
	}
	return completed, nil
}

func verifyBranchProtection(repo string, policy *config.BranchProtectionConfig) error {
	remaining, _, err := loadBranchProtectionPlan(repo, policy)
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		names := make([]string, 0, len(remaining))
		for _, change := range remaining {
			names = append(names, change.Definition.Name)
		}
		return fmt.Errorf("rulesets still differ after update: %s", strings.Join(names, ", "))
	}
	return nil
}
