package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

func cargoSDKFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root, stage, tools := t.TempDir(), t.TempDir(), t.TempDir()
	var err error
	stage, err = filepath.EvalSymlinks(stage)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"project", "home"} {
		if err := os.Mkdir(filepath.Join(stage, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	// Both scripts refuse inherited secrets, options, wrappers and shared homes.
	preamble := "#!/bin/sh\n[ -z \"$GH_TOKEN$GITHUB_TOKEN$CARGO_REGISTRIES_CRATES_IO_TOKEN$RUSTFLAGS$CARGO_ENCODED_RUSTFLAGS$RUSTUP_TOOLCHAIN$LD_PRELOAD$DYLD_INSERT_LIBRARIES\" ] || exit 9\n[ -z \"$RUSTC_WRAPPER$RUSTC_WORKSPACE_WRAPPER\" ] || exit 9\n[ \"$CARGO_NET_OFFLINE\" = true ] || exit 9\n[ \"$CARGO_HOME\" = '" + filepath.Join(stage, "home") + "' ] || exit 9\n[ \"$HOME\" = '" + filepath.Join(stage, "user") + "' ] || exit 9\n"
	cargo := preamble + "if [ \"$1\" = --version ]; then printf 'cargo 1.99.1 (fixture)'; exit 0; fi\nprintf '%s\\n' \"$@\" > '" + filepath.Join(tools, "arguments") + "'\nprintf '{\"fixture\":true}'\n"
	rust := preamble + "[ \"$1\" = -vV ] || exit 9\nprintf 'rustc 1.99.1 (fixture)\\nhost: aarch64-apple-darwin\\nrelease: 1.99.1\\n'\n"
	for name, body := range map[string]string{"cargo": cargo, "rustc": rust} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", tools)
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN", "CARGO_REGISTRIES_CRATES_IO_TOKEN", "RUSTFLAGS", "CARGO_ENCODED_RUSTFLAGS", "RUSTUP_TOOLCHAIN", "RUSTC_WRAPPER", "RUSTC_WORKSPACE_WRAPPER", "LD_PRELOAD", "DYLD_INSERT_LIBRARIES"} {
		t.Setenv(name, "must-not-inherit")
	}
	return root, stage, tools
}

func TestDependencyCargoSDK(t *testing.T) {
	root, stage, tools := cargoSDKFixture(t)
	validated := false
	runner, err := prepareDependencyCargo(root, stage, func(c, r string) error { validated = c == "1.99.1" && r == "1.99.1"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	b, err := runner(context.Background())
	if err != nil || !validated || string(b) != `{"fixture":true}` {
		t.Fatal(string(b), validated, err)
	}
	args, err := os.ReadFile(filepath.Join(tools, "arguments"))
	if err != nil || string(args) != "metadata\n--offline\n--locked\n--format-version\n1\n--filter-platform\naarch64-apple-darwin\n" {
		t.Fatal(string(args), err)
	}
}

func TestDependencyCargoSDKRejections(t *testing.T) {
	for _, test := range []string{"validator", "cargo version", "rust host", "duplicate host", "duplicate release", "command failure", "output limit", "cancel"} {
		t.Run(test, func(t *testing.T) {
			root, stage, tools := cargoSDKFixture(t)
			validate := func(string, string) error { return nil }
			if test == "validator" {
				validate = func(string, string) error { return dependencies.ErrInstallUnsupported }
			}
			var tool, body string
			switch test {
			case "cargo version":
				tool, body = "cargo", "printf 'wrong'"
			case "rust host":
				tool, body = "rustc", "printf 'host: --execute\\nrelease: 1.99.1\\n'"
			case "duplicate host":
				tool, body = "rustc", "printf 'host: aarch64-apple-darwin\\nhost: aarch64-apple-darwin\\nrelease: 1.99.1\\n'"
			case "duplicate release":
				tool, body = "rustc", "printf 'host: aarch64-apple-darwin\\nrelease: 1.99.1\\nrelease: 1.99.1\\n'"
			case "command failure":
				tool, body = "cargo", "printf 'secret' >&2; exit 1"
			case "output limit":
				tool, body = "cargo", "/usr/bin/head -c 5000 /dev/zero"
			}
			if tool != "" {
				if err := os.WriteFile(filepath.Join(tools, tool), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			runner, err := prepareDependencyCargo(root, stage, validate)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test == "cancel" {
				cancel()
			}
			if _, err = runner(ctx); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("unsafe SDK accepted or stderr exposed", err)
			}
			if test == "validator" && !errors.Is(err, dependencies.ErrInstallUnsupported) {
				t.Fatal(err)
			}
		})
	}
	for _, test := range []string{"nil validator", "ancestor config", "repository stage", "rustup proxy"} {
		t.Run(test, func(t *testing.T) {
			root, stage, tools := cargoSDKFixture(t)
			validate := func(string, string) error { return nil }
			switch test {
			case "nil validator":
				validate = nil
			case "ancestor config":
				if err := os.Mkdir(filepath.Join(stage, ".cargo"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(stage, ".cargo", "config.toml"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "repository stage":
				root = stage
			case "rustup proxy":
				if err := os.Rename(filepath.Join(tools, "cargo"), filepath.Join(tools, "rustup")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("rustup", filepath.Join(tools, "cargo")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := prepareDependencyCargo(root, stage, validate); err == nil {
				t.Fatal("unsafe preparation accepted")
			}
		})
	}
}

func TestDependencyCargoSDKIntegration(t *testing.T) {
	if os.Getenv("GH_FLAREBYTE_DEP_CARGO_INTEGRATION") != "1" {
		t.Skip("opt-in offline standalone Cargo SDK")
	}
	root, stage := t.TempDir(), t.TempDir()
	for name, data := range map[string]string{"project/Cargo.toml": "[package]\nname='sdk-fixture'\nversion='1.0.0'\nedition='2021'\n[workspace]\n", "project/Cargo.lock": "version = 4\n[[package]]\nname='sdk-fixture'\nversion='1.0.0'\n", "project/src/lib.rs": "", "project/build.rs": `fn main() { std::fs::write("hook-executed", "unexpected").unwrap(); }`, "home/config.toml": "[net]\noffline=true\n"} {
		path := filepath.Join(stage, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	runner, err := prepareDependencyCargo(root, stage, func(c, r string) error {
		if nativeMajor(c) != 1 || nativeMajor(r) != 1 {
			return errors.New("unexpected SDK")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	b, err := runner(ctx)
	if err != nil || !strings.Contains(string(b), `"sdk-fixture"`) {
		t.Fatal(string(b), err)
	}
	if _, err := os.Stat(filepath.Join(stage, "project", "hook-executed")); !os.IsNotExist(err) {
		t.Fatal("metadata executed a build script")
	}
}
