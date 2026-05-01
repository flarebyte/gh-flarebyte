package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/buildinfo"
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

func parseReleaseArgs(args []string) (draft bool, notesFile string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--draft":
			draft = true
		case "--notes-file":
			if i+1 >= len(args) {
				return false, "", errors.New("invalid invocation: --notes-file requires a path")
			}
			notesFile = args[i+1]
			i++
		default:
			return false, "", fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return draft, notesFile, nil
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
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func splitTarget(target string) (goos, goarch string, err error) {
	parts := strings.Split(target, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid target %q: expected os-arch", target)
	}
	return parts[0], parts[1], nil
}

func packageBinaryArchive(binaryPath string, target string, artifactPath string) error {
	goos, _, err := splitTarget(target)
	if err != nil {
		return err
	}
	if goos == "windows" {
		return packageZip(binaryPath, artifactPath)
	}
	return packageTarGz(binaryPath, artifactPath)
}

func packageTarGz(binaryPath string, artifactPath string) error {
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
		Name:    filepath.Base(binaryPath),
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

func packageZip(binaryPath string, artifactPath string) error {
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
		Name:   path.Base(binaryPath),
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

func resolveVersionFromSource(sourcePath string) (string, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("cannot read release.versionSource %s: %w", sourcePath, err)
	}
	ext := strings.ToLower(filepath.Ext(sourcePath))
	switch ext {
	case ".json":
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			return "", fmt.Errorf("cannot parse release.versionSource JSON %s: %w", sourcePath, err)
		}
		v, ok := payload["version"].(string)
		if !ok || strings.TrimSpace(v) == "" {
			return "", errors.New("invalid release.versionSource: top-level string field version is required")
		}
		if !isSemver(v) {
			return "", fmt.Errorf("invalid version %q: semantic version required", v)
		}
		return v, nil
	case ".yaml", ".yml":
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "version:") {
				v := strings.TrimSpace(strings.TrimPrefix(trim, "version:"))
				v = strings.Trim(v, `"'`)
				if v == "" {
					return "", errors.New("invalid release.versionSource: top-level string field version is required")
				}
				if !isSemver(v) {
					return "", fmt.Errorf("invalid version %q: semantic version required", v)
				}
				return v, nil
			}
		}
		return "", errors.New("invalid release.versionSource: top-level string field version is required")
	default:
		return "", fmt.Errorf("unsupported release.versionSource format %q: expected YAML or JSON file", ext)
	}
}

func isSemver(v string) bool {
	re := `^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`
	return regexp.MustCompile(re).MatchString(v)
}

func listReleaseArtifacts(artifactDir string, includeChecksums bool) ([]string, error) {
	entries, err := os.ReadDir(artifactDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read release.artifactDir %s: %w", artifactDir, err)
	}
	artifacts := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip") {
			artifacts = append(artifacts, filepath.Join(artifactDir, name))
		}
		if includeChecksums && (name == "checksums.txt" || strings.HasSuffix(name, "checksums.txt")) {
			artifacts = append(artifacts, filepath.Join(artifactDir, name))
		}
	}
	// deterministic order
	sort.Strings(artifacts)
	if len(artifacts) == 0 {
		return nil, fmt.Errorf("no release artifacts found in %s", artifactDir)
	}
	return artifacts, nil
}

func ghTagExists(tag string) (bool, error) {
	if os.Getenv("GH_FLAREBYTE_FAKE_RELEASE") == "1" {
		return false, nil
	}
	cmd := exec.Command("gh", "release", "view", tag)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	msg := strings.TrimSpace(stderr.String())
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "not found") || strings.Contains(lower, "http 404") {
		return false, nil
	}
	if msg == "" {
		msg = err.Error()
	}
	return false, errors.New(msg)
}

func ghCreateRelease(tag string, artifacts []string, notesMode string, notesFile string, draft bool) error {
	if os.Getenv("GH_FLAREBYTE_FAKE_RELEASE") == "1" {
		return nil
	}
	args := []string{"release", "create", tag}
	args = append(args, artifacts...)
	switch notesMode {
	case "generate-notes", "notes-from-tag":
		args = append(args, "--generate-notes")
	case "notes-file":
		args = append(args, "--notes-file", notesFile)
	}
	if draft {
		args = append(args, "--draft")
	}
	return runGH(args...)
}

func ternary(cond bool, yes string, no string) string {
	if cond {
		return yes
	}
	return no
}
