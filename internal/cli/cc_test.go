package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCCReport(t *testing.T) {
	var duplicates []ccDuplicate
	for i := 1; i <= 13; i++ {
		duplicates = append(duplicates, ccDuplicate{Lines: i, First: ccLocation{Name: "z.go", Start: 1, End: 20}, Second: ccLocation{Name: "a.go", Start: 2, End: 21}})
	}
	data, _ := json.Marshal(map[string]any{"duplicates": duplicates})
	pairs, err := parseCCReport(data)
	if err != nil {
		t.Fatal(err)
	}
	if pairs[0].Lines != 13 || pairs[0].First.Name != "a.go" {
		t.Fatalf("unexpected ordering: %+v", pairs[0])
	}
	var out bytes.Buffer
	printCCReport(&out, pairs)
	if strings.Count(out.String(), " ~ ") != 10 || !strings.Contains(out.String(), "13 duplicate pairs; showing 10, 3 more") {
		t.Fatal(out.String())
	}
	for _, bad := range []string{`{}`, `null`, `{"duplicates":null}`, `{"duplicates":[{}]}`, `bad`} {
		if _, err := parseCCReport([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestCCCommand(t *testing.T) {
	original := runCommandCapture
	t.Cleanup(func() { runCommandCapture = original })
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()+"\ncc: {minLines: 12}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, version, report string
		broken, missing       bool
		code                  int
	}{
		{name: "v4 clean", version: "4.0.5", report: `{"duplicates":[]}`, code: ExitOK},
		{name: "v5 duplicates", version: "jscpd 5.4.3", report: `{"duplicates":[{"lines":12,"firstFile":{"name":"src/a.go","start":1,"end":12},"secondFile":{"name":"tests/a.go","start":2,"end":13}}]}`, code: ExitFailure},
		{name: "missing executable", missing: true, code: ExitFailure},
		{name: "unsupported version", version: "3.0.0", code: ExitFailure},
		{name: "scan failure", version: "5.4.3", broken: true, code: ExitFailure},
		{name: "bad report", version: "5.4.3", report: `{}`, code: ExitFailure},
		{name: "missing report", version: "5.4.3", code: ExitFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tempDir string
			runCommandCapture = func(name string, args, env []string) (string, string, error) {
				if name != "jscpd" {
					t.Fatalf("unexpected executable %s", name)
				}
				if len(args) == 1 && args[0] == "--version" {
					if tc.missing {
						return "", "", fmt.Errorf("executable not found")
					}
					return tc.version, "", nil
				}
				if len(args) != 3 || args[0] != "--config" || args[2] != "." {
					t.Fatalf("arguments %v", args)
				}
				tempDir = filepath.Dir(args[1])
				data, err := os.ReadFile(args[1])
				if err != nil {
					t.Fatal(err)
				}
				var policy struct {
					MinLines  int
					Reporters []string
					Ignore    []string
					Gitignore bool
				}
				if err := json.Unmarshal(data, &policy); err != nil {
					t.Fatal(err)
				}
				if policy.MinLines != 12 || !policy.Gitignore || len(policy.Reporters) != 1 || policy.Reporters[0] != "json" || len(policy.Ignore) < 20 {
					t.Fatalf("policy %s", data)
				}
				if tc.broken {
					return "noisy stdout", strings.Repeat("error", 1000), fmt.Errorf("failed")
				}
				if tc.report != "" {
					if err := os.WriteFile(filepath.Join(tempDir, "jscpd-report.json"), []byte(tc.report), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				return "noisy stdout", "", nil
			}
			var out, stderr bytes.Buffer
			result := runCC(&out, &stderr)
			if result.ExitCode != tc.code {
				t.Fatalf("result %+v stderr %s", result, stderr.String())
			}
			if strings.Contains(out.String(), "noisy") || stderr.Len() > 2500 {
				t.Fatalf("unbounded output: %s", out.String())
			}
			if tc.missing && (!strings.Contains(stderr.String(), "brew install jscpd") || !strings.Contains(stderr.String(), "github.com/kucherenko/jscpd")) {
				t.Fatal(stderr.String())
			}
			if tempDir != "" {
				if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
					t.Fatalf("temporary report not cleaned: %v", err)
				}
			}
		})
	}
}

// Exercise PATH lookup, the execution allowlist and Cobra without installing a tool.
func TestCCInstalledExecutable(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
 printf '5.4.3\n'
 exit 0
fi
[ "$1" = "--config" ] && [ "$3" = "." ] || exit 9
printf '{"duplicates":[]}' > "${2%/*}/jscpd-report.json"
printf 'tool chatter that should be hidden\n'
`
	if err := os.WriteFile(filepath.Join(root, "jscpd"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	var out, stderr bytes.Buffer
	result := Run([]string{"cc"}, &out, &stderr)
	if result.ExitCode != ExitOK || out.String() != "cc: 0 duplicate pairs; showing 0, 0 more.\n" || stderr.Len() != 0 {
		t.Fatalf("result=%+v stdout=%s stderr=%s", result, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if result := Run([]string{"cc", "extra"}, &out, &stderr); result.ExitCode != ExitUsage {
		t.Fatalf("accepted extra argument: %+v", result)
	}
	out.Reset()
	stderr.Reset()
	if result := Run([]string{"cc", "--help"}, &out, &stderr); result.ExitCode != ExitOK || !strings.Contains(out.String(), "jscpd") {
		t.Fatalf("help=%s result=%+v", out.String(), result)
	}
}
