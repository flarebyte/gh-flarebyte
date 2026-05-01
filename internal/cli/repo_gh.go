package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

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
		return RepoMetadata{}, commandError(err, stderr.String())
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
	cmd := exec.Command("gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return commandError(err, stderr.String())
	}
	return nil
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
