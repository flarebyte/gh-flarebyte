package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

// Real public metadata must not promote a self-declared source link to approval.
func TestDepPubAdviceBlocksUnverifiedInstall(t *testing.T) {
	for _, action := range []string{"check", "install"} {
		t.Run(action, func(t *testing.T) {
			localDependencyRepo(t, "")
			t.Setenv("PATH", t.TempDir())
			old := assessDependency
			t.Cleanup(func() { assessDependency = old })
			assessDependency = func(ctx context.Context, _ string, system, name string, v *string, p dependencies.Policy, now time.Time) dependencies.Report {
				client := &dependencies.Client{HTTP: cargoInstallHTTP(func(req *http.Request) (*http.Response, error) {
					var body any
					switch req.URL.Host {
					case "pub.dev":
						body = map[string]any{"publisherId": "example.org"}
						if !strings.HasSuffix(req.URL.Path, "/publisher") {
							body = map[string]any{"name": name, "versions": []any{map[string]any{"version": "1.2.3", "published": now.AddDate(-1, 0, 0), "pubspec": map[string]string{"name": name, "version": "1.2.3", "description": "Dart fixture", "repository": "https://github.com/org/repo"}}}}
						}
					case "api.osv.dev":
						body = map[string]any{}
					case "api.github.com":
						body = map[string]any{"full_name": "org/repo", "stargazers_count": 1000}
					case "api.scorecard.dev":
						return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
					default:
						t.Fatal(req.URL)
					}
					b, _ := json.Marshal(body)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: http.Header{}}, nil
				})}
				return client.AssessPackage(ctx, system, name, v, p, now)
			}
			args := []string{"dep", action, "pub", "pkg"}
			if action == "check" {
				args = append(args, "--json", "--explain")
			}
			var out, stderr bytes.Buffer
			r := Run(args, &out, &stderr)
			if r.ExitCode != ExitDependencyUnknown {
				t.Fatal(r, out.String(), stderr.String())
			}
			report := out.String() + stderr.String()
			if !strings.Contains(report, "PACKAGE_IDENTITY") || !strings.Contains(report, "1.2.3") {
				t.Fatal(report)
			}
			if action == "install" && out.Len() != 0 {
				t.Fatal(out.String())
			}
		})
	}
}

// Inject approval solely to exercise command routing; pub.dev itself supplies no verified source binding.
func TestDepPubApprovedDelegation(t *testing.T) {
	for _, mode := range []string{"install", "exact", "native failure", "flags"} {
		t.Run(mode, func(t *testing.T) {
			localDependencyRepo(t, "")
			nativeExit := "0"
			if mode == "native failure" {
				nativeExit = "37"
			}
			stubDependencyExecutable(t, "dart", "printf '%s\\n' \"$@\" > native-args\nprintf 'native stdout\\n'\nprintf 'native stderr\\n' >&2\nexit "+nativeExit+"\n")
			old := assessDependency
			t.Cleanup(func() { assessDependency = old })
			assessDependency = func(_ context.Context, _ string, s, n string, v *string, p dependencies.Policy, now time.Time) dependencies.Report {
				r := dependencies.NewReport(s, n, v, p, now)
				r.Decision = dependencies.Allow
				r.Recommendation = &dependencies.Candidate{Version: "1.2.3"}
				return r
			}
			args := []string{"dep", "install", "pub", "pkg"}
			if mode == "exact" {
				args[3] += "@1.2.3"
			}
			if mode == "flags" {
				args = append(args, "--json")
			}
			var out, stderr bytes.Buffer
			r := Run(args, &out, &stderr)
			if mode == "flags" {
				if r.ExitCode != ExitUsage || out.Len() != 0 {
					t.Fatal(r, out.String())
				}
				return
			}
			want := 0
			if mode == "native failure" {
				want = 37
			}
			if r.ExitCode != want || out.String() != "native stdout\nnative stderr\n" {
				t.Fatal(r, out.String(), stderr.String())
			}
			data, err := os.ReadFile("native-args")
			if err != nil || string(data) != "pub\nadd\npkg:1.2.3\n" {
				t.Fatal(string(data), err)
			}
		})
	}
}

func TestDepPubNativeInputAndFailures(t *testing.T) {
	stubDependencyExecutable(t, "dart", "IFS= read -r line\nprintf '%s:%s:%s:%s' \"$line\" \"$PUB_CACHE\" \"$PUB_HOSTED_URL\" \"$3\"\n")
	t.Setenv("PUB_CACHE", "/example/pub-cache")
	t.Setenv("PUB_HOSTED_URL", "https://example.test/pub")
	var out, stderr bytes.Buffer
	r := runDelegatedPub(context.Background(), t.TempDir(), "pkg", "1.2.3", strings.NewReader("hello\n"), &out, &stderr)
	if r.ExitCode != 0 || out.String() != "hello:/example/pub-cache:https://example.test/pub:pkg:1.2.3" {
		t.Fatal(r, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	r = runDelegatedPub(context.Background(), t.TempDir(), "pkg;bad", "1.2.3", nil, &out, &stderr)
	if r.ExitCode != ExitUsage || out.Len() != 0 {
		t.Fatal(r)
	}
	t.Setenv("PATH", t.TempDir())
	r = runDelegatedPub(context.Background(), t.TempDir(), "pkg", "1.2.3", nil, &out, &stderr)
	if r.ExitCode != ExitDependencyFailure || out.Len() != 0 {
		t.Fatal(r)
	}
}
