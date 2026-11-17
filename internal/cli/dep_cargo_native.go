// purpose: Resolve staged Cargo directory sources using a trusted standalone SDK.
// responsibilities: Isolate credentials/configuration and allow metadata inspection only.
// architecture notes: No compilation, package installation, plugins or compiler wrappers are invoked.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

var dependencyCargoHost = regexp.MustCompile(`^[a-z0-9_]+(-[a-z0-9_]+){2,4}$`)

type dependencyCargoOutput struct {
	data  []byte
	limit int
}

func (w *dependencyCargoOutput) Write(p []byte) (int, error) {
	if len(p) > w.limit-len(w.data) {
		return 0, fmt.Errorf("Cargo output exceeds limit")
	}
	w.data = append(w.data, p...)
	return len(p), nil
}

// validate runs before metadata, allowing the stage to reject incompatible SDKs.
func prepareDependencyCargo(root, stage string, validate func(string, string) error) (func(context.Context) ([]byte, error), error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, fmt.Errorf("%w: native platform", dependencies.ErrInstallUnsupported)
	}
	if validate == nil {
		return nil, fmt.Errorf("Cargo SDK validation required")
	}
	cargo, err := trustedDependencyTool(root, "cargo")
	if err != nil {
		return nil, err
	}
	rustc, err := trustedDependencyTool(root, "rustc")
	if err != nil {
		return nil, err
	}
	if filepath.Base(cargo) != "cargo" || filepath.Base(rustc) != "rustc" {
		return nil, fmt.Errorf("%w: standalone Cargo and rustc required", dependencies.ErrInstallUnsupported)
	}
	stage, err = filepath.EvalSymlinks(stage)
	if err != nil {
		return nil, err
	}
	stage, err = filepath.Abs(stage)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if relative, err := filepath.Rel(root, stage); err != nil || filepath.IsLocal(relative) || relative == "." {
		return nil, fmt.Errorf("Cargo SDK stage must be outside the repository")
	}
	// Cargo searches ancestor configuration even with a separate CARGO_HOME.
	// Refuse it instead of inheriting source replacements or executable helpers.
	for dir := filepath.Join(stage, "project"); ; dir = filepath.Dir(dir) {
		if dir != filepath.Join(stage, "project") {
			if _, err := os.Lstat(filepath.Join(dir, "Cargo.toml")); !os.IsNotExist(err) {
				return nil, fmt.Errorf("%w: ancestor Cargo manifest", dependencies.ErrInstallUnsupported)
			}
		}
		for _, name := range []string{".cargo/config", ".cargo/config.toml"} {
			if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				return nil, fmt.Errorf("%w: ancestor Cargo configuration", dependencies.ErrInstallUnsupported)
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	for _, name := range []string{"tmp", "user"} {
		if err := os.Mkdir(filepath.Join(stage, name), 0700); err != nil {
			return nil, err
		}
	}
	env := []string{"PATH=" + filepath.Dir(rustc) + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin", "LANG=C", "HOME=" + filepath.Join(stage, "user"), "CARGO_HOME=" + filepath.Join(stage, "home"), "TMPDIR=" + filepath.Join(stage, "tmp"), "TMP=" + filepath.Join(stage, "tmp"), "TEMP=" + filepath.Join(stage, "tmp"), "CARGO_NET_OFFLINE=true", "CARGO_TARGET_DIR=" + filepath.Join(stage, "target"), "RUSTC=" + rustc, "RUSTC_WRAPPER=", "RUSTC_WORKSPACE_WRAPPER="}
	run := func(ctx context.Context, tool string, args []string, limit int) ([]byte, error) {
		// Executables are resolved outside the repository; arguments below are a
		// fixed allow-list and never contain project-provided executable options.
		// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		cmd := exec.CommandContext(ctx, tool, args...)
		cmd.Dir = filepath.Join(stage, "project")
		cmd.Env = env
		out, stderr := dependencyCargoOutput{limit: limit}, dependencyCargoOutput{limit: 64 << 10}
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("isolated Cargo SDK command failed")
		}
		return out.data, nil
	}
	return func(ctx context.Context) ([]byte, error) {
		b, err := run(ctx, cargo, []string{"--version"}, 4096)
		if err != nil {
			return nil, err
		}
		fields := strings.Fields(string(b))
		if len(fields) < 2 || fields[0] != "cargo" {
			return nil, fmt.Errorf("invalid Cargo SDK version")
		}
		cargoVersion := fields[1]
		b, err = run(ctx, rustc, []string{"-vV"}, 4096)
		if err != nil {
			return nil, err
		}
		rust, host := "", ""
		for _, line := range strings.Split(string(b), "\n") {
			key, value, ok := strings.Cut(line, ": ")
			if !ok {
				continue
			}
			if key == "release" {
				if rust != "" {
					return nil, fmt.Errorf("ambiguous Rust SDK version")
				}
				rust = value
			}
			if key == "host" {
				if host != "" {
					return nil, fmt.Errorf("ambiguous Rust SDK host")
				}
				host = value
			}
		}
		if !dependencyCargoHost.MatchString(host) {
			return nil, fmt.Errorf("unsupported Rust SDK host")
		}
		if err := validate(cargoVersion, rust); err != nil {
			return nil, err
		}
		return run(ctx, cargo, []string{"metadata", "--offline", "--locked", "--format-version", "1", "--filter-platform", host}, 4<<20)
	}, nil
}
