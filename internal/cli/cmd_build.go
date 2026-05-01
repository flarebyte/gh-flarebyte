package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func handleBuild(args []string, stdout, stderr io.Writer) Result {
	targetFilter, outputDirOverride, err := parseBuildArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return *usage
	}
	if cfg.Build.Language != "go" {
		err := fmt.Errorf("build.language %q is not supported yet. Supported values: go", cfg.Build.Language)
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	targets := cfg.Build.Targets
	if targetFilter != "" {
		if !contains(targets, targetFilter) {
			err := fmt.Errorf("invalid --target %q: not present in build.targets", targetFilter)
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		targets = []string{targetFilter}
	}
	outputDir := cfg.Build.OutputDir
	if outputDirOverride != "" {
		outputDir = outputDirOverride
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	tmpDir := filepath.Join(outputDir, ".tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	type artifactDigest struct {
		Name string
		SHA  string
	}
	digests := make([]artifactDigest, 0, len(targets))
	for _, target := range targets {
		goos, goarch, err := splitTarget(target)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		binBase := fmt.Sprintf("%s-%s-%s", cfg.Project.Repo, goos, goarch)
		binName := binBase
		if goos == "windows" {
			binName += ".exe"
		}
		binPath := filepath.Join(tmpDir, binName)
		archiveBinaryName := cfg.Project.Repo
		if goos == "windows" {
			archiveBinaryName += ".exe"
		}
		if err := buildTargetBinary(target, binPath); err != nil {
			msg := fmt.Sprintf("Build failed for %s during go build. Re-run with --target %s to isolate the failure.", target, target)
			_, _ = fmt.Fprintln(stderr, msg)
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		artifactBase := cfg.Project.Repo
		if cfg.Build.ArtifactTargetSuffix || len(targets) > 1 {
			artifactBase = binBase
		}
		artifactName := artifactBase + ".tar.gz"
		if goos == "windows" {
			artifactName = artifactBase + ".zip"
		}
		artifactRelPath := artifactName
		if !cfg.Build.ArtifactTargetSuffix && len(targets) > 1 {
			artifactRelPath = filepath.Join(target, artifactName)
		}
		artifactPath := filepath.Join(outputDir, artifactRelPath)
		if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitFailure, Err: err}
		}
		if err := packageBinary(binPath, target, artifactPath, archiveBinaryName); err != nil {
			_, _ = fmt.Fprintf(stderr, "Build failed for %s during packaging. Re-run with --target %s to isolate the failure.\n", target, target)
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		// Also publish a GH-extension installable binary asset name.
		releaseBinName := binBase
		if goos == "windows" {
			releaseBinName += ".exe"
		}
		releaseBinRelPath := releaseBinName
		if !cfg.Build.ArtifactTargetSuffix && len(targets) > 1 {
			releaseBinRelPath = filepath.Join(target, releaseBinName)
		}
		releaseBinPath := filepath.Join(outputDir, releaseBinRelPath)
		if err := copyFile(binPath, releaseBinPath, 0o755); err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		sum, err := sha256File(artifactPath)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		digests = append(digests, artifactDigest{Name: artifactRelPath, SHA: sum})
		binSum, err := sha256File(releaseBinPath)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		digests = append(digests, artifactDigest{Name: releaseBinRelPath, SHA: binSum})
	}
	sort.Slice(digests, func(i, j int) bool { return digests[i].Name < digests[j].Name })
	checksumPath := resolveChecksumPath(cfg.Build.ChecksumFile, outputDirOverride)
	if err := os.MkdirAll(filepath.Dir(checksumPath), 0o755); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	var b strings.Builder
	for _, d := range digests {
		b.WriteString(d.SHA)
		b.WriteString("  ")
		b.WriteString(d.Name)
		b.WriteString("\n")
	}
	if err := os.WriteFile(checksumPath, []byte(b.String()), 0o644); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitFailure, Err: err}
	}
	_, _ = fmt.Fprintf(stdout, "Build complete: %d targets written to %s/ with checksums in %s.\n", len(targets), outputDir, checksumPath)
	return Result{ExitCode: ExitOK}
}
