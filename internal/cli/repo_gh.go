// purpose: Encapsulate GitHub CLI/API I/O for repository metadata reads and repository mutation operations.
// responsibilities: Read repo/repositories state from gh; translate JSON payloads into internal models; apply settings/topics/labels via gh commands.
// architecture notes: repositoryTopics decoding accepts both connection and array shapes to tolerate upstream gh output variations without breaking audit/update.
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

type repoTopicNode struct {
	Topic struct {
		Name string `json:"name"`
	} `json:"topic"`
}

type repoTopicsField struct {
	Nodes []repoTopicNode `json:"nodes"`
}

func (f *repoTopicsField) UnmarshalJSON(data []byte) error {
	var asConnection struct {
		Nodes []repoTopicNode `json:"nodes"`
	}
	if err := json.Unmarshal(data, &asConnection); err == nil && asConnection.Nodes != nil {
		f.Nodes = asConnection.Nodes
		return nil
	}

	var asArray []repoTopicNode
	if err := json.Unmarshal(data, &asArray); err == nil {
		f.Nodes = asArray
		return nil
	}

	// Keep strict behavior for unexpected payloads.
	return json.Unmarshal(data, &asConnection)
}

func ghReadRepoMetadata(repo string) (RepoMetadata, error) {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return defaultRepoMetadata(repo), nil
	}
	cmd := exec.Command(
		"gh", "repo", "view", repo,
		"--json", "description,defaultBranchRef,homepageUrl,isPrivate,isTemplate,mergeCommitAllowed,rebaseMergeAllowed,squashMergeAllowed,deleteBranchOnMerge,repositoryTopics,labels",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return RepoMetadata{}, commandError(err, stderr.String())
	}
	var payload struct {
		Description         string `json:"description"`
		HomepageURL         string `json:"homepageUrl"`
		IsPrivate           bool   `json:"isPrivate"`
		IsTemplate          bool   `json:"isTemplate"`
		MergeCommitAllowed  bool   `json:"mergeCommitAllowed"`
		RebaseMergeAllowed  bool   `json:"rebaseMergeAllowed"`
		SquashMergeAllowed  bool   `json:"squashMergeAllowed"`
		DeleteBranchOnMerge bool   `json:"deleteBranchOnMerge"`
		DefaultBranchRef    struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
		RepositoryTopics repoTopicsField `json:"repositoryTopics"`
		Labels           []struct {
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
		Description:         payload.Description,
		DefaultBranch:       payload.DefaultBranchRef.Name,
		Homepage:            payload.HomepageURL,
		Visibility:          visibility,
		Template:            payload.IsTemplate,
		MergeCommit:         payload.MergeCommitAllowed,
		RebaseMerge:         payload.RebaseMergeAllowed,
		SquashMerge:         payload.SquashMergeAllowed,
		DeleteBranchOnMerge: payload.DeleteBranchOnMerge,
		Topics:              extractTopics(payload.RepositoryTopics.Nodes),
		Labels:              extractLabelsFromState(payload.Labels),
	}
	return meta, nil
}

func extractTopics(nodes []repoTopicNode) []string {
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
		return "", nil, commandError(err, stderr.String())
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
	if desired.SetMergeCommit {
		args = append(args, "--enable-merge-commit="+boolToCLIValue(desired.MergeCommit))
	}
	if desired.SetRebaseMerge {
		args = append(args, "--enable-rebase-merge="+boolToCLIValue(desired.RebaseMerge))
	}
	if desired.SetSquashMerge {
		args = append(args, "--enable-squash-merge="+boolToCLIValue(desired.SquashMerge))
	}
	if desired.SetDeleteBranchOnMerge {
		args = append(args, "--delete-branch-on-merge="+boolToCLIValue(desired.DeleteBranchOnMerge))
	}
	cmd := exec.Command("gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return commandError(err, stderr.String())
	}
	return nil
}

func boolToCLIValue(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func ghAddRepoTopic(repo string, topic string) error {
	return runGHReadonly("repo", "edit", repo, "--add-topic", topic)
}

func ghRemoveRepoTopic(repo string, topic string) error {
	return runGHReadonly("repo", "edit", repo, "--remove-topic", topic)
}

func ghCreateRepoLabel(repo string, label LabelState) error {
	return runGHReadonly("label", "create", label.Name, "--repo", repo, "--color", label.Color, "--description", label.Description, "--force")
}

func ghUpdateRepoLabel(repo string, label LabelState) error {
	return runGHReadonly("label", "edit", label.Name, "--repo", repo, "--color", label.Color, "--description", label.Description)
}

func ghDeleteRepoLabel(repo string, labelName string) error {
	return runGHReadonly("label", "delete", labelName, "--repo", repo, "--yes")
}

func runGHReadonly(args ...string) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_READONLY") == "1" {
		return nil
	}
	return runGH(args...)
}

func runGH(args ...string) error {
	cmd := exec.Command("gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return commandError(err, stderr.String())
	}
	return nil
}
