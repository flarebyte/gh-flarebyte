package dependencies

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// This opt-in oracle uses only generated path packages. Production checks never
// invoke Cargo; metadata must not execute fixture build scripts or proc macros.
func TestCargoFeatureOracle(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_CARGO_INTEGRATION") != "1" {
		t.Skip("opt-in offline Cargo feature oracle")
	}
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Fatal(err)
	}
	cargo, err = filepath.EvalSymlinks(cargo)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := filepath.Rel(repo, cargo); err != nil || filepath.IsLocal(rel) {
		t.Fatal("repository-local Cargo refused")
	}
	for _, test := range []string{"defaults", "strong", "strong explicit alias", "weak inactive", "weak activated", "namespaced", "renamed", "no defaults"} {
		t.Run(test, func(t *testing.T) {
			p := featureProject()
			pkg := p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")]
			switch test {
			case "strong":
				pkg.features["default"] = []string{"child/extra"}
			case "strong explicit alias":
				pkg.features["default"] = []string{"child/extra"}
				pkg.features["child"] = []string{"unrelated"}
				pkg.features["unrelated"] = []string{}
			case "weak inactive":
				pkg.features["default"] = []string{"child?/extra"}
			case "weak activated":
				pkg.features["default"] = []string{"child?/extra", "dep:child"}
			case "namespaced":
				pkg.features["default"] = []string{"dep:child"}
			case "renamed":
				pkg.dependencies[0].alias = "renamed"
				pkg.features["default"] = []string{"renamed/extra"}
			case "no defaults":
				p.cargo.featurePackages[p.cargo.root].dependencies[0].defaults = false
			}
			dir := t.TempDir()
			for path, model := range p.cargo.featurePackages {
				name := p.cargo.packages[path].Name
				base := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Join(base, "src"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(base, "src", "lib.rs"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(base, "build.rs"), []byte(`fn main() { std::fs::write("hook-executed", "unexpected").unwrap(); }`), 0600); err != nil {
					t.Fatal(err)
				}
				deps := map[string]any{}
				for _, d := range model.dependencies {
					deps[d.alias] = map[string]any{"package": d.name, "path": "../" + d.name, "version": d.requirement, "optional": d.optional, "default-features": d.defaults, "features": append([]string{}, d.features...)}
				}
				manifest := map[string]any{"package": map[string]string{"name": name, "version": "1.0.0", "edition": "2021", "rust-version": "1.85"}, "dependencies": deps, "features": model.features}
				if name == "child" {
					manifest["lib"] = map[string]bool{"proc-macro": true}
				}
				data, err := toml.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(base, "Cargo.toml"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, cargo, "metadata", "--offline", "--format-version", "1", "--manifest-path", filepath.Join(dir, "local", "Cargo.toml"))
			cmd.Dir = filepath.Join(dir, "local")
			cmd.Env = []string{"PATH=" + filepath.Dir(cargo) + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin", "CARGO_HOME=" + filepath.Join(dir, "cache"), "HOME=" + dir, "TMPDIR=" + dir, "RUSTC_WRAPPER=", "RUSTC_WORKSPACE_WRAPPER=", "CARGO_NET_OFFLINE=true"}
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("Cargo metadata: %v: %s", err, stderr.String())
			}
			var metadata struct {
				Packages []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"packages"`
				Resolve struct {
					Nodes []struct {
						ID       string   `json:"id"`
						Features []string `json:"features"`
					} `json:"nodes"`
				} `json:"resolve"`
			}
			if err := json.Unmarshal(out.Bytes(), &metadata); err != nil {
				t.Fatal(err)
			}
			names := map[string]string{}
			for _, pkg := range metadata.Packages {
				names[pkg.ID] = pkg.Name
			}
			actual := map[string][]string{}
			for _, node := range metadata.Resolve.Nodes {
				actual[names[node.ID]] = node.Features
			}
			proof := resolveCargoFeatures(p)
			if test == "weak inactive" {
				if proof.Status != "unknown" || !slices.Contains(proof.Missing, "CARGO_FEATURE_CONTEXT") {
					t.Fatal("weak context lost fail-closed verdict", proof)
				}
			} else {
				if proof.Status != "resolved" {
					t.Fatal(proof)
				}
				if len(actual) != len(proof.Enabled) {
					t.Fatal("active package disagreement", actual, proof.Enabled)
				}
				for _, enabled := range proof.Enabled {
					name := p.cargo.packages[enabled.Path].Name
					if !slices.Equal(actual[name], enabled.Features) {
						t.Fatal("feature disagreement", name, actual[name], enabled.Features)
					}
				}
			}
			for _, name := range []string{"local", "pkg", "child"} {
				if _, err := os.Stat(filepath.Join(dir, name, "hook-executed")); !os.IsNotExist(err) {
					t.Fatal("metadata executed a build script")
				}
			}
		})
	}
}
