package cli

import (
	"encoding/json"
	"testing"
)

func TestRepoTopicsFieldUnmarshalConnectionShape(t *testing.T) {
	var payload struct {
		RepositoryTopics repoTopicsField `json:"repositoryTopics"`
	}
	raw := []byte(`{"repositoryTopics":{"nodes":[{"topic":{"name":"go"}},{"topic":{"name":"cli"}}]}}`)
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	got := extractTopics(payload.RepositoryTopics.Nodes)
	if len(got) != 2 || got[0] != "go" || got[1] != "cli" {
		t.Fatalf("unexpected topics: %#v", got)
	}
}

func TestRepoTopicsFieldUnmarshalArrayShape(t *testing.T) {
	var payload struct {
		RepositoryTopics repoTopicsField `json:"repositoryTopics"`
	}
	raw := []byte(`{"repositoryTopics":[{"topic":{"name":"validation"}},{"topic":{"name":"schema"}}]}`)
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	got := extractTopics(payload.RepositoryTopics.Nodes)
	if len(got) != 2 || got[0] != "validation" || got[1] != "schema" {
		t.Fatalf("unexpected topics: %#v", got)
	}
}

func TestRepoTopicsFieldUnmarshalUnexpectedShapeFails(t *testing.T) {
	var payload struct {
		RepositoryTopics repoTopicsField `json:"repositoryTopics"`
	}
	raw := []byte(`{"repositoryTopics":"bad-shape"}`)
	if err := json.Unmarshal(raw, &payload); err == nil {
		t.Fatal("expected unmarshal error for invalid repositoryTopics shape")
	}
}
