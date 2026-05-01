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
		if err := buildTargetBinary(target, binPath); err != nil {
			msg := fmt.Sprintf("Build failed for %s during go build. Re-run with --target %s to isolate the failure.", target, target)
			_, _ = fmt.Fprintln(stderr, msg)
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		artifactBase := cfg.Project.Repo
		if cfg.Build.ArtifactTargetSuffix {
			artifactBase = binBase
		}
		artifactName := artifactBase + ".tar.gz"
		if goos == "windows" {
			artifactName = artifactBase + ".zip"
		}
		artifactPath := filepath.Join(outputDir, artifactName)
		if err := packageBinary(binPath, target, artifactPath); err != nil {
			_, _ = fmt.Fprintf(stderr, "Build failed for %s during packaging. Re-run with --target %s to isolate the failure.\n", target, target)
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		sum, err := sha256File(artifactPath)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitBuildFailure, Err: err}
		}
		digests = append(digests, artifactDigest{Name: artifactName, SHA: sum})
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
