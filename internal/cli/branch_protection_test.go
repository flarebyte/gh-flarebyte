package cli

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func testBranchRulesets(t *testing.T) []branchRuleset {
	t.Helper()
	defs := desiredBranchRulesets(config.BranchProtectionConfig{Enabled: true, AllowAdminCIBypass: true}, 123)
	var remote []branchRuleset
	for i, def := range defs {
		// Simulate API decoding, including float64 numbers in parameter maps.
		data, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		var decoded branchRulesetDefinition
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		remote = append(remote, branchRuleset{ID: int64(i + 1), Source: "owner/repo", SourceType: "Repository", Definition: decoded})
	}
	return remote
}

func TestBranchProtectionPlan(t *testing.T) {
	policy := &config.BranchProtectionConfig{Enabled: true, AllowAdminCIBypass: true}
	changes, err := planBranchProtection(policy, "owner/repo", nil, 123)
	if err != nil || len(changes) != 2 {
		t.Fatalf("create: %+v %v", changes, err)
	}
	for i, change := range changes {
		if change.Operation != "create" || change.ID != 0 {
			t.Fatalf("bad create: %+v", change)
		}
		if change.Definition.Target != "branch" || change.Definition.Enforcement != "active" || !reflect.DeepEqual(change.Definition.Conditions.RefName.Include, []string{"~DEFAULT_BRANCH"}) {
			t.Fatalf("wrong target: %+v", change)
		}
		if i == 0 && len(change.Definition.BypassActors) != 0 {
			t.Fatal("protection must have no bypass")
		}
	}
	remote := testBranchRulesets(t)
	// Different order and empty API arrays must not cause perpetual updates.
	slices.Reverse(remote[0].Definition.Rules)
	remote[0].Definition.BypassActors = nil
	remote[0].Definition.Conditions.RefName.Exclude = nil
	before, _ := json.Marshal(remote)
	changes, err = planBranchProtection(policy, "OWNER/REPO", remote, 123)
	if err != nil || len(changes) != 0 {
		t.Fatalf("no-op: %+v %v", changes, err)
	}
	after, _ := json.Marshal(remote)
	if string(before) != string(after) {
		t.Fatal("planner changed caller-owned state")
	}

	policy.AllowAdminCIBypass = false
	changes, err = planBranchProtection(policy, "owner/repo", remote, 123)
	if err != nil || len(changes) != 1 || changes[0].Operation != "update" || changes[0].ID != 2 || len(changes[0].Definition.BypassActors) != 0 {
		t.Fatalf("bypass update: %+v %v", changes, err)
	}

	policy.Enabled = false
	changes, err = planBranchProtection(policy, "owner/repo", remote, 0)
	if err != nil || len(changes) != 2 {
		t.Fatalf("delete: %+v %v", changes, err)
	}
	for _, change := range changes {
		if change.Operation != "delete" || change.ID <= 0 {
			t.Fatalf("bad deletion: %+v", change)
		}
	}
	changes, err = planBranchProtection(policy, "owner/repo", nil, 0)
	if err != nil || len(changes) != 0 {
		t.Fatalf("disabled no-op: %+v %v", changes, err)
	}
}

func TestBranchProtectionRequiredPolicy(t *testing.T) {
	defs := desiredBranchRulesets(config.BranchProtectionConfig{Enabled: true, AllowAdminCIBypass: true}, 123)
	protection, ci := defs[0], defs[1]
	if protection.Name != branchProtectionRulesetName || ci.Name != branchCIRulesetName {
		t.Fatal("managed identities changed")
	}
	if len(protection.Rules) != 3 || protection.Rules[0].Type != "deletion" || protection.Rules[1].Type != "non_fast_forward" || protection.Rules[2].Type != "pull_request" {
		t.Fatalf("unexpected protection: %+v", protection.Rules)
	}
	pr := protection.Rules[2].Parameters
	if pr["required_approving_review_count"] != 0 {
		t.Fatal("solo maintainer must not require approval")
	}
	for _, key := range []string{"dismiss_stale_reviews_on_push", "require_code_owner_review", "require_last_push_approval", "required_review_thread_resolution"} {
		if pr[key] != false {
			t.Fatalf("unexpected review restriction %s", key)
		}
	}
	if len(ci.Rules) != 1 || ci.Rules[0].Type != "required_status_checks" {
		t.Fatalf("unexpected CI rules: %+v", ci.Rules)
	}
	wantChecks := []map[string]any{{"context": "CI passed", "integration_id": int64(123)}}
	if !reflect.DeepEqual(ci.Rules[0].Parameters["required_status_checks"], wantChecks) || ci.Rules[0].Parameters["strict_required_status_checks_policy"] != false {
		t.Fatalf("unexpected CI requirements: %+v", ci.Rules)
	}
	if !reflect.DeepEqual(ci.BypassActors, []branchBypassActor{{ActorID: 5, ActorType: "RepositoryRole", BypassMode: "pull_request"}}) {
		t.Fatalf("unexpected bypass: %+v", ci.BypassActors)
	}
	remote := testBranchRulesets(t)
	remote = append(remote, branchRuleset{ID: 99, Definition: branchRulesetDefinition{Name: "manual-policy"}})
	changes, err := planBranchProtection(&config.BranchProtectionConfig{Enabled: false}, "owner/repo", remote, 0)
	if err != nil || len(changes) != 2 {
		t.Fatalf("disable: %+v %v", changes, err)
	}
	for _, change := range changes {
		if change.ID == 99 {
			t.Fatal("unrelated rule selected for deletion")
		}
	}
}

func TestBranchProtectionOwnership(t *testing.T) {
	policy := &config.BranchProtectionConfig{Enabled: true, AllowAdminCIBypass: true}
	for _, name := range []string{"duplicate", "foreign", "organization", "missing ID"} {
		t.Run(name, func(t *testing.T) {
			remote := testBranchRulesets(t)
			switch name {
			case "duplicate":
				remote = append(remote, remote[0])
			case "foreign":
				remote[0].Source = "another/repo"
			case "organization":
				remote[0].SourceType = "Organization"
			case "missing ID":
				remote[0].ID = 0
			}
			changes, err := planBranchProtection(policy, "owner/repo", remote, 123)
			if err == nil || len(changes) != 0 {
				t.Fatalf("expected error without partial plan: %+v %v", changes, err)
			}
			changes, err = planBranchProtection(nil, "owner/repo", remote, 0)
			if err != nil || len(changes) != 0 {
				t.Fatalf("omitted must be unmanaged: %+v %v", changes, err)
			}
		})
	}
	unrelated := branchRuleset{ID: 99, Definition: branchRulesetDefinition{Name: "manual-policy"}}
	changes, err := planBranchProtection(policy, "owner/repo", []branchRuleset{unrelated}, 123)
	if err != nil || len(changes) != 2 {
		t.Fatalf("unrelated ruleset: %+v %v", changes, err)
	}
	if _, err := planBranchProtection(policy, "owner/repo", nil, 0); err == nil {
		t.Fatal("missing Actions identity accepted")
	}
}

func TestBranchProtectionDrift(t *testing.T) {
	for _, scenario := range []string{"disabled", "target", "rule", "approvals", "extra parameter", "bypass", "check source"} {
		t.Run(scenario, func(t *testing.T) {
			remote := testBranchRulesets(t)
			def := &remote[0].Definition
			switch scenario {
			case "disabled":
				def.Enforcement = "disabled"
			case "target":
				def.Conditions.RefName.Include = []string{"refs/heads/main"}
			case "rule":
				def.Rules = def.Rules[1:]
			case "approvals":
				def.Rules[2].Parameters["required_approving_review_count"] = 1
			case "extra parameter":
				def.Rules[2].Parameters["unexpected"] = true
			case "bypass":
				def.BypassActors = []branchBypassActor{{ActorID: 5, ActorType: "RepositoryRole", BypassMode: "always"}}
			case "check source":
				remote[1].Definition.Rules[0].Parameters["required_status_checks"] = []map[string]any{{"context": branchCICheckName, "integration_id": 999}}
			}
			changes, err := planBranchProtection(&config.BranchProtectionConfig{Enabled: true, AllowAdminCIBypass: true}, "owner/repo", remote, 123)
			if err != nil || len(changes) != 1 || changes[0].Operation != "update" {
				t.Fatalf("drift %s: %+v %v", scenario, changes, err)
			}
		})
	}
}

func TestBranchProtectionAPIDefaults(t *testing.T) {
	remote := testBranchRulesets(t)
	params := remote[0].Definition.Rules[2].Parameters
	if err := json.Unmarshal([]byte(`{
		"allowed_merge_methods":["squash","rebase","merge"],
		"required_reviewers":[],
		"dismissal_restriction":{"allowed_actors":[],"enabled":false},
		"require_extra_approval_for_unattributed_changes":true
	}`), &params); err != nil {
		t.Fatal(err)
	}
	remote[1].Definition.Rules[0].Parameters["do_not_enforce_on_create"] = false
	policy := &config.BranchProtectionConfig{Enabled: true, AllowAdminCIBypass: true}
	changes, err := planBranchProtection(policy, "owner/repo", remote, 123)
	if err != nil || len(changes) != 0 {
		t.Fatalf("API defaults caused drift: %+v %v", changes, err)
	}

	for _, scenario := range []struct {
		name, key, value string
	}{
		{"restricted merge methods", "allowed_merge_methods", `["squash"]`},
		{"enabled dismissal restriction", "dismissal_restriction", `{"allowed_actors":[],"enabled":true}`},
		{"inactive dismissal with actors", "dismissal_restriction", `{"allowed_actors":[{"id":1,"type":"Team"}],"enabled":false}`},
		{"required reviewers", "required_reviewers", `[{"reviewer":{"id":1,"type":"Team"},"file_patterns":["*"],"minimum_approvals":1}]`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			changed := maps.Clone(params)
			var value any
			if err := json.Unmarshal([]byte(scenario.value), &value); err != nil {
				t.Fatal(err)
			}
			changed[scenario.key] = value
			remote[0].Definition.Rules[2].Parameters = changed
			changes, err := planBranchProtection(policy, "owner/repo", remote, 123)
			if err != nil || len(changes) != 1 || changes[0].Operation != "update" {
				t.Fatalf("expected update: %+v %v", changes, err)
			}
		})
	}
}

func TestBranchProtectionExtraApprovals(t *testing.T) {
	for _, count := range []struct {
		name  string
		value any
	}{
		{"positive count", float64(1)},
		{"missing count", nil},
		{"unknown count", "0"},
	} {
		t.Run(count.name, func(t *testing.T) {
			withApproval := testBranchRulesets(t)[0].Definition
			withoutApproval := testBranchRulesets(t)[0].Definition
			for _, def := range []branchRulesetDefinition{withApproval, withoutApproval} {
				params := def.Rules[2].Parameters
				if count.value == nil {
					delete(params, "required_approving_review_count")
				} else {
					params["required_approving_review_count"] = count.value
				}
			}
			withApproval.Rules[2].Parameters["require_extra_approval_for_unattributed_changes"] = true
			equal, err := equalBranchRulesets(withApproval, withoutApproval)
			if err != nil || equal {
				t.Fatalf("expected extra approval drift: equal=%t err=%v", equal, err)
			}
		})
	}
}
