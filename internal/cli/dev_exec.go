// purpose: Provide shared command-execution and environment helpers for dev workflows.
// responsibilities: Execute allowed toolchain commands; build process env; discover Go files; prepare config/env for dev commands.
// architecture notes: Command names are statically allowlisted and constructed via fixed exec branches to satisfy security analysis and reduce injection risk.
package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

var runCommandCapture = func(name string, args []string, env []string) (string, string, error) {
	cmd, err := newAllowedDevCommand(name, args)
	if err != nil {
		return "", "", err
	}
	if len(env) > 0 {
		cmd.Env = env
	}
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

func newAllowedDevCommand(name string, args []string) (*exec.Cmd, error) {
	switch name {
	case "go":
		return exec.Command("go", args...), nil
	case "gofmt":
		return exec.Command("gofmt", args...), nil
	case "dart":
		return exec.Command("dart", args...), nil
	default:
		return nil, fmt.Errorf("unsupported command: %s", name)
	}
}

func unsupportedLanguageResult(language string, stderr io.Writer) Result {
	err := fmt.Errorf("build.language %q is not supported. Supported values: go, dart", language)
	_, _ = fmt.Fprintln(stderr, err.Error())
	return Result{ExitCode: ExitUsage, Err: err}
}

func resolveCoverageMin(cliMin *float64, cfg config.Config) *float64 {
	if cliMin != nil {
		return cliMin
	}
	return cfg.Coverage.DefaultMinPercent
}

func buildCommandEnv(cfg config.Config, stderr io.Writer) []string {
	env := os.Environ()
	if cfg.Go.CacheDir != "" {
		cacheDir, warned := resolvePortablePath(cfg.Go.CacheDir)
		if warned {
			_, _ = fmt.Fprintf(stderr, "warning: config.go.cacheDir is absolute (%s). Prefer a project-relative path like ./.gocache for portable config.\n", cfg.Go.CacheDir)
		}
		env = append(env, "GOCACHE="+cacheDir)
	}
	if cfg.Go.ModCacheDir != "" {
		modCacheDir, warned := resolvePortablePath(cfg.Go.ModCacheDir)
		if warned {
			_, _ = fmt.Fprintf(stderr, "warning: config.go.modCacheDir is absolute (%s). Prefer a project-relative path like ./.gomodcache for portable config.\n", cfg.Go.ModCacheDir)
		}
		env = append(env, "GOMODCACHE="+modCacheDir)
	}
	if cfg.Go.Toolchain != "" {
		env = append(env, "GOTOOLCHAIN="+cfg.Go.Toolchain)
	}
	return env
}

func resolvePortablePath(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	path := raw
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				path = home
			} else if strings.HasPrefix(path, "~/") {
				path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
			}
		}
	}
	wasAbsolute := filepath.IsAbs(path)
	if !wasAbsolute {
		if abs, err := filepath.Abs(path); err == nil {
			return abs, false
		}
	}
	return path, wasAbsolute
}

func discoverGoFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "build" || name == ".gocache" || name == ".gomodcache" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func prepareDevCommand(colorOverride string, stderr io.Writer) (config.Config, []string, *Result) {
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return cfg, nil, usage
	}
	if colorOverride != "" {
		cfg.DevOutput.Color = colorOverride
	}
	return cfg, buildCommandEnv(cfg, stderr), nil
}
