// purpose: Load `.gh-flarebyte.cue` files and parse required config fields into strongly typed runtime structures.
// responsibilities: Resolve config path; read config bytes; extract scalar/list/object fields; assemble Config; invoke validation.
// architecture notes: Parsing is regex/string-block based (not full CUE evaluation) to keep the CLI self-contained and deterministic for the constrained config dialect it supports.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
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
	if err := assertOnlyAllowedFields("top-level", raw, map[string]struct{}{
		"project":    {},
		"sync":       {},
		"repository": {},
		"go":         {},
		"devOutput":  {},
		"coverage":   {},
		"build":      {},
		"release":    {},
	}); err != nil {
		return cfg, err
	}
	projectBlock, err := extractObjectBlock(raw, "project")
	if err != nil {
		return cfg, err
	}
	if err := assertOnlyAllowedFields("project", projectBlock, map[string]struct{}{
		"org":  {},
		"repo": {},
	}); err != nil {
		return cfg, err
	}
	syncBlock, err := extractObjectBlock(raw, "sync")
	if err != nil {
		return cfg, err
	}
	if err := assertOnlyAllowedFields("sync", syncBlock, map[string]struct{}{
		"mode": {},
	}); err != nil {
		return cfg, err
	}
	repositoryBlock, err := extractObjectBlock(raw, "repository")
	if err != nil {
		return cfg, err
	}
	if err := assertOnlyAllowedFields("repository", repositoryBlock, map[string]struct{}{
		"description":   {},
		"defaultBranch": {},
		"homepage":      {},
		"visibility":    {},
		"template":      {},
		"topics":        {},
		"labels":        {},
		"features":      {},
	}); err != nil {
		return cfg, err
	}
	buildBlock, err := extractObjectBlock(raw, "build")
	if err != nil {
		return cfg, err
	}
	if err := assertOnlyAllowedFields("build", buildBlock, map[string]struct{}{
		"language":             {},
		"mode":                 {},
		"mainPackage":          {},
		"packages":             {},
		"runTests":             {},
		"outputDir":            {},
		"checksumFile":         {},
		"targets":              {},
		"artifactTargetSuffix": {},
	}); err != nil {
		return cfg, err
	}
	releaseBlock, err := extractObjectBlock(raw, "release")
	if err != nil {
		return cfg, err
	}
	if err := assertOnlyAllowedFields("release", releaseBlock, map[string]struct{}{
		"versionSource":        {},
		"tagPrefix":            {},
		"notesMode":            {},
		"releaseNotesFilePath": {},
		"artifactDir":          {},
		"includeArtifacts":     {},
		"includeChecksums":     {},
	}); err != nil {
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
	if goBlock, ok := extractOptionalObjectBlock(raw, "go"); ok {
		if err := assertOnlyAllowedFields("go", goBlock, map[string]struct{}{
			"cache_dir":     {},
			"mod_cache_dir": {},
			"toolchain":     {},
		}); err != nil {
			return cfg, err
		}
		cfg.Go.CacheDir = extractOptionalStringField(goBlock, "cache_dir")
		cfg.Go.ModCacheDir = extractOptionalStringField(goBlock, "mod_cache_dir")
		cfg.Go.Toolchain = extractOptionalStringField(goBlock, "toolchain")
	}
	if devBlock, ok := extractOptionalObjectBlock(raw, "devOutput"); ok {
		if err := assertOnlyAllowedFields("devOutput", devBlock, map[string]struct{}{
			"color":      {},
			"style":      {},
			"showPassed": {},
		}); err != nil {
			return cfg, err
		}
		if colorRaw, found := extractOptionalBoolFieldRaw(devBlock, "color"); found {
			cfg.DevOutput.Color = colorRaw
		} else {
			cfg.DevOutput.Color = extractOptionalStringField(devBlock, "color")
		}
		cfg.DevOutput.Style = extractOptionalStringField(devBlock, "style")
		cfg.DevOutput.ShowPassed = extractOptionalBoolField(devBlock, "showPassed", true)
	} else {
		cfg.DevOutput.ShowPassed = true
	}
	if cfg.DevOutput.Color == "" {
		cfg.DevOutput.Color = "auto"
	}
	if cfg.DevOutput.Style == "" {
		cfg.DevOutput.Style = "summary"
	}
	if coverageBlock, ok := extractOptionalObjectBlock(raw, "coverage"); ok {
		if err := assertOnlyAllowedFields("coverage", coverageBlock, map[string]struct{}{
			"default_min_percent": {},
			"fail_below_min":      {},
		}); err != nil {
			return cfg, err
		}
		cfg.Coverage.DefaultMinPercent = extractOptionalNumberPointerField(coverageBlock, "default_min_percent")
		cfg.Coverage.FailBelowMin = extractOptionalBoolField(coverageBlock, "fail_below_min", true)
	} else {
		cfg.Coverage.FailBelowMin = true
	}
	cfg.Build.Mode = extractOptionalStringField(buildBlock, "mode")
	if cfg.Build.Mode == "" {
		cfg.Build.Mode = "binary"
	}
	cfg.Build.MainPackage = extractOptionalStringField(buildBlock, "mainPackage")
	if cfg.Build.MainPackage == "" {
		cfg.Build.MainPackage = fmt.Sprintf("./cmd/%s", cfg.Project.Repo)
	}
	cfg.Build.Packages = extractOptionalStringListBlock(buildBlock, "packages")
	if len(cfg.Build.Packages) == 0 {
		cfg.Build.Packages = []string{"./..."}
	}
	cfg.Build.RunTests = extractOptionalBoolField(buildBlock, "runTests", false)
	cfg.Build.OutputDir = extractOptionalStringField(buildBlock, "outputDir")
	cfg.Build.ChecksumFile = extractOptionalStringField(buildBlock, "checksumFile")
	cfg.Build.Targets = extractOptionalStringListBlock(buildBlock, "targets")
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
	if featuresBlock, ok := extractOptionalObjectBlock(raw, "features"); ok {
		cfg.Repository.Features.MergeCommit, cfg.Repository.Features.MergeCommitSet = extractOptionalBoolFieldWithPresence(featuresBlock, "mergeCommit")
		cfg.Repository.Features.RebaseMerge, cfg.Repository.Features.RebaseMergeSet = extractOptionalBoolFieldWithPresence(featuresBlock, "rebaseMerge")
		cfg.Repository.Features.SquashMerge, cfg.Repository.Features.SquashMergeSet = extractOptionalBoolFieldWithPresence(featuresBlock, "squashMerge")
		cfg.Repository.Features.DeleteBranchOnMerge, cfg.Repository.Features.DeleteBranchOnMergeSet = extractOptionalBoolFieldWithPresence(featuresBlock, "deleteBranchOnMerge")
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
	cfg.Release.ArtifactDir = extractOptionalStringField(raw, "artifactDir")
	cfg.Release.IncludeArtifacts = extractOptionalBoolField(raw, "includeArtifacts", true)
	cfg.Release.IncludeChecksums = extractOptionalBoolField(raw, "includeChecksums", true)
	return cfg, nil
}

func extractOptionalObjectBlock(raw, field string) (string, bool) {
	block, err := extractObjectBlock(raw, field)
	if err != nil {
		return "", false
	}
	return block, true
}

func extractObjectBlock(raw, field string) (string, error) {
	open := strings.Index(raw, field+":")
	if open == -1 {
		return "", fmt.Errorf("missing required object field %s", field)
	}
	brace := strings.Index(raw[open:], "{")
	if brace == -1 {
		return "", fmt.Errorf("malformed object field %s", field)
	}
	start := open + brace + 1
	end, ok := findMatchingBrace(raw, start-1)
	if !ok || end <= start {
		return "", fmt.Errorf("malformed object field %s", field)
	}
	return raw[start:end], nil
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

func extractOptionalBoolFieldRaw(raw, field string) (string, bool) {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*(true|false)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

func extractOptionalNumberPointerField(raw, field string) *float64 {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*([0-9]+(?:\.[0-9]+)?)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return nil
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	return &v
}

func extractOptionalBoolFieldWithPresence(raw, field string) (bool, bool) {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)\b%s:\s*(true|false)\b`, regexp.QuoteMeta(field)))
	m := pattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return false, false
	}
	return m[1] == "true", true
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

func extractOptionalStringListBlock(raw, field string) []string {
	open := strings.Index(raw, field+": [")
	if open == -1 {
		return nil
	}
	start := open + len(field+": [")
	end := strings.Index(raw[start:], "]")
	if end == -1 {
		return nil
	}
	block := raw[start : start+end]
	matches := quotedStringPattern.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		return nil
	}
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values
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
		if err := assertOnlyAllowedFields("repository.labels item", item, map[string]struct{}{
			"name":        {},
			"color":       {},
			"description": {},
		}); err != nil {
			return nil, err
		}
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

func assertOnlyAllowedFields(scope, raw string, allowed map[string]struct{}) error {
	for _, field := range extractTopLevelFieldNames(raw) {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("unknown field %q in %s", field, scope)
		}
	}
	return nil
}

func extractTopLevelFieldNames(raw string) []string {
	var fields []string
	seen := map[string]struct{}{}
	inString := false
	escaped := false
	braceDepth := 0
	bracketDepth := 0
	lineStart := true

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			lineStart = false
			continue
		}
		if ch == '\n' {
			lineStart = true
			continue
		}
		switch ch {
		case '{':
			braceDepth++
			lineStart = false
			continue
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
			lineStart = false
			continue
		case '[':
			bracketDepth++
			lineStart = false
			continue
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
			lineStart = false
			continue
		}
		if !lineStart || braceDepth > 0 || bracketDepth > 0 {
			if !unicode.IsSpace(rune(ch)) {
				lineStart = false
			}
			continue
		}
		j := i
		for j < len(raw) && (raw[j] == ' ' || raw[j] == '\t') {
			j++
		}
		if j+1 < len(raw) && raw[j] == '/' && raw[j+1] == '/' {
			lineStart = false
			continue
		}
		start := j
		if j >= len(raw) || (!unicode.IsLetter(rune(raw[j])) && raw[j] != '_') {
			lineStart = false
			continue
		}
		j++
		for j < len(raw) && (unicode.IsLetter(rune(raw[j])) || unicode.IsDigit(rune(raw[j])) || raw[j] == '_') {
			j++
		}
		name := raw[start:j]
		for j < len(raw) && (raw[j] == ' ' || raw[j] == '\t') {
			j++
		}
		if j < len(raw) && raw[j] == ':' {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				fields = append(fields, name)
			}
		}
		lineStart = false
	}
	return fields
}

func findMatchingBrace(raw string, openBraceIdx int) (int, bool) {
	if openBraceIdx < 0 || openBraceIdx >= len(raw) || raw[openBraceIdx] != '{' {
		return -1, false
	}
	inString := false
	escaped := false
	depth := 0
	for i := openBraceIdx; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			depth++
			continue
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return -1, false
}
