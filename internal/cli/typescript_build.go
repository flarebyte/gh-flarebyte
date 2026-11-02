// purpose: Distinguish TypeScript library verification from native Bun CLI artifact builds.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

type typescriptTargetError struct{ error }

func validateTypeScriptTarget(cfg config.Config, target string) error {
	if cfg.Build.Mode == "library" && target != "" {
		return fmt.Errorf("--target is not supported for TypeScript library verification")
	}
	if target != "" && !contains(cfg.Build.Targets, target) {
		return fmt.Errorf("invalid --target %q: not present in build.targets", target)
	}
	targets := cfg.Build.Targets
	if target != "" {
		targets = []string{target}
	}
	for _, candidate := range targets {
		if !contains([]string{"darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64"}, candidate) {
			return fmt.Errorf("unsupported TypeScript target %q: initial CLI support is native macOS or GNU/Linux on amd64/arm64", candidate)
		}
	}
	return nil
}

func runTypeScriptBuild(cfg config.Config, target, output string, stdout, stderr io.Writer) Result {
	if err := validateTypeScriptTarget(cfg, target); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitUsage, Err: err}
	}
	assets, err := buildTypeScript(cfg, target, output, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		var targetErr typescriptTargetError
		if errors.As(err, &targetErr) {
			return Result{ExitCode: ExitUsage, Err: err}
		}
		return Result{ExitCode: ExitBuildFailure, Err: err}
	}
	if cfg.Build.Mode == "library" {
		_, _ = fmt.Fprintln(stdout, "Build complete: TypeScript library type verification passed; no release artifacts.")
	} else {
		_, _ = fmt.Fprintf(stdout, "Build complete: TypeScript CLI artifacts with checksums in %s.\n", assets[len(assets)-1])
	}
	return Result{ExitCode: ExitOK}
}

func buildTypeScript(cfg config.Config, target, outputOverride string, stderr io.Writer) ([]string, error) {
	if err := validateTypeScriptTarget(cfg, target); err != nil {
		return nil, err
	}
	env := buildCommandEnv(cfg, stderr)
	project, err := prepareTypeScript(env)
	if err != nil {
		return nil, err
	}
	host, entry := "", ""
	if cfg.Build.Mode == "binary" {
		host, err = typescriptRun([]string{"--eval", `process.stdout.write(process.platform + "-" + process.arch)`}, env)
		if err != nil {
			return nil, err
		}
		host = strings.ReplaceAll(strings.TrimSpace(host), "-x64", "-amd64")
		targets := cfg.Build.Targets
		if target != "" {
			targets = []string{target}
		}
		for _, candidate := range targets {
			if candidate != host {
				return nil, typescriptTargetError{fmt.Errorf("TypeScript CLI target %q differs from the active Bun host %s; cross-compilation is outside initial support", candidate, host)}
			}
		}
		entry, err = filepath.Abs(cfg.Build.MainPackage)
		if err == nil {
			entry, err = filepath.EvalSymlinks(entry)
		}
		if _, ok := project.Files[entry]; err != nil || !ok {
			return nil, fmt.Errorf("build.mainPackage %q must be a source file included by the root tsconfig.json", cfg.Build.MainPackage)
		}
	}
	if err := typescriptTypecheck(env); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-typescript-build-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if cfg.Build.RunTests {
		if _, err := typescriptTest(project, env, false, dir); err != nil {
			return nil, err
		}
	}
	if cfg.Build.Mode == "library" {
		return nil, nil
	}
	binary := filepath.Join(dir, cfg.Project.Repo)
	if _, err := typescriptRun([]string{"build", "--compile", entry, "--outfile", binary, "--no-compile-autoload-dotenv", "--no-compile-autoload-bunfig"}, env); err != nil {
		return nil, err
	}
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Bun did not produce the CLI executable")
	}
	output := cfg.Build.OutputDir
	if outputOverride != "" {
		output = outputOverride
	}
	assets, err := packageTargetBinary(cfg, []string{host}, host, binary, output)
	if err != nil {
		return nil, err
	}
	return writeArtifactChecksums(assets, output, resolveChecksumPath(cfg.Build.ChecksumFile, outputOverride))
}
