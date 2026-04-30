package cli

import (
	"bytes"
	"encoding/json"
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
