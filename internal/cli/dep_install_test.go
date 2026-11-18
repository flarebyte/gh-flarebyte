package cli

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

func approvedDependencyInstall(t *testing.T) (string, dependencies.Report, []byte) {
	t.Helper()
	root := localDependencyRepo(t, "")
	manifest := []byte(`{"name":"local","version":"1.0.0"}`)
	lock := []byte(`{"lockfileVersion":3,"packages":{"":{"name":"local","version":"1.0.0"}}}`)
	for name, b := range map[string][]byte{"package.json": manifest, "package-lock.json": lock} {
		if err := os.WriteFile(name, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Locate the checked-in synthetic artifact independently of the temporary cwd.
	_, file, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "dependencies", "testdata", "fixture-lib.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	h := sha512.Sum512(b)
	r := dependencies.NewReport("npm", "fixture-lib", nil, dependencies.DefaultPolicy(14), time.Now().UTC())
	r.Decision = dependencies.Allow
	r.Scope = "project-resolution"
	r.Evidence.Identity = "verified"
	r.Evidence.Maintainers = "multiple"
	r.Recommendation = &dependencies.Candidate{Version: "1.0.0"}
	r.Details.Project = dependencies.ReadProject(root, "npm")
	r.Details.Project.Plans = []dependencies.ProjectPlan{{Name: "fixture-lib", Version: "1.0.0", Kind: "add-leaf", Resolved: "https://registry.npmjs.org/fixture-lib/-/fixture-lib-1.0.0.tgz", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(h[:])}}
	return root, r, b
}

func TestDependencyNPMOfflineIntegration(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_NPM_INTEGRATION") != "1" {
		t.Skip("opt-in synthetic npm integration")
	}
	root, r, b := approvedDependencyInstall(t)
	old := dependencyArtifact
	t.Cleanup(func() { dependencyArtifact = old })
	dependencyArtifact = func(context.Context, dependencies.InstallArtifact) ([]byte, error) { return b, nil }
	if err := installNPMDependency(context.Background(), root, &r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("node_modules/fixture-lib/hook-executed"); !os.IsNotExist(err) {
		t.Fatal("dependency lifecycle hook executed")
	}
	if r.Installation == nil {
		t.Fatal("installation was not reported")
	}
}

func approvedDependencyUpgrade(t *testing.T) (string, dependencies.Report, []byte) {
	t.Helper()
	root, r, artifact := approvedDependencyInstall(t)
	manifest := `{"name":"local","version":"1.0.0","dependencies":{"fixture-lib":"0.9.0"}}`
	lock := `{"lockfileVersion":3,"packages":{"":{"name":"local","version":"1.0.0","dependencies":{"fixture-lib":"0.9.0"}},"node_modules/fixture-lib":{"version":"0.9.0","resolved":"https://registry.npmjs.org/fixture-lib/-/fixture-lib-0.9.0.tgz","integrity":"` + r.Details.Project.Plans[0].Integrity + `"}}}`
	for name, data := range map[string]string{"package.json": manifest, "package-lock.json": lock} {
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll("node_modules/fixture-lib", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("node_modules/fixture-lib/package.json", []byte(`{"name":"fixture-lib","version":"0.9.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	plan := r.Details.Project.Plans[0]
	plan.Kind, plan.PreviousVersion = "replace-leaf", "0.9.0"
	r.Details.Project = dependencies.ReadProject(root, "npm")
	r.Details.Project.Plans = []dependencies.ProjectPlan{plan}
	r.Details.Project.Findings = []dependencies.ProjectFinding{{Path: "node_modules/fixture-lib", Package: "npm:fixture-lib", Finding: dependencies.Finding{Version: "0.9.0", Decision: dependencies.Deny, Reasons: []string{"KNOWN_VULNERABILITY"}}}}
	return root, r, artifact
}

func TestDependencyUpgrade(t *testing.T) {
	for _, mode := range []string{"success", "artifact", "command", "changedplan", "wrongversion"} {
		t.Run(mode, func(t *testing.T) {
			root, r, b := approvedDependencyUpgrade(t)
			stubDependencyNative(t, b, mode)
			beforeManifest, _ := os.ReadFile("package.json")
			beforeLock, _ := os.ReadFile("package-lock.json")
			err := installNPMDependency(context.Background(), root, &r)
			installed, readErr := os.ReadFile("node_modules/fixture-lib/package.json")
			if readErr != nil {
				t.Fatal(readErr)
			}
			if mode == "success" {
				if err != nil || r.Installation == nil || !strings.Contains(string(installed), `"1.0.0"`) {
					t.Fatal(err, string(installed))
				}
			} else {
				afterManifest, _ := os.ReadFile("package.json")
				afterLock, _ := os.ReadFile("package-lock.json")
				if err == nil || !bytes.Equal(beforeManifest, afterManifest) || !bytes.Equal(beforeLock, afterLock) || !strings.Contains(string(installed), `"0.9.0"`) {
					t.Fatal("failed upgrade modified original project", err)
				}
			}
		})
	}
}

func TestDependencyNPMOfflineUpgradeIntegration(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_NPM_INTEGRATION") != "1" {
		t.Skip("opt-in synthetic npm upgrade")
	}
	root, r, b := approvedDependencyUpgrade(t)
	old := dependencyArtifact
	t.Cleanup(func() { dependencyArtifact = old })
	dependencyArtifact = func(context.Context, dependencies.InstallArtifact) ([]byte, error) { return b, nil }
	if err := installNPMDependency(context.Background(), root, &r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("node_modules/fixture-lib/hook-executed"); !os.IsNotExist(err) {
		t.Fatal("lifecycle hook executed")
	}
	if s := dependencies.ReadProject(root, "npm"); !s.Complete || len(s.Nodes) != 1 || s.Nodes[0].Version != "1.0.0" {
		t.Fatal(s)
	}
}

func TestLegacyNPMInstallApproved(t *testing.T) {
	root, r, b := approvedDependencyInstall(t)
	stubDependencyNative(t, b, "success")
	if err := installRecommendedDependency(context.Background(), root, &r); err != nil || r.Installation == nil || r.Installation.Version != "1.0.0" {
		t.Fatalf("legacy install: %v %+v", err, r.Installation)
	}
}

func stubDependencyNative(t *testing.T, b []byte, mode string) {
	t.Helper()
	oldArtifact, oldRunner := dependencyArtifact, dependencyNPMRunner
	t.Cleanup(func() { dependencyArtifact = oldArtifact; dependencyNPMRunner = oldRunner })
	dependencyArtifact = func(context.Context, dependencies.InstallArtifact) ([]byte, error) {
		if mode == "artifact" {
			return []byte("wrong"), nil
		}
		return b, nil
	}
	dependencyNPMRunner = func(root, stage string) (dependencyNativeRunner, error) {
		if mode == "tool" {
			return nil, errors.New("tool unavailable")
		}
		return func(ctx context.Context, args []string) error {
			if mode == "command" {
				return errors.New("native command failed")
			}
			if args[0] == "cache" {
				if len(args) != 3 || args[1] != "add" {
					t.Fatal(args)
				}
				return nil
			}
			if len(args) != 1 || args[0] != "ci" {
				t.Fatal(args)
			}
			if mode == "changedplan" {
				return os.WriteFile(filepath.Join(stage, "package-lock.json"), []byte(`{}`), 0600)
			}
			if mode == "concurrent" {
				if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"user":"edit"}`), 0600); err != nil {
					return err
				}
			}
			module := filepath.Join(stage, "node_modules", "fixture-lib")
			if err := os.MkdirAll(module, 0700); err != nil {
				return err
			}
			version := "1.0.0"
			if mode == "wrongversion" {
				version = "2.0.0"
			}
			if err := os.WriteFile(filepath.Join(module, "package.json"), []byte(`{"name":"fixture-lib","version":"`+version+`"}`), 0600); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(module, "index.js"), []byte("module.exports = 42;"), 0600)
		}, nil
	}
}

func TestGuardedDependencyInstallation(t *testing.T) {
	for _, mode := range []string{"success", "artifact", "tool", "command", "changedplan", "wrongversion", "concurrent"} {
		t.Run(mode, func(t *testing.T) {
			root, r, b := approvedDependencyInstall(t)
			before, err := os.ReadFile("package.json")
			if err != nil {
				t.Fatal(err)
			}
			stubDependencyNative(t, b, mode)
			if err := os.Mkdir("node_modules", 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("node_modules/old.txt", []byte("original tree"), 0600); err != nil {
				t.Fatal(err)
			}
			err = installNPMDependency(context.Background(), root, &r)
			if mode == "success" {
				if err != nil || r.Installation == nil || r.Installation.Version != "1.0.0" || r.Installation.AssessmentReference == "" {
					t.Fatal(r.Installation, err)
				}
				var manifest struct {
					Dependencies map[string]string `json:"dependencies"`
				}
				data, _ := os.ReadFile("package.json")
				if json.Unmarshal(data, &manifest) != nil || manifest.Dependencies["fixture-lib"] != "1.0.0" {
					t.Fatal(string(data))
				}
				if _, err := os.Stat("node_modules/fixture-lib/package.json"); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat("node_modules/old.txt"); !os.IsNotExist(err) {
					t.Fatal("old tree not replaced")
				}
			} else {
				if err == nil {
					t.Fatal("unsafe install succeeded")
				}
				after, _ := os.ReadFile("package.json")
				if mode == "concurrent" {
					if !strings.Contains(string(after), "user") {
						t.Fatal("user edit overwritten")
					}
				} else if string(after) != string(before) {
					t.Fatal("failed install mutated manifest")
				}
				if b, err := os.ReadFile("node_modules/old.txt"); err != nil || string(b) != "original tree" {
					t.Fatal("failed install modified original tree")
				}
			}
			if _, err := os.Stat(".gh-flarebyte-dep.lock"); !os.IsNotExist(err) {
				t.Fatal("lock retained")
			}
		})
	}
}

func TestPostCommitEditPreservesRecovery(t *testing.T) {
	root, r, b := approvedDependencyInstall(t)
	stubDependencyNative(t, b, "success")
	if err := os.Mkdir("node_modules", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("node_modules/original", []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	old := dependencyRename
	t.Cleanup(func() { dependencyRename = old })
	dependencyRename = func(from, to string) error {
		if err := os.Rename(from, to); err != nil {
			return err
		}
		if to == filepath.Join(root, "node_modules") {
			return os.WriteFile(filepath.Join(root, "package.json"), []byte("concurrent edit"), 0600)
		}
		return nil
	}
	if err := installNPMDependency(context.Background(), root, &r); err == nil {
		t.Fatal("concurrent post-commit edit accepted")
	}
	if b, _ := os.ReadFile("package.json"); string(b) != "concurrent edit" {
		t.Fatal("concurrent edit overwritten")
	}
	if b, _ := os.ReadFile("node_modules/original"); string(b) != "old" {
		t.Fatal("original tree not restored")
	}
}

func TestDependencyCommitRecovery(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "preserve edit"}[conflict], func(t *testing.T) {
			root, r, b := approvedDependencyInstall(t)
			before, _ := os.ReadFile("package.json")
			stubDependencyNative(t, b, "success")
			old := dependencyRename
			t.Cleanup(func() { dependencyRename = old })
			dependencyRename = func(from, to string) error {
				if to == filepath.Join(root, "node_modules") {
					if conflict {
						if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("user edit"), 0600); err != nil {
							return err
						}
					}
					return errors.New("injected rename failure")
				}
				return os.Rename(from, to)
			}
			if err := installNPMDependency(context.Background(), root, &r); err == nil {
				t.Fatal("commit unexpectedly succeeded")
			}
			after, _ := os.ReadFile("package.json")
			if conflict {
				if string(after) != "user edit" {
					t.Fatal("concurrent edit overwritten")
				}
				stages, _ := filepath.Glob(filepath.Join(root, ".gh-flarebyte-dep-*"))
				if len(stages) != 1 {
					t.Fatal("recovery files discarded")
				}
			} else if string(after) != string(before) {
				t.Fatal("rollback failed")
			}
		})
	}
}

func TestDependencyInstallPreconditions(t *testing.T) {
	root, r, b := approvedDependencyInstall(t)
	stubDependencyNative(t, b, "success")
	r.CheckedAt = time.Now().Add(-25 * time.Hour)
	if installNPMDependency(context.Background(), root, &r) == nil {
		t.Fatal("stale approval accepted")
	}
	r.CheckedAt = time.Now()
	if err := os.WriteFile(".gh-flarebyte-dep.lock", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if installNPMDependency(context.Background(), root, &r) == nil {
		t.Fatal("concurrent installer accepted")
	}
	if err := os.Remove(".gh-flarebyte-dep.lock"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gh-flarebyte.cue", []byte(testConfigCue()+"\ndependencies: { minAgeDays: 30 }"), 0600); err != nil {
		t.Fatal(err)
	}
	if installNPMDependency(context.Background(), root, &r) == nil {
		t.Fatal("changed policy accepted")
	}
}
