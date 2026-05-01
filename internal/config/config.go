package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultPath = ".gh-flarebyte.cue"

type Config struct {
	Project    ProjectConfig    `json:"project"`
	Sync       SyncConfig       `json:"sync"`
	Repository RepositoryConfig `json:"repository"`
	Build      BuildConfig      `json:"build"`
	Release    ReleaseConfigRaw `json:"release"`
}

type ProjectConfig struct {
	Org  string `json:"org"`
	Repo string `json:"repo"`
}

type SyncConfig struct {
	Mode          string   `json:"mode"`
	ManagedFields []string `json:"managedFields"`
}

type RepositoryConfig struct {
	Description   string        `json:"description"`
	DefaultBranch string        `json:"defaultBranch"`
	Homepage      string        `json:"homepage"`
	Visibility    string        `json:"visibility"`
	Template      bool          `json:"template"`
	Topics        []string      `json:"topics"`
	Labels        []LabelConfig `json:"labels"`
}

type LabelConfig struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type BuildConfig struct {
	Language     string   `json:"language"`
	OutputDir    string   `json:"outputDir"`
	ChecksumFile string   `json:"checksumFile"`
	Targets      []string `json:"targets"`
}

type ReleaseConfigRaw struct {
	VersionSource        string `json:"versionSource"`
	TagPrefix            string `json:"tagPrefix"`
	NotesMode            string `json:"notesMode"`
	ReleaseNotesFilePath string `json:"releaseNotesFilePath,omitempty"`
	ArtifactDir          string `json:"artifactDir"`
	IncludeChecksums     bool   `json:"includeChecksums"`
}

var targetPattern = regexp.MustCompile(`^(linux|darwin|windows)-(amd64|arm64)$`)
var quotedStringPattern = regexp.MustCompile(`"([^"]+)"`)
var labelObjectPattern = regexp.MustCompile(`(?s)\{([^{}]*)\}`)

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
	includeChecksums, err := extractBoolField(raw, "includeChecksums")
	if err != nil {
		return cfg, err
	}
	cfg.Release.IncludeChecksums = includeChecksums
	return cfg, nil
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

func Validate(cfg Config) error {
	if cfg.Project.Org == "" {
		return errors.New("invalid project.org: value is required")
	}
	if cfg.Project.Repo == "" {
		return errors.New("invalid project.repo: value is required")
	}
	if cfg.Repository.DefaultBranch == "" {
		return errors.New("invalid repository.defaultBranch: value is required")
	}
	switch cfg.Repository.Visibility {
	case "public", "private", "internal":
	default:
		return fmt.Errorf("invalid repository.visibility %q: expected public, private, or internal", cfg.Repository.Visibility)
	}
	if len(cfg.Repository.Topics) == 0 {
		return errors.New("invalid repository.topics: at least one topic is required")
	}
	if len(cfg.Repository.Labels) == 0 {
		return errors.New("invalid repository.labels: at least one label is required")
	}
	for _, label := range cfg.Repository.Labels {
		if label.Name == "" {
			return errors.New("invalid repository.labels.name: value is required")
		}
		if label.Color == "" {
			return fmt.Errorf("invalid repository.labels[%s].color: value is required", label.Name)
		}
	}
	if cfg.Sync.Mode != "push" {
		return fmt.Errorf("invalid sync.mode %q: expected push", cfg.Sync.Mode)
	}
	switch cfg.Build.Language {
	case "go", "dart":
	default:
		return fmt.Errorf("invalid build.language %q: expected go or dart", cfg.Build.Language)
	}
	if cfg.Build.OutputDir == "" {
		return errors.New("invalid build.outputDir: value is required")
	}
	if cfg.Build.ChecksumFile == "" {
		return errors.New("invalid build.checksumFile: value is required")
	}
	if len(cfg.Build.Targets) == 0 {
		return errors.New("invalid build.targets: at least one target is required")
	}
	for _, target := range cfg.Build.Targets {
		if !targetPattern.MatchString(target) {
			return fmt.Errorf("invalid build.targets entry %q: expected os-arch format such as linux-amd64 or windows-amd64", target)
		}
	}

	switch cfg.Release.NotesMode {
	case "generate-notes", "notes-from-tag":
	case "notes-file":
		if cfg.Release.ReleaseNotesFilePath == "" {
			return errors.New("invalid release.releaseNotesFilePath: value is required when release.notesMode is notes-file")
		}
	default:
		return fmt.Errorf("invalid release.notesMode %q: expected generate-notes, notes-from-tag, or notes-file", cfg.Release.NotesMode)
	}
	if cfg.Release.VersionSource == "" {
		return errors.New("invalid release.versionSource: value is required")
	}
	if cfg.Release.TagPrefix == "" {
		return errors.New("invalid release.tagPrefix: value is required")
	}
	if cfg.Release.ArtifactDir == "" {
		return errors.New("invalid release.artifactDir: value is required")
	}
	return nil
}
