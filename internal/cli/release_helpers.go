package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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

func listReleaseArtifacts(artifactDir string, includeChecksums bool, repoName string, artifactTargetSuffix bool) ([]string, error) {
	entries, err := os.ReadDir(artifactDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read release.artifactDir %s: %w", artifactDir, err)
	}
	var artifactPattern *regexp.Regexp
	if artifactTargetSuffix {
		artifactPattern = regexp.MustCompile("^" + regexp.QuoteMeta(repoName) + `-(linux|darwin|windows)-(amd64|arm64)\.(tar\.gz|zip)$`)
	} else {
		artifactPattern = regexp.MustCompile("^" + regexp.QuoteMeta(repoName) + `\.(tar\.gz|zip)$`)
	}
	artifacts := make([]string, 0)
	for _, e := range entries {
		entryPath := filepath.Join(artifactDir, e.Name())
		if e.IsDir() {
			_ = filepath.WalkDir(entryPath, func(p string, d os.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() {
					return walkErr
				}
				name := path.Base(p)
				if artifactPattern.MatchString(name) {
					artifacts = append(artifacts, p)
				}
				if includeChecksums && (name == "checksums.txt" || strings.HasSuffix(name, "checksums.txt")) {
					artifacts = append(artifacts, p)
				}
				return nil
			})
			continue
		}
		name := e.Name()
		if artifactPattern.MatchString(name) {
			artifacts = append(artifacts, entryPath)
		}
		if includeChecksums && (name == "checksums.txt" || strings.HasSuffix(name, "checksums.txt")) {
			artifacts = append(artifacts, entryPath)
		}
	}
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
