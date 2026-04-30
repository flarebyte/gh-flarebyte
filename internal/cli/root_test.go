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
