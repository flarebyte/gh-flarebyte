package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
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
)

var (
	resolveBuildVersion = resolveVersionFromSource
	currentTimeUTC      = func() time.Time { return time.Now().UTC() }
	currentGoVersion    = runtime.Version
	readGitOutput       = func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
)

func parseBuildArgs(args []string) (target string, outputDir string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--target":
			if i+1 >= len(args) {
				return "", "", errors.New("invalid invocation: --target requires os-arch")
			}
			target = args[i+1]
			i++
		case "--output-dir":
			if i+1 >= len(args) {
				return "", "", errors.New("invalid invocation: --output-dir requires a path")
			}
			outputDir = args[i+1]
			i++
		default:
			return "", "", fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return target, outputDir, nil
}

func goBuildTargetBinary(target string, outputPath string) error {
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
		"./cmd/gh-flarebyte",
	}
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return commandError(err, stderr.String())
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
