package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var (
	branchAPI                = ghBranchAPI
	readBranchRulesets       = ghReadBranchRulesets
	readActionsAppID         = ghReadActionsAppID
	applyBranchRulesetChange = ghApplyBranchRulesetChange
)

// Keep authentication and host selection in gh; send structured bodies on stdin.
func ghBranchAPI(method, endpoint string, body []byte) ([]byte, error) {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return nil, fmt.Errorf("branch protection API is unavailable in fake readonly mode")
	}
	args := []string{"api", "--method", method, endpoint, "-H", "Accept: application/vnd.github+json", "-H", "X-GitHub-Api-Version: 2022-11-28"}
	if body != nil {
		args = append(args, "--input", "-")
	}
	cmd := exec.Command("gh", args...)
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("branch protection %s %s failed (check repository permissions and ruleset eligibility): %w", method, endpoint, commandError(err, stderr.String()))
	}
	return stdout.Bytes(), nil
}

var branchRepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func branchRulesetEndpoint(repo string) (string, error) {
	if !branchRepoPattern.MatchString(repo) {
		return "", fmt.Errorf("invalid branch protection repository: expected owner/name")
	}
	return "repos/" + repo + "/rulesets", nil
}

type branchRulesetPayload struct {
	branchRulesetDefinition
	ID         int64  `json:"id"`
	Source     string `json:"source"`
	SourceType string `json:"source_type"`
}

func ghReadBranchRulesets(repo string) ([]branchRuleset, error) {
	base, err := branchRulesetEndpoint(repo)
	if err != nil {
		return nil, err
	}
	var remote []branchRuleset
	for page := 1; ; page++ {
		data, err := branchAPI("GET", fmt.Sprintf("%s?includes_parents=false&per_page=100&page=%d", base, page), nil)
		if err != nil {
			return nil, err
		}
		var summaries []branchRulesetPayload
		if err := json.Unmarshal(data, &summaries); err != nil || summaries == nil {
			return nil, fmt.Errorf("invalid branch ruleset list for %s", repo)
		}
		for _, summary := range summaries {
			name := summary.Name
			if name != branchProtectionRulesetName && name != branchCIRulesetName {
				continue
			}
			if summary.ID <= 0 || summary.SourceType != "Repository" || !strings.EqualFold(summary.Source, repo) {
				return nil, fmt.Errorf("invalid ownership for ruleset %q", name)
			}
			data, err := branchAPI("GET", fmt.Sprintf("%s/%d", base, summary.ID), nil)
			if err != nil {
				return nil, err
			}
			var full branchRulesetPayload
			if err := json.Unmarshal(data, &full); err != nil {
				return nil, fmt.Errorf("invalid ruleset %q: %w", name, err)
			}
			if full.ID != summary.ID || full.Name != name || full.SourceType != "Repository" || !strings.EqualFold(full.Source, repo) {
				return nil, fmt.Errorf("ruleset %q identity changed during read; rerun", name)
			}
			// GitHub can omit bypass actors when the caller lacks sufficient access.
			if full.BypassActors == nil || full.Rules == nil || full.Target == "" || full.Enforcement == "" || full.Conditions.RefName.Include == nil {
				return nil, fmt.Errorf("incomplete ruleset %q; check repository administration read permissions", name)
			}
			remote = append(remote, branchRuleset{ID: full.ID, Source: full.Source, SourceType: full.SourceType, Definition: full.branchRulesetDefinition})
		}
		if len(summaries) < 100 {
			return remote, nil
		}
	}
}

func ghReadActionsAppID() (int64, error) {
	data, err := branchAPI("GET", "apps/github-actions", nil)
	if err != nil {
		return 0, err
	}
	var app struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &app); err != nil || app.ID <= 0 {
		return 0, fmt.Errorf("invalid GitHub Actions app identity")
	}
	return app.ID, nil
}

func ghApplyBranchRulesetChange(repo string, change branchRulesetChange) error {
	base, err := branchRulesetEndpoint(repo)
	if err != nil {
		return err
	}
	method := "POST"
	switch change.Operation {
	case "create":
	case "update", "delete":
		if change.ID <= 0 {
			return fmt.Errorf("invalid ruleset ID for %s", change.Operation)
		}
		base = fmt.Sprintf("%s/%d", base, change.ID)
		method = "PUT"
		if change.Operation == "delete" {
			method = "DELETE"
		}
	default:
		return fmt.Errorf("invalid ruleset operation %q", change.Operation)
	}
	var body []byte
	if method != "DELETE" {
		body, err = json.Marshal(change.Definition)
		if err != nil {
			return err
		}
	}
	_, err = branchAPI(method, base, body)
	return err
}
