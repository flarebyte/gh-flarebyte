package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

func stubDependencyAssessment(t *testing.T, decision string) {
	t.Helper()
	old := assessDependency
	t.Cleanup(func() { assessDependency = old })
	assessDependency = func(_ context.Context, root, system, name string, v *string, p dependencies.Policy, now time.Time) dependencies.Report {
		r := dependencies.NewReport(system, name, v, p, now)
		r.Decision = decision
		if decision == dependencies.Allow {
			r.Scope = "project-resolution"
		}
		r.Description = dependencies.Description("Example library")
		if decision == dependencies.Deny {
			r.Reasons = []string{"KNOWN_VULNERABILITY"}
		} else {
			r.Reasons = []string{"INSUFFICIENT_EVIDENCE"}
		}
		if v != nil && *v == "123" {
			r.Error = &dependencies.ReportError{Code: "VERSION_NOT_FOUND", Message: "No exact version"}
		}
		return r
	}
}

func localDependencyRepo(t *testing.T, cfg string) string {
	t.Helper()
	root := setupTempWorkdirWithConfig(t, cfg)
	if err := os.Mkdir(".git", 0755); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestDepCommands(t *testing.T) {
	for _, tt := range []struct {
		action, decision string
		code             int
	}{
		{"check", dependencies.Unknown, 8}, {"check", dependencies.Deny, 1}, {"check", dependencies.Allow, 0},
		{"install", dependencies.Unknown, 9}, {"install", dependencies.Allow, 9}, {"install", dependencies.Deny, 1},
	} {
		t.Run(tt.action+tt.decision, func(t *testing.T) {
			root := localDependencyRepo(t, testConfigCue()+"\ndependencies: { minAgeDays: 30 }\n")
			if err := os.WriteFile("package.json", []byte(`{"name":"local","private":true}`), 0644); err != nil {
				t.Fatal(err)
			}
			stubDependencyAssessment(t, tt.decision)
			var stdout, stderr bytes.Buffer
			r := Run([]string{"dep", tt.action, "npm", "pkg", "--json", "--explain"}, &stdout, &stderr)
			if r.ExitCode != tt.code || stderr.Len() != 0 {
				t.Fatalf("%+v stderr %s stdout %s", r, stderr.String(), stdout.String())
			}
			var report dependencies.Report
			if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if report.Details == nil || report.Details.Policy.MinAgeDays != 30 || report.Details.ConfigPath != filepath.Join(root, ".gh-flarebyte.cue") {
				t.Fatal(report.Details)
			}
			b, err := os.ReadFile("package.json")
			if err != nil || string(b) != `{"name":"local","private":true}` {
				t.Fatal("project changed")
			}
			if tt.action == "install" && tt.code == 9 && (report.Decision != "unknown" || report.Error.Code != "INSTALL_UNSUPPORTED") {
				t.Fatal(report)
			}
		})
	}
}

func TestDepOutputAndInput(t *testing.T) {
	localDependencyRepo(t, "")
	stubDependencyAssessment(t, dependencies.Unknown)
	for _, tt := range []struct {
		args []string
		code int
		json bool
	}{
		{[]string{"dep", "check", "npm", "pkg@1.2.3", "--json"}, 8, true},
		{[]string{"dep", "check", "npm", "pkg@1.2.3"}, 8, false},
		{[]string{"dep", "check", "npm", "pkg", "--explain"}, 8, false},
		{[]string{"dep", "check", "pub", "archive", "--json"}, 9, true},
		{[]string{"dep", "check", "npm", "pkg@123", "--json"}, 2, true},
		{[]string{"dep", "install", "npm", "pkg@1.2.3", "--json"}, 2, true},
		{[]string{"dep", "check", "npm", "pkg@^1", "--json"}, 2, true},
		{[]string{"dep", "check", "bad", "pkg", "--json"}, 2, true},
		{[]string{"dep", "check", "npm", "pkg@^1"}, 2, false},
		{[]string{"dep", "check", "npm", "pkg", "--min-age-days", "0"}, 2, false},
		{[]string{"dep", "check", "npm"}, 2, false},
		{[]string{"dep", "check", "npm", "--json"}, 2, true},
		{[]string{"dep", "check", "npm", "pkg", "--unknown", "--json"}, 2, true},
		{[]string{"dep", "install", "npm", "pkg", "--json=true", "--unknown"}, 2, true},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			r := Run(tt.args, &out, &errOut)
			if r.ExitCode != tt.code {
				t.Fatalf("%+v %s %s", r, out.String(), errOut.String())
			}
			if tt.json {
				var report map[string]any
				if err := json.Unmarshal(out.Bytes(), &report); err != nil {
					t.Fatalf("%s %v", out.String(), err)
				}
				if report["details"] != nil {
					t.Fatal("compact output includes details")
				}
			}
		})
	}
}

func TestDepErrorOutputIsBounded(t *testing.T) {
	var out, stderr bytes.Buffer
	r := dependencyError(strings.Repeat("x", 513), "INVALID_INPUT", errors.New(strings.Repeat("x", 300)+"\x1b\n"), 2, true, &out, &stderr)
	var report dependencies.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 2 || report.Package != "" || len([]rune(report.Error.Message)) > 200 {
		t.Fatal(report)
	}
	dependencyError("", "INVALID_INPUT", errors.New("bad\x1b\ninput"), 2, false, io.Discard, &stderr)
	if strings.ContainsRune(stderr.String(), '\x1b') {
		t.Fatal("terminal escape leaked")
	}
	if dependencyJSONRequested([]string{"--json", "--json=false"}) || dependencyJSONRequested([]string{"--", "--json"}) {
		t.Fatal("JSON flag interpretation")
	}
}

func TestDepPolicyAndLocalContext(t *testing.T) {
	root := localDependencyRepo(t, "")
	stubDependencyAssessment(t, dependencies.Unknown)
	if err := os.Mkdir("nested", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("nested"); err != nil {
		t.Fatal(err)
	}
	got, err := dependencyRoot()
	if err != nil || got != root {
		t.Fatalf("%q %v", got, err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()+"\ndependencies: { minAgeDays: -1 }"), 0644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	r := Run([]string{"dep", "check", "npm", "pkg", "--json"}, &out, &errOut)
	if r.ExitCode != 2 || !strings.Contains(out.String(), "INVALID_CONFIG") {
		t.Fatalf("%+v %s", r, out.String())
	}
	outside := t.TempDir()
	if err := os.Chdir(outside); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	r = Run([]string{"dep", "check", "npm", "pkg", "--json"}, &out, &errOut)
	if r.ExitCode != 2 || !strings.Contains(out.String(), "LOCAL_REPOSITORY_REQUIRED") {
		t.Fatalf("%+v %s", r, out.String())
	}
}

type brokenDependencyWriter struct{}

func (brokenDependencyWriter) Write([]byte) (int, error) { return 0, errors.New("closed output") }

func TestDepWriteFailures(t *testing.T) {
	localDependencyRepo(t, "")
	stubDependencyAssessment(t, dependencies.Unknown)
	for _, jsonOutput := range []bool{true, false} {
		r := runDependency(context.Background(), "check", "npm", "pkg", jsonOutput, true, brokenDependencyWriter{}, io.Discard)
		if r.ExitCode != 10 {
			t.Fatal(r)
		}
	}
	r := dependencyError("pkg", "INVALID", errors.New("invalid"), 2, true, brokenDependencyWriter{}, io.Discard)
	if r.ExitCode != 10 {
		t.Fatal(r)
	}
	rpt := dependencies.NewReport("npm", "pkg", nil, dependencies.DefaultPolicy(14), time.Now())
	v := "1.0.0"
	rpt.Requested = &v
	rpt.Description = dependencies.Description("description")
	rpt.Error = &dependencies.ReportError{Code: "x", Message: "blocked"}
	if err := writeDependencyReport(io.Discard, rpt, false); err != nil {
		t.Fatal(err)
	}
}

func TestDepNeverApprovesPackageOnlyEvidence(t *testing.T) {
	localDependencyRepo(t, "")
	stubDependencyAssessment(t, dependencies.Allow)
	prior := assessDependency
	assessDependency = func(ctx context.Context, root, s, n string, v *string, p dependencies.Policy, now time.Time) dependencies.Report {
		r := prior(ctx, root, s, n, v, p, now)
		r.Scope = "package"
		return r
	}
	var out bytes.Buffer
	r := runDependency(context.Background(), "check", "npm", "pkg", true, false, &out, io.Discard)
	if r.ExitCode != ExitDependencyUnknown || !strings.Contains(out.String(), "PROJECT_RESOLUTION") {
		t.Fatalf("%+v %s", r, out.String())
	}
}
