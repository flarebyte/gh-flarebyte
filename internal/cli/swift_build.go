// purpose: Verify Swift libraries and package native SwiftPM executable products.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func swiftHostTarget(env []string) (string, error) {
	out, _, err := swiftRun([]string{"-print-target-info"}, env)
	if err != nil {
		return "", err
	}
	var info struct {
		Target struct {
			Triple string `json:"triple"`
		} `json:"target"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return "", fmt.Errorf("invalid Swift target info: %w", err)
	}
	triple := info.Target.Triple
	arch := ""
	if strings.HasPrefix(triple, "arm64-") || strings.HasPrefix(triple, "aarch64-") {
		arch = "arm64"
	}
	if strings.HasPrefix(triple, "x86_64-") {
		arch = "amd64"
	}
	osName := ""
	if strings.Contains(triple, "-apple-macosx") {
		osName = "darwin"
	}
	if strings.Contains(triple, "-unknown-linux-gnu") {
		osName = "linux"
	}
	if arch == "" || osName == "" {
		return "", fmt.Errorf("unsupported Swift host triple %q: initial support covers macOS and GNU/Linux on amd64/arm64", triple)
	}
	return osName + "-" + arch, nil
}

func validateSwiftTargets(cfg config.Config, target, host string) error {
	targets := []string{target}
	if target == "" {
		targets = nil
		if cfg.Build.Mode == "binary" {
			targets = cfg.Build.Targets
		}
	}
	if cfg.Build.Mode == "binary" && target != "" && !contains(cfg.Build.Targets, target) {
		return fmt.Errorf("invalid --target %q: not present in build.targets", target)
	}
	for _, candidate := range targets {
		if candidate != host {
			return fmt.Errorf("Swift target %q is unsupported by the active host %s; cross-compilation is not supported yet", candidate, host)
		}
	}
	return nil
}

func runSwiftBuild(cfg config.Config, target, output string, stdout, stderr io.Writer) Result {
	env := buildCommandEnv(cfg, stderr)
	host, err := swiftHostTarget(env)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitBuildFailure, Err: err}
	}
	err = validateSwiftTargets(cfg, target, host)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitUsage, Err: err}
	}
	assets, err := buildSwift(cfg, target, output, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitBuildFailure, Err: err}
	}
	if len(assets) == 0 {
		_, _ = fmt.Fprintln(stdout, "Build complete: Swift library compile verification passed.")
	} else {
		_, _ = fmt.Fprintf(stdout, "Build complete: Swift binary artifacts with checksums in %s.\n", assets[len(assets)-1])
	}
	return Result{ExitCode: ExitOK}
}

func buildSwift(cfg config.Config, target, outputOverride string, stderr io.Writer) ([]string, error) {
	env := buildCommandEnv(cfg, stderr)
	project, err := loadSwiftProject(env)
	if err != nil {
		return nil, err
	}
	host, err := swiftHostTarget(env)
	if err != nil {
		return nil, err
	}
	if err := validateSwiftTargets(cfg, target, host); err != nil {
		return nil, err
	}
	var selected []string
	var product string
	if cfg.Build.Mode == "binary" {
		product, err = swiftExecutable(project, cfg.Build.MainPackage)
	} else {
		selected, err = swiftSelectedTargets(project, cfg.Build.Packages)
	}
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "gh-flarebyte-swift-build-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	common := []string{"--force-resolved-versions", "--scratch-path", dir}
	if cfg.Build.RunTests {
		if _, _, err := swiftRun(append([]string{"test"}, common...), env); err != nil {
			return nil, err
		}
	}
	args := append([]string{"build"}, common...)
	if cfg.Build.Mode == "library" {
		if len(selected) == 0 {
			_, _, err = swiftRun(args, env)
		} else {
			for _, name := range selected {
				if _, _, err = swiftRun(append(append([]string(nil), args...), "--target", name), env); err != nil {
					break
				}
			}
		}
		return nil, err
	}
	args = append(args, "--configuration", "release", "--product", product)
	if _, _, err = swiftRun(args, env); err != nil {
		return nil, err
	}
	binDir, _, err := swiftRun(append(args, "--show-bin-path"), env)
	if err != nil {
		return nil, err
	}
	binDir = strings.TrimSpace(binDir)
	if !filepath.IsAbs(binDir) {
		return nil, fmt.Errorf("SwiftPM did not report an absolute executable directory")
	}
	executable := filepath.Join(binDir, product)
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("SwiftPM executable %q is missing", product)
	}
	// Resources need a bundle-aware artifact contract; do not ship an incomplete executable.
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".bundle") || strings.HasSuffix(entry.Name(), ".resources") || strings.HasSuffix(entry.Name(), ".dylib") || strings.HasSuffix(entry.Name(), ".so") {
			return nil, fmt.Errorf("Swift binary packaging with resource bundles or package-built shared libraries is not supported yet")
		}
	}
	outputDir := cfg.Build.OutputDir
	if outputOverride != "" {
		outputDir = outputOverride
	}
	assets, err := packageTargetBinary(cfg, []string{host}, host, executable, outputDir)
	if err != nil {
		return nil, err
	}
	return writeArtifactChecksums(assets, outputDir, resolveChecksumPath(cfg.Build.ChecksumFile, outputOverride))
}
