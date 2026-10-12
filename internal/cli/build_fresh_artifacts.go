// purpose: Package target executables and checksum only fresh Rust and Swift build assets.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func packageTargetBinary(cfg config.Config, targets []string, target, binary, outputDir string) ([]string, error) {
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
	if err := os.Chmod(executable, 0o755); err != nil {
		return nil, err
	}
	return []string{archive, executable}, nil
}

func writeArtifactChecksums(artifacts []string, outputDir, checksumPath string) ([]string, error) {
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
			return nil, fmt.Errorf("build.checksumFile conflicts with a build artifact")
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
