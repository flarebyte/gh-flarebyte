// purpose: Build Rust libraries, packaged crates, and target binaries with fresh release assets.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func runRustBuild(cfg config.Config, target, output string, stdout, stderr io.Writer) Result {
	if err := validateRustBuildTarget(cfg, target); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitUsage, Err: err}
	}
	artifacts, err := buildRust(cfg, target, output, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitBuildFailure, Err: err}
	}
	if len(artifacts) == 0 {
		_, _ = fmt.Fprintln(stdout, "Build complete: Rust library compile verification passed.")
	} else {
		_, _ = fmt.Fprintf(stdout, "Build complete: Rust %s artifacts with checksums in %s.\n", cfg.Build.Mode, artifacts[len(artifacts)-1])
	}
	return Result{ExitCode: ExitOK}
}

func validateRustBuildTarget(cfg config.Config, target string) error {
	if target == "" {
		return nil
	}
	if cfg.Build.Mode == "package" {
		return fmt.Errorf("--target is not supported for Rust package mode")
	}
	if _, err := rustTargetTriple(target); err != nil {
		return err
	}
	if cfg.Build.Mode == "binary" && !contains(cfg.Build.Targets, target) {
		return fmt.Errorf("invalid --target %q: not present in build.targets", target)
	}
	return nil
}

func buildRust(cfg config.Config, target, outputOverride string, stderr io.Writer) ([]string, error) {
	if err := validateRustBuildTarget(cfg, target); err != nil {
		return nil, err
	}
	env := buildCommandEnv(cfg, stderr)
	project, err := cargoMetadata("Cargo.toml", env)
	if err != nil {
		return nil, err
	}
	packages, err := rustPackages(project, cfg.Build.Packages)
	if err != nil {
		return nil, err
	}
	var binary string
	if cfg.Build.Mode == "binary" {
		p, bin, err := rustBinaryPackage(project, cfg.Build.MainPackage)
		if err != nil {
			return nil, err
		}
		packages, binary = []cargoPackage{p}, bin
	}
	if cfg.Build.RunTests {
		if err := rustBuildTests(packages, env); err != nil {
			return nil, err
		}
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-rust-build-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	env = append(env, "CARGO_TARGET_DIR="+filepath.Join(dir, "target"))
	selection := rustPackageArgs(packages)
	if cfg.Build.Mode == "library" {
		args := append([]string{"build", "--locked"}, selection...)
		if target != "" {
			triple, _ := rustTargetTriple(target)
			args = append(args, "--target", triple)
		}
		_, err := cargoRun(args, env)
		return nil, err
	}
	outputDir := cfg.Build.OutputDir
	if outputOverride != "" {
		outputDir = outputOverride
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}
	var artifacts []string
	if cfg.Build.Mode == "package" {
		if _, err := cargoRun(append([]string{"package", "--locked"}, selection...), env); err != nil {
			return nil, err
		}
		for _, p := range packages {
			name := p.Name + "-" + p.Version + ".crate"
			dst := filepath.Join(outputDir, name)
			if err := copyFile(filepath.Join(dir, "target", "package", name), dst, 0o644); err != nil {
				return nil, fmt.Errorf("Cargo package artifact missing or unreadable: %w", err)
			}
			artifacts = append(artifacts, dst)
		}
	} else {
		targets := cfg.Build.Targets
		if target != "" {
			targets = []string{target}
		}
		for _, target := range targets {
			triple, err := rustTargetTriple(target)
			if err != nil {
				return nil, err
			}
			args := append([]string{"build", "--locked", "--release", "--bin", binary, "--target", triple, "--message-format", "json"}, selection...)
			out, err := cargoRun(args, env)
			if err != nil {
				return nil, fmt.Errorf("Rust target=%s triple=%s: ensure the Rust target and linker are installed\n%w", target, triple, err)
			}
			path, err := rustCompilerBinary(out, binary)
			if err != nil {
				return nil, err
			}
			built, err := packageRustBinary(cfg, targets, target, path, outputDir)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, built...)
		}
	}
	return rustChecksums(artifacts, outputDir, resolveChecksumPath(cfg.Build.ChecksumFile, outputOverride))
}

func rustBuildTests(packages []cargoPackage, env []string) error {
	// Query the active compiler so cross-build settings cannot make us execute foreign binaries.
	out, errText, err := runCommandCapture("rustc", []string{"-vV"}, env)
	if err != nil {
		return fmt.Errorf("rustc host detection failed: %w\n%s", err, errText)
	}
	host := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "host: ") {
			host = strings.TrimSpace(strings.TrimPrefix(line, "host: "))
		}
	}
	if host == "" {
		return fmt.Errorf("rustc did not report its host triple")
	}
	args := append([]string{"test", "--locked", "--no-fail-fast", "--target", host}, rustPackageArgs(packages)...)
	_, err = cargoRun(args, env)
	return err
}

func packageRustBinary(cfg config.Config, targets []string, target, binary, outputDir string) ([]string, error) {
	goos, _, _ := splitTarget(target)
	binBase := cfg.Project.Repo + "-" + target
	binName, archiveBin := binBase, cfg.Project.Repo
	if goos == "windows" {
		binName += ".exe"
		archiveBin += ".exe"
	}
	archiveBase := cfg.Project.Repo
	if cfg.Build.ArtifactTargetSuffix || len(targets) > 1 {
		archiveBase = binBase
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	subdir := ""
	if !cfg.Build.ArtifactTargetSuffix && len(targets) > 1 {
		subdir = target
	}
	dir := filepath.Join(outputDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	archive, executable := filepath.Join(dir, archiveBase+ext), filepath.Join(dir, binName)
	if err := packageBinaryArchive(binary, target, archive, archiveBin); err != nil {
		return nil, err
	}
	if err := copyFile(binary, executable, 0o755); err != nil {
		return nil, err
	}
	return []string{archive, executable}, nil
}

func rustChecksums(artifacts []string, outputDir, checksumPath string) ([]string, error) {
	sort.Strings(artifacts)
	checksumAbs, err := filepath.Abs(checksumPath)
	if err != nil {
		return nil, err
	}
	var manifest strings.Builder
	for _, artifact := range artifacts {
		abs, err := filepath.Abs(artifact)
		if err != nil {
			return nil, err
		}
		if abs == checksumAbs {
			return nil, fmt.Errorf("build.checksumFile conflicts with a Rust artifact")
		}
		sum, err := sha256File(artifact)
		if err != nil {
			return nil, err
		}
		name, err := filepath.Rel(outputDir, artifact)
		if err != nil {
			return nil, err
		}
		_, _ = fmt.Fprintf(&manifest, "%s  %s\n", sum, filepath.ToSlash(name))
	}
	if err := os.MkdirAll(filepath.Dir(checksumPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(checksumPath, []byte(manifest.String()), 0o644); err != nil {
		return nil, err
	}
	return append(artifacts, checksumPath), nil
}
