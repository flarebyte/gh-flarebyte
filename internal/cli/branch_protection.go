package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

const (
	branchProtectionRulesetName = "gh-flarebyte-default-branch-protection"
	branchCIRulesetName         = "gh-flarebyte-default-branch-ci"
	branchCICheckName           = "CI passed"
)

// Metadata is separate from the writable definition to avoid sending read-only
// API fields back to GitHub. Reserved names identify CLI-managed rulesets.
type branchRuleset struct {
	ID         int64
	Source     string
	SourceType string
	Definition branchRulesetDefinition
}

type branchRulesetDefinition struct {
	Name         string                  `json:"name"`
	Target       string                  `json:"target"`
	Enforcement  string                  `json:"enforcement"`
	Conditions   branchRulesetConditions `json:"conditions"`
	Rules        []branchRule            `json:"rules"`
	BypassActors []branchBypassActor     `json:"bypass_actors"`
}

type branchRulesetConditions struct {
	RefName branchRefCondition `json:"ref_name"`
}

type branchRefCondition struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type branchRule struct {
	Type       string         `json:"type"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type branchBypassActor struct {
	ActorID    int64  `json:"actor_id"`
	ActorType  string `json:"actor_type"`
	BypassMode string `json:"bypass_mode"`
}

type branchRulesetChange struct {
	Operation  string // create, update, delete
	ID         int64
	Definition branchRulesetDefinition
}

func desiredBranchRulesets(policy config.BranchProtectionConfig, actionsAppID int64) []branchRulesetDefinition {
	protection := branchRulesetDefinition{
		Name: branchProtectionRulesetName, Target: "branch", Enforcement: "active",
		Conditions:   branchRulesetConditions{RefName: branchRefCondition{Include: []string{"~DEFAULT_BRANCH"}, Exclude: []string{}}},
		BypassActors: []branchBypassActor{},
		Rules: []branchRule{
			{Type: "deletion"}, {Type: "non_fast_forward"},
			{Type: "pull_request", Parameters: map[string]any{
				"required_approving_review_count":   0,
				"dismiss_stale_reviews_on_push":     false,
				"require_code_owner_review":         false,
				"require_last_push_approval":        false,
				"required_review_thread_resolution": false,
			}},
		},
	}
	ci := protection
	ci.Name = branchCIRulesetName
	ci.Rules = []branchRule{{Type: "required_status_checks", Parameters: map[string]any{
		"strict_required_status_checks_policy": false,
		"required_status_checks":               []map[string]any{{"context": branchCICheckName, "integration_id": actionsAppID}},
	}}}
	if policy.AllowAdminCIBypass {
		ci.BypassActors = []branchBypassActor{{ActorID: 5, ActorType: "RepositoryRole", BypassMode: "pull_request"}}
	}
	return []branchRulesetDefinition{protection, ci}
}

// Pure planning: no remote reads, writes, or changes to caller-owned state.
// The adapter must supply complete remote definitions, not list summaries.
func planBranchProtection(policy *config.BranchProtectionConfig, repo string, remote []branchRuleset, actionsAppID int64) ([]branchRulesetChange, error) {
	if policy == nil {
		return nil, nil
	}
	if policy.Enabled && actionsAppID <= 0 {
		return nil, fmt.Errorf("branch protection requires a valid GitHub Actions app ID")
	}
	managed := map[string]branchRuleset{}
	for _, rule := range remote {
		name := rule.Definition.Name
		if name != branchProtectionRulesetName && name != branchCIRulesetName {
			continue
		}
		if _, exists := managed[name]; exists {
			return nil, fmt.Errorf("ambiguous managed ruleset %q", name)
		}
		if rule.ID <= 0 || rule.SourceType != "Repository" || !strings.EqualFold(rule.Source, repo) {
			return nil, fmt.Errorf("cannot manage ruleset %q: invalid ID or repository ownership", name)
		}
		managed[name] = rule
	}
	var changes []branchRulesetChange
	for _, desired := range desiredBranchRulesets(*policy, actionsAppID) {
		current, exists := managed[desired.Name]
		if !policy.Enabled {
			if exists {
				changes = append(changes, branchRulesetChange{Operation: "delete", ID: current.ID, Definition: current.Definition})
			}
			continue
		}
		operation := "create"
		if exists {
			equal, err := equalBranchRulesets(current.Definition, desired)
			if err != nil {
				return nil, err
			}
			if equal {
				continue
			}
			operation = "update"
		}
		changes = append(changes, branchRulesetChange{Operation: operation, ID: current.ID, Definition: desired})
	}
	return changes, nil
}

// Rule order is not significant; canonical JSON also normalizes decoded numeric
// values and map ordering without discarding unknown rule parameters.
func equalBranchRulesets(a, b branchRulesetDefinition) (bool, error) {
	encode := func(def branchRulesetDefinition) (string, error) {
		def.Rules = slices.Clone(def.Rules)
		for i := range def.Rules {
			def.Rules[i].Parameters = normalizeBranchRuleParameters(def.Rules[i])
		}
		slices.SortFunc(def.Rules, func(a, b branchRule) int { return strings.Compare(a.Type, b.Type) })
		if len(def.BypassActors) == 0 {
			def.BypassActors = []branchBypassActor{}
		}
		if len(def.Conditions.RefName.Exclude) == 0 {
			def.Conditions.RefName.Exclude = []string{}
		}
		data, err := json.Marshal(def)
		return string(data), err
	}
	left, err := encode(a)
	if err != nil {
		return false, fmt.Errorf("invalid remote ruleset %q: %w", a.Name, err)
	}
	right, err := encode(b)
	return left == right, err
}

// GitHub may return optional defaults absent from the write payload. Ignore only
// known nonrestrictive defaults; retain other parameters so extra restrictions
// or weakened policy still produce drift.
func normalizeBranchRuleParameters(rule branchRule) map[string]any {
	params := maps.Clone(rule.Parameters)
	removeDefault := func(key, expected string) {
		data, err := json.Marshal(params[key])
		if err == nil && string(data) == expected {
			delete(params, key)
		}
	}
	switch rule.Type {
	case "pull_request":
		removeDefault("dismissal_restriction", `{"allowed_actors":[],"enabled":false}`)
		// GitHub applies the extra approval only when approvals are required.
		if count, err := json.Marshal(params["required_approving_review_count"]); err == nil && string(count) == "0" {
			delete(params, "require_extra_approval_for_unattributed_changes")
		}
		removeDefault("required_reviewers", "[]")
		removeDefault("required_reviewers", "null")
		data, err := json.Marshal(params["allowed_merge_methods"])
		var methods []string
		if err == nil && json.Unmarshal(data, &methods) == nil {
			slices.Sort(methods)
			if slices.Equal(methods, []string{"merge", "rebase", "squash"}) {
				delete(params, "allowed_merge_methods")
			}
		}
	case "required_status_checks":
		removeDefault("do_not_enforce_on_create", "false")
	}
	return params
}
