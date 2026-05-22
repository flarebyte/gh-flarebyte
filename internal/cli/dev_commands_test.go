// purpose: Verify shared developer-command helper behavior.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func TestRunHelpIncludesDevCommands(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	res := Run([]string{"--help"}, &out, &errOut)
	if res.ExitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, res.ExitCode)
	}
	text := out.String()
	for _, cmd := range []string{"gh flarebyte test", "gh flarebyte format", "gh flarebyte lint", "gh flarebyte cov"} {
		if !strings.Contains(text, cmd) {
			t.Fatalf("expected help to include %q, got: %s", cmd, text)
		}
	}
}

func TestDiscoverGoFilesSkipsExcludedDirs(t *testing.T) {
	tmp := t.TempDir()
	mustWrite := func(rel string) {
		t.Helper()
		path := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := os.WriteFile(path, []byte("package x\n"), 0o644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}
	mustWrite("a/main.go")
	mustWrite("vendor/v.go")
	mustWrite("build/b.go")
	mustWrite(".gocache/c.go")
	files, err := discoverGoFiles(tmp)
	if err != nil {
		t.Fatalf("discoverGoFiles failed: %v", err)
	}
	if len(files) != 1 || !strings.Contains(files[0], "a/main.go") {
		t.Fatalf("unexpected files: %v", files)
	}
}

func TestParseGoTestJSONSummaryIgnoresPackageLevelEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"Action":"run","Package":"x/y","Test":"TestA"}`,
		`{"Action":"pass","Package":"x/y","Test":"TestA"}`,
		`{"Action":"pass","Package":"x/y"}`,
		"",
	}, "\n")
	tests, failed, skipped := parseGoTestJSONSummary(input)
	if tests != 1 || failed != 0 || skipped != 0 {
		t.Fatalf("unexpected summary tests=%d failed=%d skipped=%d", tests, failed, skipped)
	}
}

func TestUseColorModes(t *testing.T) {
	oldNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	oldTerm, hadTerm := os.LookupEnv("TERM")
	t.Cleanup(func() {
		if hadNoColor {
			_ = os.Setenv("NO_COLOR", oldNoColor)
		} else {
			_ = os.Unsetenv("NO_COLOR")
		}
		if hadTerm {
			_ = os.Setenv("TERM", oldTerm)
		} else {
			_ = os.Unsetenv("TERM")
		}
	})

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Setenv("TERM", "xterm-256color")
	if !useColor("auto") || !useColor("true") {
		t.Fatalf("expected color enabled for auto/true")
	}
	if useColor("false") {
		t.Fatalf("expected color disabled for false")
	}
	_ = os.Setenv("NO_COLOR", "1")
	if useColor("true") {
		t.Fatalf("expected NO_COLOR to disable color")
	}
}

func TestPrintDevSummaryPerTestStyleUsesSummaryLine(t *testing.T) {
	var b bytes.Buffer
	cfg := config.Config{DevOutput: config.DevOutputConfig{Color: "false", Style: "per_test", ShowPassed: true}}
	printDevSummary(&b, cfg, devSummary{Kind: "lint", Status: "PASS", Duration: 1500 * 1e6, Details: "ok=1"})
	out := b.String()
	if !strings.Contains(out, "LINT PASS duration=") || !strings.Contains(out, "ok=1") {
		t.Fatalf("unexpected per_test summary output: %s", out)
	}
}
