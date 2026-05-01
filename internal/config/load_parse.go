package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func ResolvePath(path string) (string, error) {
	p := path
	if p == "" {
		p = DefaultPath
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func Load(path string) (Config, error) {
	var cfg Config
	absPath, err := ResolvePath(path)
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return cfg, fmt.Errorf("cannot read config %s: %w", absPath, err)
	}

	cfg, err = parseCueConfig(string(data))
	if err != nil {
		return cfg, fmt.Errorf("cannot parse cue config %s: %w", absPath, err)
	}
	if err := Validate(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func parseCueConfig(raw string) (Config, error) {
	var cfg Config
	var err error
	buildBlock, err := extractObjectBlock(raw, "build")
	if err != nil {
		return cfg, err
	}
	releaseBlock, err := extractObjectBlock(raw, "release")
	if err != nil {
		return cfg, err
	}
	cfg.Project.Org, err = extractStringField(raw, "org")
	if err != nil {
		return cfg, err
	}
	cfg.Project.Repo, err = extractStringField(raw, "repo")
	if err != nil {
		return cfg, err
	}
	cfg.Sync.Mode, err = extractStringField(raw, "mode")
	if err != nil {
		return cfg, err
	}
	cfg.Build.Language, err = extractStringField(raw, "language")
	if err != nil {
		return cfg, err
	}
	cfg.Build.OutputDir, err = extractStringField(raw, "outputDir")
	if err != nil {
		return cfg, err
	}
	cfg.Build.ChecksumFile, err = extractStringField(raw, "checksumFile")
	if err != nil {
		return cfg, err
	}
	cfg.Build.Targets, err = extractStringListBlock(raw, "targets")
	if err != nil {
		return cfg, err
	}
	cfg.Build.ArtifactTargetSuffix = extractOptionalBoolField(buildBlock, "artifactTargetSuffix", true)
	cfg.Repository.Description, err = extractStringField(raw, "description")
	if err != nil {
		return cfg, err
	}
	cfg.Repository.DefaultBranch, err = extractStringField(raw, "defaultBranch")
	if err != nil {
		return cfg, err
	}
	cfg.Repository.Homepage, err = extractStringField(raw, "homepage")
	if err != nil {
		return cfg, err
	}
	cfg.Repository.Visibility, err = extractStringField(raw, "visibility")
	if err != nil {
		return cfg, err
	}
	template, err := extractBoolField(raw, "template")
	if err != nil {
		return cfg, err
	}
	cfg.Repository.Template = template
	cfg.Repository.Topics, err = extractStringListBlock(raw, "topics")
	if err != nil {
		return cfg, err
	}
	cfg.Repository.Labels, err = extractLabels(raw)
	if err != nil {
		return cfg, err
	}
	cfg.Release.VersionSource, err = extractStringField(raw, "versionSource")
	if err != nil {
		return cfg, err
	}
	cfg.Release.TagPrefix, err = extractStringField(raw, "tagPrefix")
	if err != nil {
		return cfg, err
	}
	cfg.Release.NotesMode, err = extractStringField(raw, "notesMode")
	if err != nil {
		return cfg, err
	}
	cfg.Release.ReleaseNotesFilePath = extractOptionalStringField(raw, "releaseNotesFilePath")
	cfg.Release.ArtifactDir, err = extractStringField(raw, "artifactDir")
	if err != nil {
		return cfg, err
	}
	cfg.Release.ArtifactTargetSuffix = extractOptionalBoolField(releaseBlock, "artifactTargetSuffix", true)
	includeChecksums, err := extractBoolField(raw, "includeChecksums")
	if err != nil {
		return cfg, err
	}
	cfg.Release.IncludeChecksums = includeChecksums
	return cfg, nil
}

func extractObjectBlock(raw, field string) (string, error) {
	open := strings.Index(raw, field+": {")
	if open == -1 {
		return "", fmt.Errorf("missing required object field %s", field)
	}
	start := open + len(field+": {")
	end := strings.Index(raw[start:], "\n}")
	if end == -1 {
		return "", fmt.Errorf("malformed object field %s", field)
	}
	return raw[start : start+end], nil
}

func extractStringField(raw, field string) (string, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*"([^"]+)"`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return "", fmt.Errorf("missing required string field %s", field)
	}
	return m[1], nil
}

func extractOptionalStringField(raw, field string) string {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*"([^"]+)"`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func extractBoolField(raw, field string) (bool, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*(true|false)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return false, fmt.Errorf("missing required bool field %s", field)
	}
	return m[1] == "true", nil
}

func extractOptionalBoolField(raw, field string, fallback bool) bool {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*(true|false)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return fallback
	}
	return m[1] == "true"
}

func extractStringListBlock(raw, field string) ([]string, error) {
	open := strings.Index(raw, field+": [")
	if open == -1 {
		return nil, fmt.Errorf("missing required list field %s", field)
	}
	start := open + len(field+": [")
	end := strings.Index(raw[start:], "]")
	if end == -1 {
		return nil, fmt.Errorf("malformed list field %s", field)
	}
	block := raw[start : start+end]
	matches := quotedStringPattern.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("list field %s has no entries", field)
	}
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values, nil
}

func extractLabels(raw string) ([]LabelConfig, error) {
	open := strings.Index(raw, "labels: [")
	if open == -1 {
		return nil, errors.New("missing required list field labels")
	}
	start := open + len("labels: [")
	end := strings.Index(raw[start:], "]")
	if end == -1 {
		return nil, errors.New("malformed list field labels")
	}
	block := raw[start : start+end]
	matches := labelObjectPattern.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		return nil, errors.New("list field labels has no entries")
	}
	labels := make([]LabelConfig, 0, len(matches))
	for _, match := range matches {
		item := match[1]
		name, err := extractStringField(item, "name")
		if err != nil {
			return nil, fmt.Errorf("invalid repository.labels entry: %w", err)
		}
		color, err := extractStringField(item, "color")
		if err != nil {
			return nil, fmt.Errorf("invalid repository.labels entry: %w", err)
		}
		description := extractOptionalStringField(item, "description")
		labels = append(labels, LabelConfig{
			Name:        name,
			Color:       color,
			Description: description,
		})
	}
	return labels, nil
}
