package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

)

func TestRunHelp(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	result := Run([]string{"--help"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, result.ExitCode)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("expected help output to contain Usage, got: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
	}
}

func TestRunVersionPlainText(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	result := Run([]string{"--version"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, result.ExitCode)
	}
	text := out.String()
	if !strings.Contains(text, "gh-flarebyte ") {
		t.Fatalf("expected version output to start with gh-flarebyte, got: %s", text)
	}
	if !strings.Contains(text, "commitId=") || !strings.Contains(text, "date=") {
		t.Fatalf("expected version output to include commitId/date, got: %s", text)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
	}
}

func TestRunVersionJSON(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	result := Run([]string{"--version", "--json"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, result.ExitCode)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	required := []string{"version", "commitId", "date", "os", "arch", "goVersion"}
	for _, key := range required {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected key %q in payload", key)
		}
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
	}
}

func TestRunJSONWithoutVersionIsUsageError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	result := Run([]string{"--json"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "--json must be used with --version") {
		t.Fatalf("expected usage guidance error, got: %s", errOut.String())
	}
}

func TestRunConfigValidateValid(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	result := Run([]string{"config", "validate", "--config", "../config/testdata/valid.cue"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d, err=%v", ExitOK, result.ExitCode, result.Err)
	}
	if !strings.Contains(out.String(), "config is valid:") {
		t.Fatalf("expected success output, got: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
	}
}

func TestRunConfigValidateInvalid(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	result := Run([]string{"config", "validate", "--config", "../config/testdata/invalid-target.cue"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "invalid build.targets entry") {
		t.Fatalf("expected target validation error, got: %s", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty stdout, got: %s", out.String())
	}
}

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
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	oldReadRepoMetadata := readRepoMetadata
	oldFileExists := fileExists
	oldWriteFile := writeFile
	t.Cleanup(func() {
		readRepoMetadata = oldReadRepoMetadata
		fileExists = oldFileExists
		writeFile = oldWriteFile
	})

	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		return RepoMetadata{}, errors.New("gh not authenticated")
	}
	fileExists = func(path string) bool { return false }
	writeFile = func(path string, data []byte) error {
		return os.WriteFile(path, data, 0o644)
	}

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
	if !strings.Contains(text, "org:  \"flarebyte\"") {
		t.Fatalf("expected owner in config, got: %s", text)
	}
	if !strings.Contains(text, "repo: \"gh-flarebyte\"") {
		t.Fatalf("expected repo in config, got: %s", text)
	}
	if !strings.Contains(text, "homepage:      \"https://github.com/flarebyte/gh-flarebyte\"") {
		t.Fatalf("expected default homepage, got: %s", text)
	}
}

func TestRunRepoInitOverwriteWritesImportedMetadata(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	oldReadRepoMetadata := readRepoMetadata
	oldFileExists := fileExists
	oldWriteFile := writeFile
	t.Cleanup(func() {
		readRepoMetadata = oldReadRepoMetadata
		fileExists = oldFileExists
		writeFile = oldWriteFile
	})

	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		return RepoMetadata{
			Description:   "Imported description",
			DefaultBranch: "trunk",
			Homepage:      "https://example.test/repo",
			Visibility:    "private",
			Template:      true,
		}, nil
	}
	fileExists = func(path string) bool { return true }
	writeFile = func(path string, data []byte) error {
		return os.WriteFile(path, data, 0o644)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer

	result := Run([]string{"repo", "init", "--repo", "flarebyte/gh-flarebyte", "--overwrite"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d, err=%v", ExitOK, result.ExitCode, result.Err)
	}
	if !strings.Contains(out.String(), "from current GitHub state") {
		t.Fatalf("expected imported success message, got: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gh-flarebyte.cue"))
	if err != nil {
		t.Fatalf("expected config file, read error: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "description:   \"Imported description\"") {
		t.Fatalf("expected imported description, got: %s", text)
	}
	if !strings.Contains(text, "defaultBranch: \"trunk\"") {
		t.Fatalf("expected imported branch, got: %s", text)
	}
	if !strings.Contains(text, "visibility:    \"private\"") {
		t.Fatalf("expected imported visibility, got: %s", text)
	}
	if !strings.Contains(text, "template:      true") {
		t.Fatalf("expected imported template flag, got: %s", text)
	}
}

func TestRunRepoAuditJSONNoDrift(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldReadRepoMetadata := readRepoMetadata
	t.Cleanup(func() {
		readRepoMetadata = oldReadRepoMetadata
	})
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		return RepoMetadata{
			Description:   "CLI for landing your git commands right",
			DefaultBranch: "main",
			Homepage:      "https://github.com/flarebyte/gh-flarebyte",
			Visibility:    "public",
			Template:      false,
			Topics:        []string{"gh-extension", "github-cli", "git", "flarebyte"},
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "audit", "--json"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, result.ExitCode)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got %s", errOut.String())
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
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldReadRepoMetadata := readRepoMetadata
	t.Cleanup(func() {
		readRepoMetadata = oldReadRepoMetadata
	})
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		return RepoMetadata{
			Description:   "Different description",
			DefaultBranch: "main",
			Homepage:      "https://github.com/flarebyte/gh-flarebyte",
			Visibility:    "public",
			Template:      false,
			Topics:        []string{"gh-extension", "github-cli"},
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "audit"}, &out, &errOut)
	if result.ExitCode != ExitDrift {
		t.Fatalf("expected exit code %d, got %d", ExitDrift, result.ExitCode)
	}
	if !strings.Contains(out.String(), "differences found") {
		t.Fatalf("expected drift summary output, got %s", out.String())
	}
	if !strings.Contains(out.String(), "repository.description") {
		t.Fatalf("expected field-level drift output, got %s", out.String())
	}
}

func TestRunReposMineJSON(t *testing.T) {
	oldReadReposMine := readReposMine
	t.Cleanup(func() {
		readReposMine = oldReadReposMine
	})
	readReposMine = func(org string) (string, []ContributedRepo, error) {
		return "olivier", []ContributedRepo{
			{Owner: "flarebyte", Name: "gh-flarebyte", Visibility: "public", DefaultBranch: "main"},
		}, nil
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repos", "mine", "--org", "flarebyte", "--json"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, result.ExitCode)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got %s", errOut.String())
	}
	var report ReposMineReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if report.Org != "flarebyte" || report.Contributor != "olivier" || report.Count != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestRunReposMineRequiresOrg(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repos", "mine"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "--org is required") {
		t.Fatalf("expected missing org guidance, got: %s", errOut.String())
	}
}

func TestRunRepoUpdateBlocksDeletionsWithoutConfirmFlag(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldReadRepoMetadata := readRepoMetadata
	t.Cleanup(func() { readRepoMetadata = oldReadRepoMetadata })
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		return RepoMetadata{
			Description:   "CLI for landing your git commands right",
			DefaultBranch: "main",
			Homepage:      "https://github.com/flarebyte/gh-flarebyte",
			Visibility:    "public",
			Template:      false,
			Topics:        []string{"gh-extension", "github-cli", "git", "flarebyte", "extra-topic"},
			Labels: []LabelState{
				{Name: "bug", Color: "B60205", Description: "Something is broken"},
				{Name: "enhancement", Color: "0E8A16", Description: "New feature"},
				{Name: "legacy", Color: "CCCCCC", Description: "Legacy"},
			},
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "update"}, &out, &errOut)
	if result.ExitCode != ExitBlockedDeletions {
		t.Fatalf("expected exit code %d, got %d", ExitBlockedDeletions, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "--confirm-deletions") {
		t.Fatalf("expected deletion safety guidance, got: %s", errOut.String())
	}
}

func TestRunRepoUpdateBlocksVisibilityWithoutAcceptFlag(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldReadRepoMetadata := readRepoMetadata
	t.Cleanup(func() { readRepoMetadata = oldReadRepoMetadata })
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		return RepoMetadata{
			Description:   "CLI for landing your git commands right",
			DefaultBranch: "main",
			Homepage:      "https://github.com/flarebyte/gh-flarebyte",
			Visibility:    "private",
			Template:      false,
			Topics:        []string{"gh-extension", "github-cli", "git", "flarebyte"},
			Labels: []LabelState{
				{Name: "bug", Color: "B60205", Description: "Something is broken"},
				{Name: "enhancement", Color: "0E8A16", Description: "New feature"},
			},
		}, nil
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "update"}, &out, &errOut)
	if result.ExitCode != ExitBlockedVisibility {
		t.Fatalf("expected exit code %d, got %d", ExitBlockedVisibility, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "--accept-visibility-change-consequences") {
		t.Fatalf("expected visibility safety guidance, got: %s", errOut.String())
	}
}

func TestRunRepoUpdateSuccessSummary(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

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
		return RepoMetadata{
			Description:   "Old description",
			DefaultBranch: "main",
			Homepage:      "https://github.com/flarebyte/gh-flarebyte",
			Visibility:    "public",
			Template:      false,
			Topics:        []string{"gh-extension"},
			Labels: []LabelState{
				{Name: "bug", Color: "AAAAAA", Description: "Old"},
			},
		}, nil
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
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d, err=%v", ExitOK, result.ExitCode, result.Err)
	}
	if !strings.Contains(out.String(), "Update complete:") {
		t.Fatalf("expected success summary, got: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
	}
}

func TestRunRepoUpdatePartialFailureMessage(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldReadRepoMetadata := readRepoMetadata
	oldApplyRepoSettings := applyRepoSettings
	oldAddRepoTopic := addRepoTopic
	t.Cleanup(func() {
		readRepoMetadata = oldReadRepoMetadata
		applyRepoSettings = oldApplyRepoSettings
		addRepoTopic = oldAddRepoTopic
	})
	readRepoMetadata = func(repo string) (RepoMetadata, error) {
		return RepoMetadata{
			Description:   "Old description",
			DefaultBranch: "main",
			Homepage:      "https://github.com/flarebyte/gh-flarebyte",
			Visibility:    "public",
			Template:      false,
			Topics:        []string{"gh-extension"},
			Labels: []LabelState{
				{Name: "bug", Color: "B60205", Description: "Something is broken"},
				{Name: "enhancement", Color: "0E8A16", Description: "New feature"},
			},
		}, nil
	}
	applyRepoSettings = func(repo string, desired RepoSettingsPatch) error { return nil }
	addRepoTopic = func(repo string, topic string) error { return errors.New("topic write failed") }

	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"repo", "update", "--confirm-deletions"}, &out, &errOut)
	if result.ExitCode != ExitFailure {
		t.Fatalf("expected exit code %d, got %d", ExitFailure, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "no rollback was attempted") {
		t.Fatalf("expected partial-failure guidance, got: %s", errOut.String())
	}
}
func TestRunBuildRejectsUnknownConfiguredTargetFilter(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build", "--target", "darwin-arm64"}, &out, &errOut)
	if result.ExitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, result.ExitCode)
	}
	if !strings.Contains(errOut.String(), "not present in build.targets") {
		t.Fatalf("expected target validation error, got: %s", errOut.String())
	}
}

func TestRunBuildSuccessWritesChecksumAndSummary(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldBuildTargetBinary := buildTargetBinary
	oldPackageBinary := packageBinary
	t.Cleanup(func() {
		buildTargetBinary = oldBuildTargetBinary
		packageBinary = oldPackageBinary
	})
	buildTargetBinary = func(target string, outputPath string) error {
		return os.WriteFile(outputPath, []byte("binary-"+target), 0o755)
	}
	packageBinary = packageBinaryArchive

	var out bytes.Buffer
	var errOut bytes.Buffer
	result := Run([]string{"build", "--output-dir", "dist"}, &out, &errOut)
	if result.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d, err=%v", ExitOK, result.ExitCode, result.Err)
	}
	if !strings.Contains(out.String(), "Build complete: 1 targets written to dist/ with checksums in dist/checksums.txt.") {
		t.Fatalf("expected success summary, got: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
	}
	if _, err := os.Stat(filepath.Join("dist", "gh-flarebyte-linux-amd64.tar.gz")); err != nil {
		t.Fatalf("expected packaged artifact, got: %v", err)
	}
	checksums, err := os.ReadFile(filepath.Join("dist", "checksums.txt"))
	if err != nil {
		t.Fatalf("expected checksum file, got: %v", err)
	}
	if !strings.Contains(string(checksums), "gh-flarebyte-linux-amd64.tar.gz") {
		t.Fatalf("expected checksum entry, got: %s", string(checksums))
	}
}

func TestPackageBinaryArchiveTarGzAndZip(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "gh-flarebyte-linux-amd64")
	if err := os.WriteFile(src, []byte("linux-binary"), 0o755); err != nil {
		t.Fatalf("write src failed: %v", err)
	}
	tarGz := filepath.Join(tmpDir, "artifact.tar.gz")
	if err := packageBinaryArchive(src, "linux-amd64", tarGz); err != nil {
		t.Fatalf("package tar.gz failed: %v", err)
	}
	f, err := os.Open(tarGz)
	if err != nil {
		t.Fatalf("open tar.gz failed: %v", err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader failed: %v", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tar entry missing: %v", err)
	}
	if hdr.Name != "gh-flarebyte-linux-amd64" {
		t.Fatalf("unexpected tar entry name: %s", hdr.Name)
	}
	content, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("read tar content failed: %v", err)
	}
	if string(content) != "linux-binary" {
		t.Fatalf("unexpected tar content: %s", string(content))
	}

	winSrc := filepath.Join(tmpDir, "gh-flarebyte-windows-amd64.exe")
	if err := os.WriteFile(winSrc, []byte("win-binary"), 0o755); err != nil {
		t.Fatalf("write win src failed: %v", err)
	}
	zipPath := filepath.Join(tmpDir, "artifact.zip")
	if err := packageBinaryArchive(winSrc, "windows-amd64", zipPath); err != nil {
		t.Fatalf("package zip failed: %v", err)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip failed: %v", err)
	}
	defer zr.Close()
	if len(zr.File) != 1 {
		t.Fatalf("expected one zip entry, got %d", len(zr.File))
	}
	if zr.File[0].Name != "gh-flarebyte-windows-amd64.exe" {
		t.Fatalf("unexpected zip entry name: %s", zr.File[0].Name)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatalf("open zip entry failed: %v", err)
	}
	defer rc.Close()
	winContent, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read zip content failed: %v", err)
	}
	if string(winContent) != "win-binary" {
		t.Fatalf("unexpected zip content: %s", string(winContent))
	}
}
func testConfigCue() string {
	return `package ghflarebyte

project: {
	org:  "flarebyte"
	repo: "gh-flarebyte"
}

sync: {
	mode: "push"
	managedFields: ["topics"]
}

repository: {
	description:   "CLI for landing your git commands right"
	defaultBranch: "main"
	homepage:      "https://github.com/flarebyte/gh-flarebyte"
	visibility:    "public"
	template:      false
	topics: [
		"gh-extension",
		"github-cli",
		"git",
		"flarebyte",
	]
	labels: [
		{
			name:        "bug"
			color:       "B60205"
			description: "Something is broken"
		},
		{
			name:        "enhancement"
			color:       "0E8A16"
			description: "New feature"
		},
	]
}

build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
	]
}

release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}
`
}
