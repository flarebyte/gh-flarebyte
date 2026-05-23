// purpose: Define the canonical in-memory schema for gh-flarebyte configuration.
// responsibilities: Declare config structs/constants; model repository/build/release fields and feature-presence flags used by planning logic.
// architecture notes: Feature `*Set` booleans preserve "explicitly configured" semantics so unset fields are not treated as forced false during reconciliation.
package config

import "regexp"

const DefaultPath = ".gh-flarebyte.cue"

type Config struct {
	Project    ProjectConfig    `json:"project"`
	Sync       SyncConfig       `json:"sync"`
	Repository RepositoryConfig `json:"repository"`
	Go         GoConfig         `json:"go"`
	DevOutput  DevOutputConfig  `json:"devOutput"`
	Coverage   CoverageConfig   `json:"coverage"`
	Build      BuildConfig      `json:"build"`
	Release    ReleaseConfigRaw `json:"release"`
}

type GoConfig struct {
	CacheDir    string `json:"cacheDir"`
	ModCacheDir string `json:"modCacheDir"`
	Toolchain   string `json:"toolchain"`
	CGO         CGOConfig
}

type CGOConfig struct {
	Enabled bool   `json:"enabled"`
	CC      string `json:"cc"`
	CXX     string `json:"cxx"`
}

type DevOutputConfig struct {
	Color      string `json:"color"`
	Style      string `json:"style"`
	ShowPassed bool   `json:"showPassed"`
}

type CoverageConfig struct {
	DefaultMinPercent *float64 `json:"defaultMinPercent"`
	FailBelowMin      bool     `json:"failBelowMin"`
}

type ProjectConfig struct {
	Org  string `json:"org"`
	Repo string `json:"repo"`
}

type SyncConfig struct {
	Mode string `json:"mode"`
}

type RepositoryConfig struct {
	Description   string                   `json:"description"`
	DefaultBranch string                   `json:"defaultBranch"`
	Homepage      string                   `json:"homepage"`
	Visibility    string                   `json:"visibility"`
	Template      bool                     `json:"template"`
	Topics        []string                 `json:"topics"`
	Labels        []LabelConfig            `json:"labels"`
	Features      RepositoryFeaturesConfig `json:"features"`
}

type LabelConfig struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type RepositoryFeaturesConfig struct {
	MergeCommit            bool `json:"mergeCommit"`
	MergeCommitSet         bool `json:"mergeCommitSet"`
	RebaseMerge            bool `json:"rebaseMerge"`
	RebaseMergeSet         bool `json:"rebaseMergeSet"`
	SquashMerge            bool `json:"squashMerge"`
	SquashMergeSet         bool `json:"squashMergeSet"`
	DeleteBranchOnMerge    bool `json:"deleteBranchOnMerge"`
	DeleteBranchOnMergeSet bool `json:"deleteBranchOnMergeSet"`
}

type BuildConfig struct {
	Language             string   `json:"language"`
	Mode                 string   `json:"mode"`
	MainPackage          string   `json:"mainPackage"`
	Packages             []string `json:"packages"`
	RunTests             bool     `json:"runTests"`
	OutputDir            string   `json:"outputDir"`
	ChecksumFile         string   `json:"checksumFile"`
	Targets              []string `json:"targets"`
	ArtifactTargetSuffix bool     `json:"artifactTargetSuffix"`
}

type ReleaseConfigRaw struct {
	VersionSource        string `json:"versionSource"`
	TagPrefix            string `json:"tagPrefix"`
	NotesMode            string `json:"notesMode"`
	ReleaseNotesFilePath string `json:"releaseNotesFilePath,omitempty"`
	ArtifactDir          string `json:"artifactDir"`
	IncludeArtifacts     bool   `json:"includeArtifacts"`
	IncludeChecksums     bool   `json:"includeChecksums"`
}

var targetPattern = regexp.MustCompile(`^(linux|darwin|windows)-(amd64|arm64)$`)
var quotedStringPattern = regexp.MustCompile(`"([^"]+)"`)
var labelObjectPattern = regexp.MustCompile(`(?s)\{([^{}]*)\}`)
