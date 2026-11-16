// purpose: Run an isolated npm toolchain only after an installation plan is approved.
// responsibilities: Reject repository-local SDKs, suppress hooks and isolate caches and configuration.
// architecture notes: Native commands receive no credential environment and install exclusively from verified cached artifacts.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

type dependencyNativeRunner func(context.Context, []string) error

var dependencyNPMRunner = prepareDependencyNPM

type dependencyOutput struct{ data []byte }

func (w *dependencyOutput) Write(p []byte) (int, error) {
	if len(w.data)+len(p) > 64<<10 {
		return 0, fmt.Errorf("native output exceeds limit")
	}
	w.data = append(w.data, p...)
	return len(p), nil
}

func trustedDependencyTool(root, name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%w: %s unavailable", dependencies.ErrInstallUnsupported, name)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if relative, err := filepath.Rel(root, path); err != nil || filepath.IsLocal(relative) {
		return "", fmt.Errorf("repository-local tool refused")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0002 != 0 {
		return "", fmt.Errorf("unsafe SDK executable")
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil || parent.Mode().Perm()&0002 != 0 {
		return "", fmt.Errorf("world-writable SDK directory refused")
	}
	return path, nil
}

func prepareDependencyNPM(root, stage string) (dependencyNativeRunner, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, fmt.Errorf("%w: native platform", dependencies.ErrInstallUnsupported)
	}
	node, err := trustedDependencyTool(root, "node")
	if err != nil {
		return nil, err
	}
	npm, err := trustedDependencyTool(root, "npm")
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"user.npmrc", "global.npmrc"} {
		if err := os.WriteFile(filepath.Join(stage, name), nil, 0600); err != nil {
			return nil, err
		}
	}
	env := []string{"PATH=" + filepath.Dir(node) + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin", "LANG=C", "TMPDIR=" + stage, "TMP=" + stage, "TEMP=" + stage}
	base := []string{"--ignore-scripts", "--offline", "--no-audit", "--no-fund", "--update-notifier=false", "--registry=https://registry.npmjs.org", "--cache=" + filepath.Join(stage, "cache"), "--userconfig=" + filepath.Join(stage, "user.npmrc"), "--globalconfig=" + filepath.Join(stage, "global.npmrc")}
	run := func(ctx context.Context, args []string) (string, error) {
		// node/npm were resolved outside the repository and checked for writable
		// executable locations. Arguments are separate values; no shell is used.
		// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		cmd := exec.CommandContext(ctx, node, args...)
		cmd.Dir = stage
		cmd.Env = env
		var output dependencyOutput
		cmd.Stdout = &output
		cmd.Stderr = &output
		err := cmd.Run()
		if err != nil {
			message := "isolated npm command failed"
			if text := dependencies.Description(string(output.data)); text != nil {
				message += ": " + *text
			}
			return "", fmt.Errorf("%s", message)
		}
		return strings.TrimSpace(string(output.data)), nil
	}
	checked := false
	return func(ctx context.Context, args []string) error {
		// Version checks execute only the trusted SDK, not project or package code.
		if len(args) == 0 {
			return fmt.Errorf("empty native command")
		}
		if !checked {
			version, err := run(ctx, []string{"--version"})
			if err != nil || nativeMajor(version) < 24 {
				return fmt.Errorf("%w: Node 24+ required", dependencies.ErrInstallUnsupported)
			}
			version, err = run(ctx, append([]string{npm, "--version"}, base...))
			if err != nil || nativeMajor(version) < 10 {
				return fmt.Errorf("%w: npm 10+ required", dependencies.ErrInstallUnsupported)
			}
			checked = true
		}
		_, err := run(ctx, append(append([]string{"--max-old-space-size=256", npm}, args...), base...))
		return err
	}, nil
}

func nativeMajor(version string) int {
	part := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".", 2)[0]
	n, err := strconv.Atoi(part)
	if err != nil {
		return 0
	}
	return n
}
