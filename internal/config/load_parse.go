// purpose: Load `.gh-flarebyte.cue` files and parse required config fields into strongly typed runtime structures.
// responsibilities: Resolve config path; read config bytes; extract scalar/list/object fields; assemble Config; invoke validation.
// architecture notes: Parsing is regex/string-block based (not full CUE evaluation) to keep the CLI self-contained and deterministic for the constrained config dialect it supports.
package config

import (
	"fmt"
	"os"
	"path/filepath"
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
			"cgo":           {},
		}); err != nil {
			return cfg, err
		}
		cfg.Go.CacheDir = extractOptionalStringField(goBlock, "cache_dir")
		cfg.Go.ModCacheDir = extractOptionalStringField(goBlock, "mod_cache_dir")
		cfg.Go.Toolchain = extractOptionalStringField(goBlock, "toolchain")
		if cgoBlock, ok := extractOptionalObjectBlock(goBlock, "cgo"); ok {
			if err := assertOnlyAllowedFields("go.cgo", cgoBlock, map[string]struct{}{
				"enabled": {},
				"cc":      {},
				"cxx":     {},
			}); err != nil {
				return cfg, err
			}
			cfg.Go.CGO.Enabled = extractOptionalBoolField(cgoBlock, "enabled", false)
			cfg.Go.CGO.CC = extractOptionalStringField(cgoBlock, "cc")
			cfg.Go.CGO.CXX = extractOptionalStringField(cgoBlock, "cxx")
		}
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
