package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

func stubNPMExecutable(t *testing.T, script string) {
	t.Helper()
	stubDependencyExecutable(t, "npm", script)
}

func stubDependencyExecutable(t *testing.T, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
}

func stubPackageAdviceAPI(t *testing.T, vulnerable bool) {
	t.Helper()
	old := assessDependency
	t.Cleanup(func() { assessDependency = old })
	assessDependency = func(ctx context.Context, _ string, system, name string, v *string, p dependencies.Policy, now time.Time) dependencies.Report {
		selected := "1.2.3"
		if system == "go" {
			selected = "v1.2.3"
		}
		client := &dependencies.Client{HTTP: cargoInstallHTTP(func(req *http.Request) (*http.Response, error) {
			var body any
			switch {
			case req.URL.Host == "api.osv.dev":
				body = map[string]any{}
				if vulnerable {
					body = map[string]any{"vulns": []any{map[string]string{"id": "GHSA-fixture"}}}
				}
			case req.URL.Host == "api.scorecard.dev":
				return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
			case req.URL.Host == "api.deps.dev" && strings.Contains(req.URL.Path, "/projects/"):
				body = map[string]any{"projectKey": map[string]string{"id": "github.com/org/repo"}, "starsCount": 1000}
			case req.URL.Host == "api.deps.dev" && strings.Contains(req.URL.Path, "/versions/"):
				body = dependencies.Version{Key: dependencies.VersionKey{System: strings.ToUpper(system), Name: name, Version: selected}, PublishedAt: now.AddDate(-1, 0, 0), Attestations: []dependencies.Attestation{{Type: "https://slsa.dev/provenance/v1", Verified: true, SourceRepository: "https://github.com/org/repo", Commit: strings.Repeat("a", 40)}}}
			case req.URL.Host == "api.deps.dev":
				body = map[string]any{"packageKey": map[string]string{"name": name}, "versions": []any{map[string]any{"versionKey": map[string]string{"version": selected}}}}
			case req.URL.Host == "registry.npmjs.org" && req.URL.Path == "/pkg":
				body = map[string]any{"name": name, "maintainers": []any{map[string]string{"name": "one"}, map[string]string{"name": "two"}}}
			case req.URL.Host == "registry.npmjs.org":
				body = map[string]any{"name": name, "version": selected, "description": "Fixture", "dependencies": map[string]string{"child": "^1"}}
			case req.URL.Host == "pypi.org" && req.URL.Path == "/pypi/"+name+"/json":
				body = map[string]any{"info": map[string]string{"name": name}, "ownership": map[string]any{"roles": []any{map[string]string{"role": "Owner", "user": "one"}, map[string]string{"role": "Maintainer", "user": "two"}}, "organization": nil}}
			case req.URL.Host == "pypi.org" && req.URL.Path == "/pypi/"+name+"/"+selected+"/json":
				body = map[string]any{"info": map[string]string{"name": name, "version": selected, "summary": "Fixture", "requires_python": ">=999"}, "urls": []any{map[string]any{"filename": "fixture.whl", "yanked": false}}}
			default:
				t.Fatal("unexpected request", req.URL)
			}
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: http.Header{}}, nil
		})}
		return client.AssessPackage(ctx, system, name, v, p, now)
	}
}

func TestDepNPMAdviceAndDelegation(t *testing.T) {
	testPackageAdviceAndDelegation(t, "npm", "pkg", "1.2.3")
}

func testPackageAdviceAndDelegation(t *testing.T, system, name, version string) {
	for _, mode := range []string{"check", "install", "exact", "blocked", "native failure", "flags"} {
		t.Run(mode, func(t *testing.T) {
			root := localDependencyRepo(t, "")
			sub := filepath.Join(root, "app")
			if err := os.Mkdir(sub, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(sub); err != nil {
				t.Fatal(err)
			}
			exit := "0"
			if mode == "native failure" {
				exit = "37"
			}
			executable := system
			if system == "pypi" {
				executable = "uv"
			}
			stubDependencyExecutable(t, executable, "printf '%s\\n' \"$@\" > native-args\nprintf '%s' \"$PWD\" > native-dir\nprintf 'native stdout\\n'\nprintf 'native stderr\\n' >&2\nexit "+exit+"\n")
			stubPackageAdviceAPI(t, mode == "blocked")
			args := []string{"dep", "install", system, name}
			if mode == "check" {
				args = []string{"dep", "check", system, name, "--json"}
			}
			if mode == "exact" {
				args[3] = name + "@" + version
			}
			if mode == "flags" {
				args = append(args, "--json")
			}
			var out, stderr bytes.Buffer
			r := Run(args, &out, &stderr)
			switch mode {
			case "check":
				var report dependencies.Report
				if json.Unmarshal(out.Bytes(), &report) != nil || r.ExitCode != 0 || report.Scope != "package" || report.Coverage != "direct-package-only" || report.Recommendation == nil {
					t.Fatal(r, out.String(), stderr.String())
				}
			case "blocked", "flags":
				if r.ExitCode == 0 || out.Len() != 0 || stderr.Len() == 0 {
					t.Fatal(r, out.String(), stderr.String())
				}
			default:
				want := 0
				if mode == "native failure" {
					want = 37
				}
				if r.ExitCode != want || out.String() != "native stdout\nnative stderr\n" || stderr.Len() != 0 {
					t.Fatal(r, out.String(), stderr.String())
				}
				expectedArgs := "install\n--save-exact\n--\n" + name + "@" + version + "\n"
				if system == "go" {
					expectedArgs = "get\n" + name + "@" + version + "\n"
				}
				if system == "pypi" {
					expectedArgs = "add\n" + name + "==" + version + "\n"
				}
				b, err := os.ReadFile("native-args")
				if err != nil || string(b) != expectedArgs {
					t.Fatal(string(b), err)
				}
				b, err = os.ReadFile("native-dir")
				if err != nil || string(b) != sub {
					t.Fatal(string(b), err)
				}
			}
			if mode == "check" || mode == "blocked" || mode == "flags" {
				if _, err := os.Stat("native-args"); !os.IsNotExist(err) {
					t.Fatal("native launched unexpectedly")
				}
			}
		})
	}
}

func TestDepNPMNativeInputAndFailures(t *testing.T) {
	stubNPMExecutable(t, "IFS= read -r line\nprintf '%s:%s' \"$line\" \"$npm_config_registry\"\n")
	t.Setenv("npm_config_registry", "https://example.test/registry")
	var out, stderr bytes.Buffer
	r := runDelegatedNPM(context.Background(), t.TempDir(), "pkg", "1.2.3", strings.NewReader("hello\n"), &out, &stderr)
	if r.ExitCode != 0 || out.String() != "hello:https://example.test/registry" {
		t.Fatal(r, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	r = runDelegatedNPM(context.Background(), t.TempDir(), "pkg;bad", "1.2.3", nil, &out, &stderr)
	if r.ExitCode != ExitUsage {
		t.Fatal(r)
	}
	t.Setenv("PATH", t.TempDir())
	r = runDelegatedNPM(context.Background(), t.TempDir(), "pkg", "1.2.3", nil, &out, &stderr)
	if r.ExitCode != ExitDependencyFailure || out.Len() != 0 {
		t.Fatal(r, out.String())
	}
}

func TestDepNPMNativeCancellation(t *testing.T) {
	stubNPMExecutable(t, "exec sleep 10\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r := runDelegatedNPM(ctx, t.TempDir(), "pkg", "1.2.3", nil, io.Discard, io.Discard)
	if r.ExitCode != 130 {
		t.Fatal(r)
	}
}

type npmProgressWriter struct {
	buffer bytes.Buffer
	ready  chan struct{}
	once   sync.Once
}

func (w *npmProgressWriter) Write(b []byte) (int, error) {
	n, err := w.buffer.Write(b)
	if strings.Contains(w.buffer.String(), "progress\n") {
		w.once.Do(func() { close(w.ready) })
	}
	return n, err
}

func TestDepNPMStreamsBeforeExit(t *testing.T) {
	stubNPMExecutable(t, "printf 'progress\\n'\nIFS= read -r line\nprintf 'done:%s\\n' \"$line\"\n")
	input, feed := io.Pipe()
	defer input.Close()
	defer feed.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	w := &npmProgressWriter{ready: make(chan struct{})}
	done := make(chan Result, 1)
	dir := t.TempDir()
	go func() { done <- runDelegatedNPM(ctx, dir, "pkg", "1.2.3", input, w, io.Discard) }()
	select {
	case <-w.ready:
	case <-ctx.Done():
		t.Fatal("native output was buffered until exit")
	}
	if _, err := io.WriteString(feed, "hello\n"); err != nil {
		t.Fatal(err)
	}
	feed.Close()
	r := <-done
	if r.ExitCode != 0 || w.buffer.String() != "progress\ndone:hello\n" {
		t.Fatal(r, w.buffer.String())
	}
}
