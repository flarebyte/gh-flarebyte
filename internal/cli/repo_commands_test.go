package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRepoInitRequiresRepo(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "init"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "--repo owner/name is required") {
		t.Fatalf("expected repo requirement error, got: %s", errOut.String())
	}
}

func TestRunRepoInitRejectsExistingWithoutOverwrite(t *testing.T) {
	oldReadRepoMetadata := readRepoMetadata
	oldFileExists := fileExists
	oldWriteFile := writeFile
	t.Cleanup(func() {
		readRepoMetadata = oldReadRepoMetadata
		fileExists = oldFileExists
		writeFile = oldWriteFile
	})
	fileExists = func(path string) bool { return true }
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "init", "--repo", "flarebyte/gh-flarebyte"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "already exists") {
		t.Fatalf("expected existing-file guidance, got: %s", errOut.String())
	}
}

func TestRunRepoInitWritesDefaultsWhenImportFails(t *testing.T) {
	tmpDir := setupTempWorkdirWithConfig(t, "")
	stubRepoInitIO(t, false, func(repo string) (RepoMetadata, error) {
		return RepoMetadata{}, errors.New("gh not authenticated")
	})
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "init", "--repo", "flarebyte/gh-flarebyte"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d, err=%v", ExitOK, result.ExitCode, result.Err)
	}
	if !strings.Contains(errOut.String(), "warning: unable to import remote metadata") {
		t.Fatalf("expected warning on stderr, got: %s", errOut.String())
	}
	if !strings.Contains(out.String(), "from defaults") {
		t.Fatalf("expected defaults success message, got: %s", out.String())
	}
	contentPath := filepath.Join(tmpDir, ".gh-flarebyte.cue")
	content, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("expected config file, read error: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "org:  \"flarebyte\"") || !strings.Contains(text, "repo: \"gh-flarebyte\"") {
		t.Fatalf("expected owner/repo in config, got: %s", text)
	}
}

func TestRunRepoInitOverwriteWritesImportedMetadata(t *testing.T) {
	tmpDir := setupTempWorkdirWithConfig(t, "")
	stubRepoInitIO(t, true, func(repo string) (RepoMetadata, error) {
		return RepoMetadata{
			Description:   "Imported description",
			DefaultBranch: "trunk",
			Homepage:      "https://example.test/repo",
			Visibility:    "private",
			Template:      true,
		}, nil
	})
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "init", "--repo", "flarebyte/gh-flarebyte", "--overwrite"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d, err=%v", ExitOK, result.ExitCode, result.Err)
	}
	content, err := os.ReadFile(filepath.Join(tmpDir, ".gh-flarebyte.cue"))
	if err != nil {
		t.Fatalf("expected config file, read error: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "defaultBranch: \"trunk\"") {
		t.Fatalf("expected imported branch, got: %s", text)
	}
}

func TestRunRepoAuditJSONNoDrift(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldReadRepoMetadata := readRepoMetadata
	t.Cleanup(func() { readRepoMetadata = oldReadRepoMetadata })
	readRepoMetadata = func(repo string) (RepoMetadata, error) { return baseRepoMetadata(), nil }
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "audit", "--json"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, result.ExitCode)
	}
	var report AuditReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if report.DriftCount != 0 || report.HasDrift {
		t.Fatalf("expected no drift, got %+v", report)
	}
}

func TestRunRepoAuditTextWithDrift(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldReadRepoMetadata := readRepoMetadata
	t.Cleanup(func() { readRepoMetadata = oldReadRepoMetadata })
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		meta := baseRepoMetadata()
		meta.Description = "Different description"
		meta.Topics = []string{"gh-extension", "github-cli"}
		return meta, nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "audit"}, &out, &errOut)
	if result.ExitCode != ExitDrift || !strings.Contains(out.String(), "differences found") {
		t.Fatalf("unexpected audit output: code=%d out=%s", result.ExitCode, out.String())
	}
}

func TestRunReposMineJSON(t *testing.T) {
	oldReadReposMine := readReposMine
	t.Cleanup(func() { readReposMine = oldReadReposMine })
	readReposMine = func(org string) (string, []ContributedRepo, error) {
		return "olivier", []ContributedRepo{{Owner: "flarebyte", Name: "gh-flarebyte", Visibility: "public", DefaultBranch: "main"}}, nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repos", "mine", "--org", "flarebyte", "--json"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, result.ExitCode)
	}
}

func TestRunReposMineRequiresOrg(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repos", "mine"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, result.ExitCode)
	}
}

func TestRunRepoUpdateBlocksDeletionsWithoutConfirmFlag(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldReadRepoMetadata := readRepoMetadata
	t.Cleanup(func() { readRepoMetadata = oldReadRepoMetadata })
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		meta := baseRepoMetadata()
		meta.Topics = append(meta.Topics, "extra-topic")
		meta.Labels = append(meta.Labels, LabelState{Name: "legacy", Color: "CCCCCC", Description: "Legacy"})
		return meta, nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "update"}, &out, &errOut)
	if result.ExitCode != ExitBlockedDeletions {
		t.Fatalf("expected exit code %d, got %d", ExitBlockedDeletions, result.ExitCode)
	}
}

func TestRunRepoUpdateBlocksVisibilityWithoutAcceptFlag(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldReadRepoMetadata := readRepoMetadata
	t.Cleanup(func() { readRepoMetadata = oldReadRepoMetadata })
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		meta := baseRepoMetadata()
		meta.Visibility = "private"
		return meta, nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "update"}, &out, &errOut)
	if result.ExitCode != ExitBlockedVisibility {
		t.Fatalf("expected exit code %d, got %d", ExitBlockedVisibility, result.ExitCode)
	}
}

func TestRunRepoUpdateSuccessSummary(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldReadRepoMetadata := readRepoMetadata
	oldApplyRepoSettings := applyRepoSettings
	oldAddRepoTopic := addRepoTopic
	oldRemoveRepoTopic := removeRepoTopic
	oldCreateRepoLabel := createRepoLabel
	oldUpdateRepoLabel := updateRepoLabel
	oldDeleteRepoLabel := deleteRepoLabel
	t.Cleanup(func() {
		readRepoMetadata = oldReadRepoMetadata
		applyRepoSettings = oldApplyRepoSettings
		addRepoTopic = oldAddRepoTopic
		removeRepoTopic = oldRemoveRepoTopic
		createRepoLabel = oldCreateRepoLabel
		updateRepoLabel = oldUpdateRepoLabel
		deleteRepoLabel = oldDeleteRepoLabel
	})
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		meta := baseRepoMetadata()
		meta.Description = "Old description"
		meta.Topics = []string{"gh-extension"}
		meta.Labels = []LabelState{{Name: "bug", Color: "AAAAAA", Description: "Old"}}
		return meta, nil
	}
	applyRepoSettings = func(repo string, desired RepoSettingsPatch) error { return nil }
	addRepoTopic = func(repo string, topic string) error { return nil }
	removeRepoTopic = func(repo string, topic string) error { return nil }
	createRepoLabel = func(repo string, label LabelState) error { return nil }
	updateRepoLabel = func(repo string, label LabelState) error { return nil }
	deleteRepoLabel = func(repo string, labelName string) error { return nil }
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "update", "--confirm-deletions"}, &out, &errOut)
	if result.ExitCode != ExitOK || !strings.Contains(out.String(), "Update complete:") {
		t.Fatalf("unexpected result: code=%d out=%s err=%s", result.ExitCode, out.String(), errOut.String())
	}
}

func TestRunRepoUpdatePartialFailureMessage(t *testing.T) {
	_ = setupTempWorkdirWithConfig(t, testConfigCue())
	oldReadRepoMetadata := readRepoMetadata
	oldApplyRepoSettings := applyRepoSettings
	oldAddRepoTopic := addRepoTopic
	t.Cleanup(func() {
		readRepoMetadata = oldReadRepoMetadata
		applyRepoSettings = oldApplyRepoSettings
		addRepoTopic = oldAddRepoTopic
	})
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		meta := baseRepoMetadata()
		meta.Description = "Old description"
		meta.Topics = []string{"gh-extension"}
		return meta, nil
	}
	applyRepoSettings = func(repo string, desired RepoSettingsPatch) error { return nil }
	addRepoTopic = func(repo string, topic string) error { return errors.New("topic write failed") }
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "update", "--confirm-deletions"}, &out, &errOut)
	if result.ExitCode != ExitFailure || !strings.Contains(errOut.String(), "no rollback was attempted") {
		t.Fatalf("unexpected result: code=%d err=%s", result.ExitCode, errOut.String())
	}
}
