package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

type cargoInstallFixture struct {
	Manifest  string                     `json:"manifest"`
	Lockfile  string                     `json:"lockfile"`
	Responses map[string]json.RawMessage `json:"responses"`
	Artifacts map[string][]byte          `json:"artifacts"`
}
type cargoInstallHTTP func(*http.Request) (*http.Response, error)

func (f cargoInstallHTTP) Do(r *http.Request) (*http.Response, error) { return f(r) }

// Synthetic provider fixtures exercise the real assessment/approval boundary;
// sealed private evidence is never forged or reconstructed from report JSON.
func approvedCargoInstall(t *testing.T, transforms ...func(string) string) (string, dependencies.Report, map[string][]byte) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "dependencies", "testdata", "cargo-install.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f cargoInstallFixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	root := localDependencyRepo(t, "")
	for _, transform := range transforms {
		f.Manifest = transform(f.Manifest)
	}
	for name, data := range map[string]string{"Cargo.toml": f.Manifest, "Cargo.lock": f.Lockfile, "src/lib.rs": "// unchanged source\n"} {
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	c := dependencies.NewClient()
	c.HTTP = cargoInstallHTTP(func(r *http.Request) (*http.Response, error) {
		b, ok := f.Responses[r.URL.String()]
		if !ok {
			t.Fatalf("unexpected provider request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(b))}, nil
	})
	c.GitHubToken = func(context.Context) (string, error) { return "synthetic-test-token", nil }
	checked := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	r := c.AssessLocal(context.Background(), root, "cargo", "pkg", nil, dependencies.DefaultPolicy(14), checked)
	if r.Decision != dependencies.Allow {
		b, _ := json.Marshal(r)
		t.Fatal("synthetic fixture not approved", string(b))
	}
	// Freshness behavior is tested separately from fixed provider fixture dates.
	r.CheckedAt = time.Now().UTC()
	return root, r, f.Artifacts
}

func stubCargoInstall(t *testing.T, root string, blobs map[string][]byte, mode string) {
	t.Helper()
	oldArtifact, oldRunner := dependencyCargoArtifact, dependencyCargoRunner
	t.Cleanup(func() { dependencyCargoArtifact, dependencyCargoRunner = oldArtifact, oldRunner })
	dependencyCargoArtifact = func(_ context.Context, a dependencies.CargoArtifact) ([]byte, error) {
		if mode == "artifact" {
			return []byte("corrupted"), nil
		}
		return blobs[a.Node.Name], nil
	}
	dependencyCargoRunner = func(_ string, stage string, validate func(string, string) error) (func(context.Context) ([]byte, error), error) {
		var err error
		stage, err = filepath.EvalSymlinks(stage)
		if err != nil {
			return nil, err
		}
		return func(context.Context) ([]byte, error) {
			if err := validate("1.99.1", "1.99.1"); err != nil {
				return nil, err
			}
			if mode == "native" {
				return nil, errors.New("native command failed")
			}
			packages, nodes := []any{}, []any{}
			for _, name := range []string{"local", "pkg", "child"} {
				var source any = "registry+https://github.com/rust-lang/crates.io-index"
				manifest := filepath.Join(stage, "vendor", name+"-1.0.0", "Cargo.toml")
				p := map[string]any{"id": name, "name": name, "version": "1.0.0", "source": source, "manifest_path": manifest}
				if name == "local" {
					p["source"], p["manifest_path"] = nil, filepath.Join(stage, "project", "Cargo.toml")
					p["targets"] = []any{map[string]any{"name": "local", "kind": []string{"lib"}, "crate_types": []string{"lib"}, "edition": "2021", "src_path": filepath.Join(stage, "project", "src", "lib.rs")}}
				}
				packages = append(packages, p)
				deps := []string{}
				if name == "local" {
					deps = []string{"pkg"}
				}
				if name == "pkg" {
					deps = []string{"child"}
				}
				nodes = append(nodes, map[string]any{"id": name, "dependencies": deps, "features": []string{}})
			}
			switch mode {
			case "metadata":
				packages[1].(map[string]any)["source"] = "git+https://evil.test/repo"
			case "changedmanifest":
				if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("user edit"), 0600); err != nil {
					t.Fatal(err)
				}
			case "changedlock":
				if err := os.WriteFile(filepath.Join(root, "Cargo.lock"), []byte("user lock edit"), 0600); err != nil {
					t.Fatal(err)
				}
			case "changedpolicy":
				if err := os.WriteFile(filepath.Join(root, ".gh-flarebyte.cue"), []byte(testConfigCue()+"\ndependencies: { minAgeDays: 30 }\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "changedsource":
				if err := os.WriteFile(filepath.Join(root, "src", "main.rs"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "staged":
				if err := os.WriteFile(filepath.Join(stage, "project", "Cargo.toml"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			return json.Marshal(map[string]any{"packages": packages, "workspace_members": []string{"local"}, "resolve": map[string]any{"root": "local", "nodes": nodes}})
		}, nil
	}
}

func TestCargoDependencyInstall(t *testing.T) {
	for _, mode := range []string{"success", "artifact", "native", "metadata", "staged", "changedmanifest", "changedlock", "changedpolicy", "changedsource"} {
		t.Run(mode, func(t *testing.T) {
			root, r, blobs := approvedCargoInstall(t)
			stubCargoInstall(t, root, blobs, mode)
			before, err := os.ReadFile("Cargo.toml")
			if err != nil {
				t.Fatal(err)
			}
			lock, err := os.ReadFile("Cargo.lock")
			if err != nil {
				t.Fatal(err)
			}
			err = installRecommendedDependency(context.Background(), root, &r)
			if mode == "success" {
				if err != nil || r.Installation == nil || !sameFileBytes("Cargo.lock", lock) || !strings.Contains(string(mustReadCargoFile(t, "Cargo.toml")), `pkg = "=1.0.0"`) {
					t.Fatal("install failed", err, r.Installation)
				}
				info, _ := os.Stat("Cargo.toml")
				if info.Mode().Perm() != 0644 {
					t.Fatal("manifest permissions changed")
				}
			} else {
				if err == nil || r.Installation != nil {
					t.Fatal("unsafe install accepted", err)
				}
				if mode != "changedmanifest" && !sameFileBytes("Cargo.toml", before) {
					t.Fatal("failed install changed manifest")
				}
			}
			if !sameFileBytes("src/lib.rs", []byte("// unchanged source\n")) {
				t.Fatal("source overwritten")
			}
			if _, err := os.Stat(".gh-flarebyte-dep.lock"); !os.IsNotExist(err) {
				t.Fatal("installation lock not cleaned up")
			}
		})
	}
}
func mustReadCargoFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCargoDependencyPreconditions(t *testing.T) {
	for _, mode := range []string{"denied", "stale", "future", "policy", "lock", "unsupported context"} {
		t.Run(mode, func(t *testing.T) {
			root, r, _ := approvedCargoInstall(t)
			switch mode {
			case "denied":
				r.Decision = dependencies.Deny
			case "stale":
				r.CheckedAt = time.Now().Add(-25 * time.Hour)
			case "future":
				r.CheckedAt = time.Now().Add(time.Hour)
			case "policy":
				r.PolicyDigest = "changed"
			case "lock":
				if err := os.WriteFile(".gh-flarebyte-dep.lock", nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "unsupported context":
				if err := os.WriteFile("build.rs", nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := mustReadCargoFile(t, "Cargo.toml")
			old := dependencyCargoArtifact
			t.Cleanup(func() { dependencyCargoArtifact = old })
			dependencyCargoArtifact = func(context.Context, dependencies.CargoArtifact) ([]byte, error) {
				t.Fatal("download before preconditions passed")
				return nil, nil
			}
			if err := installCargoDependency(context.Background(), root, &r); err == nil || !sameFileBytes("Cargo.toml", before) {
				t.Fatal("failed precondition modified project", err)
			}
		})
	}
}

func TestCargoCommitRecovery(t *testing.T) {
	for _, mode := range []string{"rename failure", "backup edit", "lock edit", "destination edit"} {
		t.Run(mode, func(t *testing.T) {
			root, r, blobs := approvedCargoInstall(t)
			stubCargoInstall(t, root, blobs, "success")
			before := mustReadCargoFile(t, "Cargo.toml")
			old := dependencyRename
			t.Cleanup(func() { dependencyRename = old })
			dependencyRename = func(from, to string) error {
				if mode == "rename failure" {
					return errors.New("injected failure")
				}
				if err := os.Rename(from, to); err != nil {
					return err
				}
				switch mode {
				case "backup edit":
					return os.WriteFile(to, []byte("concurrent manifest edit"), 0600)
				case "lock edit":
					return os.WriteFile(filepath.Join(root, "Cargo.lock"), []byte("concurrent lock edit"), 0600)
				case "destination edit":
					return os.WriteFile(from, []byte("concurrent manifest edit"), 0600)
				}
				return nil
			}
			if err := installCargoDependency(context.Background(), root, &r); err == nil {
				t.Fatal("concurrent edit accepted")
			}
			if mode == "backup edit" || mode == "destination edit" {
				if string(mustReadCargoFile(t, "Cargo.toml")) != "concurrent manifest edit" {
					t.Fatal("user edit overwritten")
				}
			} else if !sameFileBytes("Cargo.toml", before) {
				t.Fatal("manifest not restored")
			}
			stages, _ := filepath.Glob(filepath.Join(root, ".gh-flarebyte-dep-*"))
			if mode == "destination edit" && len(stages) != 1 {
				t.Fatal("recovery data discarded")
			}
			if mode != "destination edit" && len(stages) != 0 {
				t.Fatal("unnecessary recovery data retained")
			}
		})
	}
}

func TestCargoDependencyInstallIntegration(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_CARGO_INTEGRATION") != "1" {
		t.Skip("opt-in offline guarded Cargo install")
	}
	for _, mode := range []string{"library", "binary", "mixed", "renamed"} {
		t.Run(mode, func(t *testing.T) {
			transform := func(m string) string { return m }
			if mode == "renamed" {
				transform = func(m string) string {
					return strings.Replace(m, "pkg = '1.0'", "alias = { package = 'pkg', version = '1.0', default-features = true, features = [] } # keep options", 1)
				}
			}
			root, r, blobs := approvedCargoInstall(t, transform)
			if mode == "binary" {
				if err := os.Remove("src/lib.rs"); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "binary" || mode == "mixed" {
				if err := os.WriteFile("src/main.rs", []byte("fn main() {}\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			old := dependencyCargoArtifact
			t.Cleanup(func() { dependencyCargoArtifact = old })
			dependencyCargoArtifact = func(_ context.Context, a dependencies.CargoArtifact) ([]byte, error) { return blobs[a.Node.Name], nil }
			lock := mustReadCargoFile(t, "Cargo.lock")
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := installRecommendedDependency(ctx, root, &r); err != nil {
				t.Fatal(err)
			}
			if r.Installation == nil || !sameFileBytes("Cargo.lock", lock) {
				t.Fatal("Cargo pin missing or lock changed")
			}
			if mode == "renamed" && !strings.Contains(string(mustReadCargoFile(t, "Cargo.toml")), `version = "=1.0.0", default-features = true, features = [] } # keep options`) {
				t.Fatal("renamed options/comments changed")
			}
			if _, err := os.Stat("target"); !os.IsNotExist(err) {
				t.Fatal("install produced build outputs")
			}
		})
	}
}

func TestCargoDependencyCommand(t *testing.T) {
	for _, mode := range []string{"allow", "deny", "unsupported", "failure"} {
		t.Run(mode, func(t *testing.T) {
			root, r, blobs := approvedCargoInstall(t)
			nativeMode := "success"
			expected := ExitOK
			switch mode {
			case "deny":
				r.Decision = dependencies.Deny
				expected = ExitFailure
			case "unsupported":
				if err := os.WriteFile("build.rs", nil, 0600); err != nil {
					t.Fatal(err)
				}
				expected = ExitDependencyUnsupported
			case "failure":
				nativeMode = "artifact"
				expected = ExitDependencyFailure
			}
			stubCargoInstall(t, root, blobs, nativeMode)
			old := assessDependency
			t.Cleanup(func() { assessDependency = old })
			assessDependency = func(context.Context, string, string, string, *string, dependencies.Policy, time.Time) dependencies.Report {
				return r
			}
			var stdout, stderr bytes.Buffer
			result := Run([]string{"dep", "install", "cargo", "pkg", "--json"}, &stdout, &stderr)
			if result.ExitCode != expected || stderr.Len() != 0 {
				t.Fatal(result.ExitCode, stdout.String(), stderr.String())
			}
			var report dependencies.Report
			if json.Unmarshal(stdout.Bytes(), &report) != nil {
				t.Fatal("invalid JSON")
			}
			if mode == "allow" && (report.Installation == nil || report.Installation.Version != "1.0.0" || report.Decision != dependencies.Allow) {
				t.Fatal(report)
			}
			if mode == "deny" && report.Decision != dependencies.Deny {
				t.Fatal(report)
			}
			if mode == "unsupported" && (report.Error == nil || report.Error.Code != "INSTALL_UNSUPPORTED") {
				t.Fatal(report)
			}
			if mode == "failure" && (report.Error == nil || report.Error.Code != "INSTALL_FAILED") {
				t.Fatal(report)
			}
		})
	}
}
