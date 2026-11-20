package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

type branchCommandFixture struct {
	remote            []branchRuleset
	writes            []branchRulesetChange
	readErr, writeErr error
	reads             int
}

func setupBranchCommand(t *testing.T, enabled string) *branchCommandFixture {
	t.Helper()
	cue := testConfigCue()
	if enabled != "" {
		cue = strings.Replace(cue, "repository: {", "repository: {\nbranchProtection: { enabled: "+enabled+" }", 1)
	}
	setupTempWorkdirWithConfig(t, cue)
	oldMeta, oldRead, oldApp, oldApply := readRepoMetadata, readBranchRulesets, readActionsAppID, applyBranchRulesetChange
	t.Cleanup(func() {
		readRepoMetadata, readBranchRulesets, readActionsAppID, applyBranchRulesetChange = oldMeta, oldRead, oldApp, oldApply
	})
	f := &branchCommandFixture{}
	readRepoMetadata = func(string) (RepoMetadata, error) { return baseRepoMetadata(), nil }
	readBranchRulesets = func(string) ([]branchRuleset, error) { f.reads++; return f.remote, f.readErr }
	readActionsAppID = func() (int64, error) { return 123, nil }
	applyBranchRulesetChange = func(repo string, change branchRulesetChange) error {
		if f.writeErr != nil {
			return f.writeErr
		}
		f.writes = append(f.writes, change)
		switch change.Operation {
		case "create":
			f.remote = append(f.remote, branchRuleset{ID: int64(len(f.remote) + 1), Source: repo, SourceType: "Repository", Definition: change.Definition})
		case "update", "delete":
			for i, r := range f.remote {
				if r.ID == change.ID {
					if change.Operation == "delete" {
						f.remote = append(f.remote[:i], f.remote[i+1:]...)
					} else {
						f.remote[i].Definition = change.Definition
					}
					break
				}
			}
		}
		return nil
	}
	return f
}

func runBranchCommand(args ...string) (Result, string, string) {
	var out, stderr bytes.Buffer
	result := Run(args, &out, &stderr)
	return result, out.String(), stderr.String()
}

func TestBranchProtectionUpdate(t *testing.T) {
	f := setupBranchCommand(t, "true")
	r, out, errOut := runBranchCommand("repo", "update")
	if r.ExitCode != ExitOK || len(f.writes) != 2 || !strings.Contains(out, "2 ruleset changes") {
		t.Fatalf("enable: %+v %s %s", r, out, errOut)
	}
	if f.writes[0].Definition.Name != branchProtectionRulesetName {
		t.Fatal("must apply protection before CI")
	}
	r, out, errOut = runBranchCommand("repo", "update")
	if r.ExitCode != ExitOK || len(f.writes) != 2 || !strings.Contains(out, "0 ruleset changes") {
		t.Fatalf("rerun: %+v %s %s", r, out, errOut)
	}
	r, out, errOut = runBranchCommand("repo", "audit", "--json")
	if r.ExitCode != ExitOK || !strings.Contains(out, `"hasDrift":false`) {
		t.Fatalf("post-update audit: %+v %s %s", r, out, errOut)
	}
}

func TestBranchProtectionUpdateDeletionGate(t *testing.T) {
	f := setupBranchCommand(t, "false")
	f.remote = testBranchRulesets(t)
	for i := range f.remote {
		f.remote[i].Source = "flarebyte/gh-flarebyte"
	}
	readActionsAppID = func() (int64, error) { t.Fatal("disabled must not fetch app identity"); return 0, nil }
	oldSettings := applyRepoSettings
	t.Cleanup(func() { applyRepoSettings = oldSettings })
	applyRepoSettings = func(string, RepoSettingsPatch) error { t.Fatal("deletion gate must precede all mutations"); return nil }
	readRepoMetadata = func(string) (RepoMetadata, error) { r := baseRepoMetadata(); r.Description = "old"; return r, nil }
	r, _, errOut := runBranchCommand("repo", "update")
	if r.ExitCode != ExitBlockedDeletions || len(f.writes) != 0 || !strings.Contains(errOut, "2 branch rulesets") {
		t.Fatalf("gate: %+v %s", r, errOut)
	}
	readRepoMetadata = func(string) (RepoMetadata, error) { return baseRepoMetadata(), nil }
	r, _, errOut = runBranchCommand("repo", "update", "--confirm-deletions")
	if r.ExitCode != ExitOK || len(f.writes) != 2 || len(f.remote) != 0 {
		t.Fatalf("delete: %+v %s", r, errOut)
	}
}

func TestBranchProtectionUpdateOmitted(t *testing.T) {
	f := setupBranchCommand(t, "")
	for _, args := range [][]string{{"repo", "update"}, {"repo", "audit", "--json"}} {
		r, _, errOut := runBranchCommand(args...)
		if r.ExitCode != ExitOK {
			t.Fatalf("omitted: %+v %s", r, errOut)
		}
	}
	if f.reads != 0 || len(f.writes) != 0 {
		t.Fatal("omitted config touched branch APIs")
	}
}

func TestBranchProtectionUpdateFailure(t *testing.T) {
	for _, scenario := range []string{"read", "app", "write", "partial", "verify"} {
		t.Run(scenario, func(t *testing.T) {
			f := setupBranchCommand(t, "true")
			switch scenario {
			case "read":
				f.readErr = errors.New("permission denied")
			case "app":
				readActionsAppID = func() (int64, error) { return 0, errors.New("app lookup failed") }
			case "write":
				f.writeErr = errors.New("ruleset write failed")
			case "partial":
				apply := applyBranchRulesetChange
				applyBranchRulesetChange = func(repo string, change branchRulesetChange) error {
					if len(f.writes) == 1 {
						return errors.New("CI write failed")
					}
					return apply(repo, change)
				}
			case "verify":
				applyBranchRulesetChange = func(string, branchRulesetChange) error { return nil }
			}
			r, out, stderr := runBranchCommand("repo", "update")
			if r.ExitCode != ExitFailure || strings.Contains(out, "Update complete") {
				t.Fatalf("false success: %+v %s %s", r, out, stderr)
			}
			if scenario == "partial" && (!strings.Contains(stderr, "1 branch ruleset changes") || !strings.Contains(stderr, "No rollback")) {
				t.Fatalf("missing progress: %s", stderr)
			}
			if (scenario == "read" || scenario == "app") && len(f.writes) > 0 {
				t.Fatal("preflight failure applied changes")
			}
		})
	}
}

func TestBranchProtectionAudit(t *testing.T) {
	f := setupBranchCommand(t, "true")
	for _, asJSON := range []bool{false, true} {
		args := []string{"repo", "audit"}
		if asJSON {
			args = append(args, "--json")
		}
		r, out, stderr := runBranchCommand(args...)
		if r.ExitCode != ExitDrift || !strings.Contains(out, "repository.branchProtection") {
			t.Fatalf("audit drift: %+v %s %s", r, out, stderr)
		}
		if asJSON {
			var report AuditReport
			if err := json.Unmarshal([]byte(out), &report); err != nil {
				t.Fatal(err)
			}
			if report.DriftCount != 2 || !report.HasDrift || report.Diffs[0].Remote != nil || report.Diffs[0].Local == nil {
				t.Fatalf("incorrect report: %+v", report)
			}
		}
	}
	if len(f.writes) != 0 {
		t.Fatal("audit mutated rulesets")
	}
	f.readErr = errors.New("forbidden")
	r, out, _ := runBranchCommand("repo", "audit", "--json")
	if r.ExitCode != ExitFailure || out != "" {
		t.Fatalf("read error treated as drift: %+v %s", r, out)
	}
}

func TestBranchProtectionAuditDisabled(t *testing.T) {
	f := setupBranchCommand(t, "false")
	f.remote = testBranchRulesets(t)
	for i := range f.remote {
		f.remote[i].Source = "flarebyte/gh-flarebyte"
	}
	r, out, stderr := runBranchCommand("repo", "audit", "--json")
	if r.ExitCode != ExitDrift || !strings.Contains(out, `"local":null`) {
		t.Fatalf("disabled drift: %+v %s %s", r, out, stderr)
	}
	// Ensure disabling/omitting remains an explicit file operation, never init import.
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o600); err != nil {
		t.Fatal(err)
	}
	f.reads = 0
	r, _, stderr = runBranchCommand("repo", "audit")
	if r.ExitCode != ExitOK || f.reads != 0 {
		t.Fatalf("omitted after enabled: %+v %s", r, stderr)
	}
}
