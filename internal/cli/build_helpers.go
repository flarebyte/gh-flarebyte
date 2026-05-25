// purpose: Provide reusable build primitives for compiling, packaging, hashing, and build-metadata hydration.
// responsibilities: Parse build args; compile per-target binaries; package archives; compute checksums; copy artifacts; populate ldflag metadata inputs.
// architecture notes: External process interactions are exposed through package variables for deterministic test stubbing while keeping production behavior shell-based.
package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/buildinfo"
	"github.com/flarebyte/gh-flarebyte/internal/config"
)

var (
	resolveBuildVersion = resolveVersionFromSource
	currentTimeUTC      = func() time.Time { return time.Now().UTC() }
	currentGoVersion    = runtime.Version
	goBuildPackages     = runGoBuildPackages
	dartBuildPackages   = runDartBuildPackages
	readGitOutput       = func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	detectCGODependencies = findCGODependencies
)

type goBuildInvocationError struct {
	Command string
	Env     []string
	Stderr  string
	Cause   error
}

func (e *goBuildInvocationError) Error() string {
	return e.Cause.Error()
}

func goBuildError(err error, command string, env []string, stderr string) error {
	if err == nil {
		return nil
	}
	return &goBuildInvocationError{
		Command: command,
		Env:     env,
		Stderr:  stderr,
		Cause:   commandError(err, stderr),
	}
}

func buildGoEnv(goos string, goarch string, cgo config.CGOConfig) []string {
	env := append(os.Environ(), "CGO_ENABLED=0")
	if cgo.Enabled {
		env = append(os.Environ(), "CGO_ENABLED=1")
	}
	if goos != "" && goarch != "" {
		env = append(env, "GOOS="+goos, "GOARCH="+goarch)
	}
	if cgo.CC != "" {
		env = append(env, "CC="+cgo.CC)
	}
	if cgo.CXX != "" {
		env = append(env, "CXX="+cgo.CXX)
	}
	return env
}

func findCGODependencies(goos string, goarch string, packages []string) ([]string, error) {
	env := append(os.Environ(), "CGO_ENABLED=1")
	if goos != "" && goarch != "" {
		env = append(env, "GOOS="+goos, "GOARCH="+goarch)
	}
	args := append([]string{"list", "-deps", "-f", "{{if and .CgoFiles (not .Standard)}}{{.ImportPath}}{{end}}"}, packages...)
	cmd := exec.Command("go", args...)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	seen := map[string]struct{}{}
	cgoPkgs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		cgoPkgs = append(cgoPkgs, line)
	}
	return cgoPkgs, nil
}

func runGoBuildPackages(goos string, goarch string, packages []string, runTests bool, cgo config.CGOConfig) error {
	env := buildGoEnv(goos, goarch, cgo)
	buildArgs := append([]string{"build"}, packages...)
	buildCmd := exec.Command("go", buildArgs...)
	buildCmd.Env = env
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr
	if err := buildCmd.Run(); err != nil {
		return goBuildError(err, "go "+strings.Join(buildArgs, " "), env, buildStderr.String())
	}
	if !runTests {
		return nil
	}
	testArgs := append([]string{"test"}, packages...)
	testCmd := exec.Command("go", testArgs...)
	testCmd.Env = env
	var testStderr bytes.Buffer
	testCmd.Stderr = &testStderr
	if err := testCmd.Run(); err != nil {
		return goBuildError(err, "go "+strings.Join(testArgs, " "), env, testStderr.String())
	}
	return nil
}

func runDartBuildPackages(runTests bool) error {
	pubGetCmd := exec.Command("dart", "pub", "get")
	var pubGetStderr bytes.Buffer
	pubGetCmd.Stderr = &pubGetStderr
	if err := pubGetCmd.Run(); err != nil {
		return commandError(err, pubGetStderr.String())
	}

	analyzeCmd := exec.Command("dart", "analyze")
	var analyzeStderr bytes.Buffer
	analyzeCmd.Stderr = &analyzeStderr
	if err := analyzeCmd.Run(); err != nil {
		return commandError(err, analyzeStderr.String())
	}

	if !runTests {
		return nil
	}
	testCmd := exec.Command("dart", "test")
	var testStderr bytes.Buffer
	testCmd.Stderr = &testStderr
	if err := testCmd.Run(); err != nil {
		return commandError(err, testStderr.String())
	}
	return nil
}

func goBuildTargetBinary(target string, outputPath string, mainPackage string, cgo config.CGOConfig) error {
	goos, goarch, err := splitTarget(target)
	if err != nil {
		return err
	}
	args := []string{
		"build",
		"-ldflags",
		fmt.Sprintf("-X github.com/flarebyte/gh-flarebyte/internal/buildinfo.Version=%s -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.CommitID=%s -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.Date=%s -X github.com/flarebyte/gh-flarebyte/internal/buildinfo.GoVersion=%s",
			buildinfo.Version, buildinfo.CommitID, buildinfo.Date, buildinfo.GoVersion),
		"-o",
		outputPath,
		mainPackage,
	}
	cmd := exec.Command("go", args...)
	cmd.Env = buildGoEnv(goos, goarch, cgo)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return goBuildError(err, "go "+strings.Join(args, " "), cmd.Env, stderr.String())
	}
	return nil
}

func hydrateBuildInfo(versionSource string) {
	if version, err := resolveBuildVersion(versionSource); err == nil && version != "" {
		buildinfo.Version = version
	}
	if commitID, err := readGitOutput("rev-parse", "--short=12", "HEAD"); err == nil && commitID != "" {
		buildinfo.CommitID = commitID
	}
	buildinfo.Date = currentTimeUTC().Format(time.RFC3339)
	buildinfo.GoVersion = currentGoVersion()
}

func splitTarget(target string) (goos, goarch string, err error) {
	parts := strings.Split(target, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid target %q: expected os-arch", target)
	}
	return parts[0], parts[1], nil
}

func packageBinaryArchive(binaryPath string, target string, artifactPath string, archiveBinaryName string) error {
	goos, _, err := splitTarget(target)
	if err != nil {
		return err
	}
	if goos == "windows" {
		return packageZip(binaryPath, artifactPath, archiveBinaryName)
	}
	return packageTarGz(binaryPath, artifactPath, archiveBinaryName)
}

func packageTarGz(binaryPath string, artifactPath string, archiveBinaryName string) error {
	src, err := os.Open(binaryPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	out, err := os.Create(artifactPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	gw := gzip.NewWriter(out)
	defer func() { _ = gw.Close() }()
	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()
	hdr := &tar.Header{
		Name:    archiveBinaryName,
		Mode:    0o755,
		Size:    info.Size(),
		ModTime: zeroTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.Copy(tw, src); err != nil {
		return err
	}
	return nil
}

func packageZip(binaryPath string, artifactPath string, archiveBinaryName string) error {
	content, err := os.ReadFile(binaryPath)
	if err != nil {
		return err
	}
	out, err := os.Create(artifactPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	zw := zip.NewWriter(out)
	defer func() { _ = zw.Close() }()
	hdr := &zip.FileHeader{
		Name:   path.Base(archiveBinaryName),
		Method: zip.Deflate,
	}
	hdr.SetMode(os.FileMode(0o755))
	hdr.Modified = zeroTime()
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	if _, err := w.Write(content); err != nil {
		return err
	}
	return nil
}

func zeroTime() time.Time {
	return time.Unix(0, 0).UTC()
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

func resolveChecksumPath(configPath string, outputOverride string) string {
	if outputOverride == "" {
		return configPath
	}
	return filepath.Join(outputOverride, filepath.Base(configPath))
}

func copyFile(srcPath string, dstPath string, mode os.FileMode) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, data, mode)
}
