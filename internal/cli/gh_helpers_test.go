// purpose: Protect GitHub helper adaptation logic so metadata decoding and fake-mode shortcuts stay stable for automation.
// responsibilities: Test topic/label extraction, fake readonly/release behaviors, and selected helper edge branches.
// architecture notes: Uses environment-driven fake modes to avoid external `gh` calls while still validating integration helpers.
package cli

import (
	"encoding/json"
	"os"
	"testing"
)

func TestRepoTopicsFieldUnmarshalShapes(t *testing.T) {
	var conn repoTopicsField
	if err := json.Unmarshal([]byte(`{"nodes":[{"topic":{"name":"a"}}]}`), &conn); err != nil {
		t.Fatalf("unexpected connection unmarshal error: %v", err)
	}
	if len(conn.Nodes) != 1 {
		t.Fatalf("expected 1 node")
	}

	var arr repoTopicsField
	if err := json.Unmarshal([]byte(`[{"name":"b"}]`), &arr); err != nil {
		t.Fatalf("unexpected array unmarshal error: %v", err)
	}
	if len(arr.Nodes) != 1 {
		t.Fatalf("expected 1 node")
	}
}

func TestGHFakeReadonlyHelpers(t *testing.T) {
	old := os.Getenv("GH_FLAREBYTE_FAKE_READONLY")
	_ = os.Setenv("GH_FLAREBYTE_FAKE_READONLY", "1")
	t.Cleanup(func() { _ = os.Setenv("GH_FLAREBYTE_FAKE_READONLY", old) })

	if err := ghApplyRepoSettings("o/r", RepoSettingsPatch{}); err != nil {
		t.Fatalf("expected fake ghApplyRepoSettings success: %v", err)
	}
	if err := ghAddRepoTopic("o/r", "x"); err != nil {
		t.Fatalf("expected fake ghAddRepoTopic success: %v", err)
	}
	if err := ghRemoveRepoTopic("o/r", "x"); err != nil {
		t.Fatalf("expected fake ghRemoveRepoTopic success: %v", err)
	}
	if err := ghCreateRepoLabel("o/r", LabelState{Name: "bug", Color: "fff", Description: "d"}); err != nil {
		t.Fatalf("expected fake ghCreateRepoLabel success: %v", err)
	}
	if err := ghUpdateRepoLabel("o/r", LabelState{Name: "bug", Color: "fff", Description: "d"}); err != nil {
		t.Fatalf("expected fake ghUpdateRepoLabel success: %v", err)
	}
	if err := ghDeleteRepoLabel("o/r", "bug"); err != nil {
		t.Fatalf("expected fake ghDeleteRepoLabel success: %v", err)
	}
	user, repos, err := ghReadReposMine("flarebyte")
	if err != nil || user == "" || len(repos) == 0 {
		t.Fatalf("expected fake ghReadReposMine result, got user=%q repos=%d err=%v", user, len(repos), err)
	}
	meta, err := ghReadRepoMetadata("flarebyte/gh-flarebyte")
	if err != nil || meta.DefaultBranch == "" {
		t.Fatalf("expected fake ghReadRepoMetadata result, got %+v err=%v", meta, err)
	}
}

func TestReleaseFakeModeHelpersAndTernary(t *testing.T) {
	old := os.Getenv("GH_FLAREBYTE_FAKE_RELEASE")
	_ = os.Setenv("GH_FLAREBYTE_FAKE_RELEASE", "1")
	t.Cleanup(func() { _ = os.Setenv("GH_FLAREBYTE_FAKE_RELEASE", old) })

	exists, err := ghTagExists("v1.2.3")
	if err != nil || exists {
		t.Fatalf("expected fake ghTagExists false,nil got exists=%v err=%v", exists, err)
	}
	if err := ghCreateRelease("v1.2.3", nil, "generate-notes", "", false); err != nil {
		t.Fatalf("expected fake ghCreateRelease success: %v", err)
	}
	if ternary(true, "yes", "no") != "yes" || ternary(false, "yes", "no") != "no" {
		t.Fatalf("unexpected ternary behavior")
	}
}

func TestExtractHelpersAndBoolToCLI(t *testing.T) {
	nodes := []repoTopicNode{{Name: "fallback"}, {}}
	nodes[1].Topic.Name = "primary"
	topics := extractTopics(nodes)
	if len(topics) != 2 || topics[0] != "fallback" || topics[1] != "primary" {
		t.Fatalf("unexpected topics: %#v", topics)
	}
	labels := extractLabelsFromState([]struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}{{Name: "bug", Color: "fff", Description: "x"}})
	if len(labels) != 1 || labels[0].Name != "bug" {
		t.Fatalf("unexpected labels: %#v", labels)
	}
	if boolToCLIValue(true) != "true" || boolToCLIValue(false) != "false" {
		t.Fatalf("unexpected boolToCLIValue")
	}
}

func TestBuildHelperDirectBranches(t *testing.T) {
	if err := runGoBuildPackages("", "", []string{"./does/not/exist"}, false); err == nil {
		t.Fatalf("expected runGoBuildPackages error")
	}
	if err := goBuildTargetBinary("invalid-target", "out", "./cmd/x"); err == nil {
		t.Fatalf("expected goBuildTargetBinary target parse error")
	}
}
