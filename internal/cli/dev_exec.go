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
	cmd := exec.Command(name, args...)
	if len(env) > 0 {
		cmd.Env = env
	}
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
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

func buildCommandEnv(cfg config.Config) []string {
	env := os.Environ()
	if cfg.Go.CacheDir != "" {
		env = append(env, "GOCACHE="+cfg.Go.CacheDir)
	}
	if cfg.Go.ModCacheDir != "" {
		env = append(env, "GOMODCACHE="+cfg.Go.ModCacheDir)
	}
	if cfg.Go.Toolchain != "" {
		env = append(env, "GOTOOLCHAIN="+cfg.Go.Toolchain)
	}
	return env
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
	return cfg, buildCommandEnv(cfg), nil
}
