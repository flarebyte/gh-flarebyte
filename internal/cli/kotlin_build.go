// purpose: Verify Kotlin libraries and package fresh platform-independent JVM CLI distributions.
package cli

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func runKotlinBuild(cfg config.Config, target, output string, stdout, stderr io.Writer) Result {
	if target != "" {
		err := fmt.Errorf("--target is not supported for Kotlin JVM builds")
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitUsage, Err: err}
	}
	_, err := buildKotlin(cfg, target, output, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return Result{ExitCode: ExitBuildFailure, Err: err}
	}
	_, _ = fmt.Fprintf(stdout, "Build complete: Kotlin %s JVM build passed.\n", cfg.Build.Mode)
	return Result{ExitCode: ExitOK}
}

func buildKotlin(cfg config.Config, target, outputOverride string, stderr io.Writer) ([]string, error) {
	if target != "" {
		return nil, fmt.Errorf("--target is not supported for Kotlin JVM builds")
	}
	project, err := prepareKotlin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(project.Dir) }()
	tasks := []string{}
	if cfg.Build.RunTests {
		tasks = append(tasks, "test")
	}
	if cfg.Build.Mode == "library" {
		tasks = append(tasks, "assemble")
	} else {
		tasks = append(tasks, "distZip")
	}
	if err := project.run(buildCommandEnv(cfg, stderr), cfg.Build.Mode, tasks...); err != nil {
		return nil, err
	}
	if cfg.Build.RunTests {
		if _, err := project.tests(); err != nil {
			return nil, err
		}
	}
	if cfg.Build.Mode == "library" {
		return nil, nil
	}
	source := filepath.Join(project.Dir, "distribution", "application.zip")
	if err := validateKotlinDistribution(source); err != nil {
		return nil, err
	}
	version, err := findVersion(cfg.Release.VersionSource)
	if err != nil {
		return nil, err
	}
	// The repo name becomes an artifact basename, never a directory selector.
	if filepath.Base(cfg.Project.Repo) != cfg.Project.Repo || strings.ContainsAny(cfg.Project.Repo, `/\\`) {
		return nil, fmt.Errorf("invalid Kotlin artifact repository name")
	}
	output := cfg.Build.OutputDir
	if outputOverride != "" {
		output = outputOverride
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(output, cfg.Project.Repo+"-"+version+".zip")
	if err := copyFile(source, dest, 0o644); err != nil {
		return nil, err
	}
	return writeArtifactChecksums([]string{dest}, output, resolveChecksumPath(cfg.Build.ChecksumFile, outputOverride))
}

func validateKotlinDistribution(archivePath string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("Kotlin application distribution missing or invalid: %w", err)
	}
	defer func() { _ = archive.Close() }()
	launcher, jar := false, false
	for _, file := range archive.File {
		name := file.Name
		// ZIP names use forward slashes on every host. Reject Windows drive
		// paths, UNC names, and alternate data streams even when building on Unix.
		if strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00") || path.Clean(name) != strings.TrimSuffix(name, "/") || name == "." || name == ".." || strings.HasPrefix(name, "../") || file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe Kotlin distribution entry %q", name)
		}
		parts := strings.Split(name, "/")
		if len(parts) == 3 && !file.FileInfo().IsDir() {
			if parts[1] == "bin" && !strings.HasSuffix(name, ".bat") && file.Mode().Perm()&0o111 != 0 {
				launcher = true
			}
			if parts[1] == "lib" && strings.HasSuffix(name, ".jar") {
				jar = true
			}
		}
	}
	if !launcher || !jar {
		return fmt.Errorf("Kotlin distribution requires executable bin launch scripts and lib JARs")
	}
	return nil
}
