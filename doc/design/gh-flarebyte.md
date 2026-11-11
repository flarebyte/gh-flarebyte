# gh-flarebyte Design Notes

Repository sync model for the flarebyte GitHub CLI extension.

## 01 Overview

What the extension manages and why the repo-local CUE file exists.

### 01 Intent

Why the checked-in config file exists.

#### Project Summary

Flarebyte's `gh` extension manages GitHub repository state from a checked-in `.gh-flarebyte.cue` file so repo metadata, topics, labels, and repo settings can be synchronized deterministically. The same file also carries top-level `build` and `release` blocks that drive local extension commands. `gh flarebyte repo update` applies the config-driven `gh repo edit` changes.

### 02 Scope

The operational boundary for the extension.

#### Project Scope

The extension is centered on repo bootstrap, reconciliation, audit, repository discovery, build, and release. Topics are flat strings, labels are structured objects, repo-edit mutations are applied by `gh flarebyte repo update` rather than by manually repeating `gh repo edit` flags, and build and release automation remain extension-local configuration rather than GitHub repository state.

## 02 Configuration

Canonical repo configuration and the field map used for sync.

### 01 Config Example

The repo-local `.gh-flarebyte.cue` file.

#### Spec Config Example

```cue
package ghflarebyte

project: {
	org:  "flarebyte"
	repo: "gh-flarebyte"
}

sync: {
	mode: "push"
}

repository: {
	description:   "CLI for landing your git commands right"
	defaultBranch: "main"
	homepage:      "https://github.com/flarebyte/gh-flarebyte"
	visibility:    "public"
	template:      false
	topics: [
		"gh-extension",
		"github-cli",
		"git",
		"flarebyte",
	]
	labels: [
		{
			name:        "bug"
			color:       "B60205"
			description: "Something is broken"
		},
		{
			name:        "enhancement"
			color:       "0E8A16"
			description: "New feature"
		},
	]
	features: {
		issues:                        true
		wiki:                          false
		projects:                      false
		discussions:                   false
		autoMerge:                     true
		mergeCommit:                   false
		rebaseMerge:                   false
		squashMerge:                   true
		squashMergeCommitMessage:      "pr-title"
		deleteBranchOnMerge:           true
		allowForking:                  false
		allowUpdateBranch:             false
		advancedSecurity:              true
		secretScanning:                true
		secretScanningPushProtection:  true
	}
}

build: {
	language:             "go"
	mode:                 "binary"
	outputDir:            "build"
	checksumFile:         "build/checksums.txt"
	artifactTargetSuffix: true
	targets: [
		"linux-amd64",
		"darwin-arm64",
		"windows-amd64",
	]
}

go: {
	toolchain: "local"
	cgo: {
		enabled: false
	}
}

release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	includeArtifacts: true
	artifactDir:      "build"
	includeChecksums: true
}
```

### 02 Field Map

How config fields map onto GitHub sync targets and extension-local automation settings.

#### Config Sync Field Map

| field | kind | managed_by | notes | sync_direction |
| --- | --- | --- | --- | --- |
| project.org | string | local config | The owner org anchors discovery and bootstrap. | read-only |
| project.repo | string | local config | The repository name binds the config to one GitHub repo. | read-only |
| sync.mode | enum | extension | Only push is accepted. Init can import current GitHub metadata into a new local file; update and audit do not pull remote changes into config. | read-only |
| repository.description | string | .gh-flarebyte.cue | Primary repo metadata field. | local-to-remote |
| repository.defaultBranch | string | .gh-flarebyte.cue | Usually `main` for flarebyte repositories. | local-to-remote |
| repository.homepage | string | .gh-flarebyte.cue | Repository homepage URL. | local-to-remote |
| repository.visibility | enum | .gh-flarebyte.cue | Repository visibility: public, private, or internal. | local-to-remote |
| repository.template | boolean | .gh-flarebyte.cue | Make the repository available as a template. | local-to-remote |
| repository.topics | list | .gh-flarebyte.cue | Topics are managed as the complete desired set. Remote topics absent from config are deletions requiring confirmation. | local-to-remote |
| repository.labels | list | .gh-flarebyte.cue | Labels are managed as the complete desired set by label name. Remote labels absent from config are deletions, and renaming a label is modeled as delete-plus-create. | local-to-remote |
| repository.features.issues | boolean | .gh-flarebyte.cue | GitHub issues enabled or disabled. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.wiki | boolean | .gh-flarebyte.cue | GitHub wiki enabled or disabled. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.projects | boolean | .gh-flarebyte.cue | GitHub projects enabled or disabled. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.discussions | boolean | .gh-flarebyte.cue | GitHub discussions enabled or disabled. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.autoMerge | boolean | .gh-flarebyte.cue | Auto-merge policy. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.mergeCommit | boolean | .gh-flarebyte.cue | Merge commit policy. | local-to-remote |
| repository.features.rebaseMerge | boolean | .gh-flarebyte.cue | Rebase merge policy. | local-to-remote |
| repository.features.squashMerge | boolean | .gh-flarebyte.cue | Squash merge policy. | local-to-remote |
| repository.features.squashMergeCommitMessage | enum | .gh-flarebyte.cue | Default squash merge commit message behavior. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.deleteBranchOnMerge | boolean | .gh-flarebyte.cue | Head branch cleanup policy. | local-to-remote |
| repository.features.allowForking | boolean | .gh-flarebyte.cue | Forking policy for organization repos. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.allowUpdateBranch | boolean | .gh-flarebyte.cue | Allow PR head branch updates when behind the base branch. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.advancedSecurity | boolean | .gh-flarebyte.cue | GitHub Advanced Security. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.secretScanning | boolean | .gh-flarebyte.cue | Secret scanning for the repository. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| repository.features.secretScanningPushProtection | boolean | .gh-flarebyte.cue | Secret scanning push protection. Accepted in config but not applied or audited by the CLI. | accepted-not-enforced |
| build.language | enum | .gh-flarebyte.cue | Supported languages: go, dart, python, rust, swift, typescript, kotlin. Python uses uv; Rust uses Cargo. Swift uses SwiftPM and requires a Swift 6+ toolchain. TypeScript uses npm and Node for tests and quality tools; Bun compiles standalone CLIs. Kotlin uses the committed Gradle wrapper for one root Kotlin/JVM project. | local-only |
| build.mode | enum | .gh-flarebyte.cue | Go supports binary/library; Dart supports library; Python supports package; Rust supports binary/library/package. Default remains binary. Swift supports binary/library; package mode is excluded. TypeScript supports library verification and binary CLI builds; library builds emit no release assets and require includeArtifacts=false. Kotlin supports library verification and binary JVM CLI distributions; library releases require includeArtifacts=false. | local-only |
| build.packages | list | .gh-flarebyte.cue | Defaults to ["./..."]. Go uses package patterns; Rust uses all workspace members or explicit crate names for library/package builds. Swift uses all root build targets or exact target names. TypeScript accepts only ./... for one root tsconfig.json project. Kotlin accepts only ./... for one root Gradle JVM project. | local-only |
| build.runTests | boolean | .gh-flarebyte.cue | Enable optional tests during library/package builds and Rust binary builds. Rust tests selected crates on the active compiler host before cross-building. Swift tests the whole root package before library/binary builds. TypeScript checks types and optionally runs npm/Node tests before library or CLI builds. Applies to both Kotlin build modes. | local-only |
| build.mainPackage | string | .gh-flarebyte.cue | Go main package defaults to ./cmd/<repo>; Rust package manifest directory defaults to a dot. Rust chooses the sole binary or package.default-run. Swift names an executable product or infers the only one. TypeScript CLI builds require an explicit source entry point included in tsconfig.json; library builds reject this field. Kotlin rejects mainPackage; Gradle application.mainClass selects the CLI entry point. | local-only |
| build.outputDir | string | .gh-flarebyte.cue | Directory for built artifacts. | local-only |
| build.checksumFile | string | .gh-flarebyte.cue | Checksum manifest for built artifacts. | local-only |
| build.targets | list | .gh-flarebyte.cue | Go/Rust target matrix using os-arch strings. Rust maps to GNU Linux and Apple Darwin and MSVC Windows triples. Package mode rejects target matrices and --target. Swift supports only the native macOS or GNU/Linux host on amd64/arm64. TypeScript CLI targets must match the native Bun macOS or GNU/Linux amd64/arm64 host. TypeScript library mode rejects targets and --target. Kotlin rejects targets and --target because JVM distributions are platform-independent. | local-only |
| go.cgo.enabled | boolean | .gh-flarebyte.cue | Explicit CGO mode for Go builds. When true build sets `CGO_ENABLED=1`; when false build sets `CGO_ENABLED=0`. | local-only |
| go.cgo.cc | string | .gh-flarebyte.cue | Optional C compiler override for CGO builds. Requires `go.cgo.enabled: true`. | local-only |
| go.cgo.cxx | string | .gh-flarebyte.cue | Optional C++ compiler override for CGO builds. Requires `go.cgo.enabled: true`. | local-only |
| release.versionSource | string | .gh-flarebyte.cue | YAML/JSON version or Python project.version in TOML or a Rust package Cargo.toml including workspace-inherited versions. Virtual Rust workspaces require a member manifest. Swift uses the existing YAML/JSON top-level version. TypeScript can use the existing top-level package.json version. | local-only |
| release.tagPrefix | string | .gh-flarebyte.cue | Prefix used for release tags. | local-only |
| release.notesMode | enum | .gh-flarebyte.cue | Release note strategy, such as generate-notes or notes-file. | local-only |
| release.releaseNotesFilePath | string | .gh-flarebyte.cue | Path required when notesMode is `notes-file`. | local-only |
| release.includeArtifacts | boolean | .gh-flarebyte.cue | Whether release uploads build artifacts or publishes tag/notes only. | local-only |
| release.artifactDir | string | .gh-flarebyte.cue | Release asset directory. Go scans matching assets; Python/Rust stage only current build outputs and preserve relative artifact paths. Swift stages only fresh executable assets and checksums. TypeScript CLI releases stage only fresh executable assets and checksums. Kotlin stages fresh JVM ZIP distributions and checksums. | local-only |
| release.includeChecksums | boolean | .gh-flarebyte.cue | Whether to include the checksum manifest as a release asset. | local-only |
| e2e.runtime | string | .gh-flarebyte.cue | Optional independent e2e runtime: bun or python3. | local-only |
| e2e.root | string | .gh-flarebyte.cue | Repository-relative suite folder; defaults to e2e. | local-only |
| e2e.dependencies.manager | string | .gh-flarebyte.cue | Optional dependency preparation: bun or npm for Bun; uv for Python. Defaults to bun for Bun. | local-only |
| e2e.dependencies.root | string | .gh-flarebyte.cue | Dependency files at repository root (.) or suite root; defaults to repository root. | local-only |
| script.runtime | string | .gh-flarebyte.cue | Optional independent script runtime: bun or python3. | local-only |
| script.root | string | .gh-flarebyte.cue | Repository-relative script folder; defaults to script. | local-only |
| script.dependencies.manager | string | .gh-flarebyte.cue | Optional dependency preparation: bun or npm for Bun; uv for Python. | local-only |
| script.dependencies.root | string | .gh-flarebyte.cue | Dependency root defaults to .; matching JavaScript roots share one manager. | local-only |

### 03 Topics

Repository topics are a flat sync target in the cue config.

#### Topic Sync

Topics are kept as a flat string list in the cue config and synchronized as the complete desired repository topics set. If applying the config would delete remote topics, `gh flarebyte repo update` fails unless the user explicitly confirms deletions.

### 04 Labels

Repository labels have their own structured sync shape in the cue config.

#### Label Sync

Labels use a structured list of objects in the cue config so name, color, and description can be reconciled separately from repository topics. The config represents the complete desired label set; labels missing from config are treated as deletions and require explicit confirmation before update. Because labels are keyed by name, renaming a label is modeled explicitly as delete-plus-create.

### 05 Build

Build language selection for gh flarebyte build.

#### Build Config

Build orchestration uses the shared top-level `build` block. Go supports binary and library modes; Dart supports library validation; Python 3 with uv supports package mode; Rust with Cargo supports binary/library/package modes; Swift 6+ with SwiftPM supports library verification and native macOS/Linux binary builds. Kotlin/JVM with Gradle supports library verification and platform-independent CLI distributions containing launch scripts and dependency JARs; targets and mainPackage are rejected. TypeScript with npm and Node supports library type verification without assets and native macOS/Linux CLI binaries from an explicit mainPackage source entry point. Swift uses existing configuration fields, executable product names, and exact root target names or ./... for build selection. Python package builds produce a wheel and source distribution via `uv build --no-sources`, reuse outputDir/checksumFile/runTests, and reject target matrices and --target. All Python developer tools run through `uv run --locked` with a repository-local pyproject.toml and uv.lock. Python version selection and tool rules remain in pyproject.toml and optional .python-version. Go binary mode writes per-target binaries and archives with SHA-256 checksums; library mode compiles configured packages and optionally runs tests without binary artifacts. Go library --target maps to GOOS/GOARCH validation. Isolated Python build dependencies are resolved separately from uv.lock; pin them in build-system.requires for repeatable builds.

### 06 Release

Release publication settings for gh flarebyte release.

#### Release Config

Release publication uses the shared top-level `release` block for tag prefix, version source, artifact policy, and notes. YAML reads a version line and JSON reads a top-level version; TOML reads Python static [project].version or Rust Cargo package.version including workspace inheritance under the same semantic-version rule. Virtual Rust workspaces require a member manifest as versionSource. Dynamic Python versions are rejected. Python, Rust, Swift, TypeScript, and Kotlin releases stage only artifacts and optional checksums returned by the fresh build into artifactDir, preserving relative paths and avoiding stale assets. Dart releases require includeArtifacts=false. No PyPI or crates.io publishing or native Python wheel matrix is included.

### 07 Sync Types

TypeScript shapes for the sync contract.

#### Sync Types

```ts
export type SyncMode = "push";

// Only mergeCommit, rebaseMerge, squashMerge, and deleteBranchOnMerge are enforced.
export type RepositoryFeatures = {
  issues?: boolean;
  wiki?: boolean;
  projects?: boolean;
  discussions?: boolean;
  autoMerge?: boolean;
  mergeCommit?: boolean;
  rebaseMerge?: boolean;
  squashMerge?: boolean;
  squashMergeCommitMessage?: "default" | "pr-title" | "pr-title-commits" | "pr-title-description";
  deleteBranchOnMerge?: boolean;
  allowForking?: boolean;
  allowUpdateBranch?: boolean;
  advancedSecurity?: boolean;
  secretScanning?: boolean;
  secretScanningPushProtection?: boolean;
};

export type LabelConfig = {
  name: string;
  color: string;
  description: string;
};

// Shape examples; language-specific selector and target restrictions are checked at runtime.
export type BuildConfig =
  | {
      language: "go" | "rust" | "swift" | "typescript";
      mode?: "binary"; // The default mode.
      mainPackage?: string; // Required for TypeScript; other languages have selection defaults.
      outputDir: string;
      checksumFile: string;
      targets: BuildTarget[];
      artifactTargetSuffix?: boolean;
      runTests?: boolean;
    }
  | {
      language: "go" | "dart" | "rust" | "swift" | "typescript" | "kotlin";
      mode: "library";
      packages?: string[]; // Defaults to ["./..."]; TS/Kotlin accept only that root selection.
      runTests?: boolean;
    }
  | {
      language: "python" | "rust";
      mode: "package";
      packages?: string[];
      outputDir: string;
      checksumFile: string;
      runTests?: boolean;
    }
  | {
      language: "kotlin";
      mode?: "binary";
      outputDir: string;
      checksumFile: string;
      runTests?: boolean;
      // Gradle selects the entry point. Platform targets and target suffixes are rejected.
    };

export type BuildTarget = `${"linux" | "darwin" | "windows"}-${"amd64" | "arm64"}`;

export type GoCGOConfig = {
  enabled: boolean;
  cc?: string;
  cxx?: string;
};

export type GoConfig = {
  cacheDir?: string;
  modCacheDir?: string;
  toolchain?: string;
  cgo?: GoCGOConfig;
};

export type ReleaseConfigBase = {
  versionSource: string;
  tagPrefix: string;
  includeArtifacts?: boolean; // Defaults to true; libraries require false.
  artifactDir?: string;
  includeChecksums?: boolean;
};

export type ReleaseConfig =
  | (ReleaseConfigBase & {
      notesMode: "generate-notes" | "notes-from-tag";
    })
  | (ReleaseConfigBase & {
      notesMode: "notes-file";
      releaseNotesFilePath: string;
    });

export type ProjectConfig = {
  org: string;
  repo: string;
};

export type RepositoryConfig = {
  description: string;
  defaultBranch: string;
  homepage: string;
  visibility: "public" | "private" | "internal";
  template: boolean;
  topics: string[];
  labels: LabelConfig[];
  features?: RepositoryFeatures;
};

export type ToolingConfig = {
  runtime: "bun" | "python3";
  root?: string; // Defaults to e2e or script; repository-relative.
  dependencies?: {
    manager?: "bun" | "npm" | "uv";
    root?: string; // Defaults to the repository root (.).
  };
};

export type GhFlarebyteConfig = {
  project: ProjectConfig;
  sync: SyncPlan;
  repository: RepositoryConfig;
  go?: GoConfig;
  e2e?: ToolingConfig;
  script?: ToolingConfig;
  devOutput?: {
    color?: "auto" | "true" | "false";
    style?: "summary" | "per_test";
    showPassed?: boolean;
  };
  coverage?: { min?: number; enforceMin?: boolean };
  build: BuildConfig;
  release: ReleaseConfig;
};

export type SyncPlan = {
  mode: SyncMode;
};

export type DriftItem = {
  field: string;
  local: string | boolean | string[];
  remote: string | boolean | string[];
};
```

### 08 Config Coverage

Additional gh repo edit settings that are now modeled in the cue sync config.

#### Additional Config Coverage

Update and audit enforce description, defaultBranch, homepage, visibility, template, topics, labels, and four explicitly set feature flags: mergeCommit, rebaseMerge, squashMerge, and deleteBranchOnMerge. Other repository.features keys are accepted in configuration but are not applied or audited.

## 03 Commands

User-facing extension commands and the config-driven sync path.

### 01 Command Matrix

User-facing extension actions and their purpose.

#### Extension Command Matrix

| command | config_touchpoints | output | purpose | read_write |
| --- | --- | --- | --- | --- |
| gh flarebyte build | build.* -> binaries or library checks or Python distributions or Rust crate packages | language-specific build output Swift verifies libraries or packages native executable artifacts. TypeScript verifies libraries or compiles native Bun CLI executables with checksums. Kotlin assembles libraries or packages a fresh Gradle application distZip as repo-version.zip plus checksums; no platform targets. | build the project according to the configured language and build mode | write |
| gh flarebyte test | build.language and devOutput.* | Go JSON or Dart JSON or Python JUnit XML or Rust standard harness pretty output. Rust includes doctests; custom harnesses require summary style. Swift combines serial XCTest results with Swift Testing JSON schema 0 and rejects incomplete reports. TypeScript runs npm test with Node JUnit reports and per-test output. Kotlin runs root Gradle test with fresh JUnit XML and validates testcase totals. | run language tests with summary or per-test output | write |
| gh flarebyte format | build.language | Go gofmt or Dart format or Python locked Ruff format or Rust cargo fmt --all Swift formats target-owned sources and Package.swift using swift format. TypeScript runs project-local Biome formatting through Node. Kotlin runs Gradle ktlintFormat. | format sources for the configured language | write |
| gh flarebyte lint | build.language and devOutput.* | Go vet or Dart analyze or Python Ruff or Rust Clippy with warnings denied across workspace targets Swift runs strict formatter lint and compiles sources/tests with warnings as errors. TypeScript runs Biome checks without fixes and tsc --noEmit. Kotlin runs ktlintCheck and compiles production and test classes with warnings treated as errors. | run static checks without fixing sources | write |
| gh flarebyte cov | build.language and coverage.* and devOutput.* | Go coverage or Dart LCOV or Python JSON or Rust cargo-llvm-cov LCOV. Rust stable coverage excludes doctests and dependency/generated directories. Swift uses LLVM JSON production-source line coverage weighted by line count. TypeScript uses fresh c8 source-mapped LCOV reports from npm test; unimported production files receive zero coverage and shared thresholds remain authoritative. Kotlin runs test and jacocoTestReport with weighted production LINE coverage and shared thresholds. | measure coverage and apply the shared threshold policy | write |
| gh flarebyte release | build.* and release.* -> GitHub release with optional assets | versioned GitHub release Kotlin stages only current JVM distribution assets; library releases have no assets. | run build then publish a GitHub release from the configured release settings | write |
| gh flarebyte repo init | .gh-flarebyte.cue created or seeded | local .gh-flarebyte.cue created; no remote mutation | create local config using imported metadata or defaults | write |
| gh flarebyte repo update | .gh-flarebyte.cue -> GitHub repo state with `--confirm-deletions` and `--accept-visibility-change-consequences` when needed | updated remote repo state | reconcile GitHub repo metadata from local config with explicit safety flags | write |
| gh flarebyte repo audit | .gh-flarebyte.cue and GitHub state | drift report | compare local config with remote GitHub state | read |
| gh flarebyte repos mine | none | relevant repo list | list repositories the user contributes to within an org | read |
| gh flarebyte e2e | e2e.* | Bun JUnit or Python unittest discovery; empty suites fail. | run an independent Bun or Python e2e suite | write |
| gh flarebyte script | script.* | Child output and exit status; no shell interpolation. | execute a Bun TypeScript or Python file with forwarded arguments | write |

### 02 Command Flows

TypeScript examples that show the intended command sequences.

#### Extension Command Flows

```ts
export type CommandFlow = {
  name: string;
  command: string;
  repo: string;
  purpose: string;
  syncEffect: string;
};

export const commandFlows: CommandFlow[] = [
  {
    name: "build",
    command: "gh flarebyte build",
    repo: "my-org/my-repo",
    purpose: "Build local artifacts from the top-level build block.",
    syncEffect: "Verify a library or write artifacts and checksums according to the configured language and mode.",
  },
  {
    name: "release",
    command: "gh flarebyte release",
    repo: "my-org/my-repo",
    purpose: "Build first, then publish a GitHub release from the configured release block.",
    syncEffect: "Build and create a GitHub release; upload fresh assets only when configured.",
  },
  {
    name: "init",
    command: "gh flarebyte repo init --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Create repo-local config using imported metadata when available plus local build and release defaults.",
    syncEffect: "Write `.gh-flarebyte.cue` locally; existing files require --overwrite. No remote mutation.",
  },
  {
    name: "update",
    command:
      "gh flarebyte repo update --repo my-org/my-repo --confirm-deletions --accept-visibility-change-consequences",
    repo: "my-org/my-repo",
    purpose: "Reconcile GitHub repository state from the local config.",
    syncEffect:
      "Push local desired state to GitHub and fail if deletions or visibility changes are detected without the required safety flags.",
  },
  {
    name: "audit",
    command: "gh flarebyte repo audit --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Report drift between the checked-in config and GitHub.",
    syncEffect: "Read-only comparison.",
  },
  {
    name: "repos-mine",
    command: "gh flarebyte repos mine --org my-org",
    repo: "my-org",
    purpose: "Discover repositories the current user contributes to within an organization.",
    syncEffect: "Read-only discovery against GitHub data.",
  },
];
```

### 03 Flags

CLI flags supported by the current command surface.

#### Extension Command Flags

| automation_mode | command | flag | kind | purpose | required_when |
| --- | --- | --- | --- | --- | --- |
| script-friendly | gh flarebyte | --version | boolean | Print CLI version metadata including version, commitId, and build date. | optional |
| script-friendly | gh flarebyte | --json | boolean | When combined with `--version`, print version metadata as JSON instead of plain text. | optional |
| safety-gate | gh flarebyte repo init | --overwrite | boolean | Replace an existing `.gh-flarebyte.cue` file instead of failing. | optional |
| script-friendly | gh flarebyte build | --target | string | Select a supported language target; rejected by Dart, Python, Kotlin, and TypeScript library mode. | optional |
| script-friendly | gh flarebyte build | --output-dir | string | Override the configured output directory for one invocation. | optional |
| script-friendly | gh flarebyte release | --notes-file | string | Override the configured release.releaseNotesFilePath; configuration must already supply a path. | optional with release.notesMode notes-file |
| script-friendly | gh flarebyte release | --draft | boolean | Publish the release as a draft instead of a final release. | optional |
| script-friendly | gh flarebyte repo update | --repo | string | Override the GitHub repository to update using the local config. | optional; defaults to project.org/project.repo |
| safety-gate | gh flarebyte repo update | --confirm-deletions | boolean | Allow exact-set reconciliation to remove remote topics or labels missing from config. | required when topic or label deletions are detected |
| safety-gate | gh flarebyte repo update | --accept-visibility-change-consequences | boolean | Allow visibility transitions that need explicit acknowledgement. | required when repository visibility changes |
| script-friendly | gh flarebyte repo audit | --json | boolean | Emit the audit report as stable machine-readable JSON. | optional |
| script-friendly | gh flarebyte repos mine | --json | boolean | Emit discovered repositories as stable machine-readable JSON. | optional |
| script-friendly | gh flarebyte script | -- | separator | Forward subsequent arguments unchanged to the configured script file. | optional |
| script-friendly | gh flarebyte repo init | --repo | string | Repository owner/name used for metadata import and local config defaults. | always |
| script-friendly | gh flarebyte repo audit | --repo | string | Override the GitHub repository compared with the local config. | optional; defaults to project.org/project.repo |
| script-friendly | gh flarebyte repos mine | --org | string | Organization queried for contributor repositories. | always |
| script-friendly | gh flarebyte config validate | --config | string | Validate another file; other commands continue to read .gh-flarebyte.cue in the working directory. | optional |

### 04 Automation

Which flags are safe for scripts and which are explicit safety gates.

#### Automation Policy

Flags marked `script-friendly` are intended for repeatable local scripts and CI jobs. Flags marked `safety-gate` exist to make destructive or high-consequence changes explicit. In practice, `gh flarebyte repo update` fails in unattended runs when deletions or visibility changes are detected unless the corresponding safety flag is passed deliberately.

### 05 Guidance And Errors

Current usage guidance and error feedback.

#### Guidance And Error Feedback

| example_feedback | expectation | scenario |
| --- | --- | --- |
| Run `gh flarebyte repo update --help` to see the update flags. | Commands expose usage and flags through --help. | general help |
| `.gh-flarebyte.cue already exists in /path/to/repo. Re-run with --overwrite if replacing it is intentional` | An existing local configuration is rejected unless --overwrite is supplied. | init overwrite blocked |
| `cannot read config /path/to/repo/.gh-flarebyte.cue: open /path/to/repo/.gh-flarebyte.cue: no such file or directory` | Config load failures report the resolved file path and filesystem error. --repo does not replace the requirement for local configuration. | missing config |
| `invalid build.targets entry "linux/x64": expected os-arch format such as linux-amd64 or windows-amd64` | Validation identifies an invalid field or field combination. | config validation error |
| `update would delete 2 labels and 1 topics. Re-run with --confirm-deletions if that is intentional` | Topic/label deletions without --confirm-deletions are rejected with counts and the required flag. | deletion blocked |
| `visibility would change from public to private. Re-run with --accept-visibility-change-consequences if that is intentional` | Visibility changes without acknowledgement are rejected with old/new values and the required flag. | visibility blocked |
| `Build failed in library mode during dart pub/analyze/test.` | Build errors report the failing tool or mode. Go target failures also print the effective environment and build command. | build failure |
| `release tag v1.2.3 already exists. refusing to mutate existing release implicitly` | Release returns build failures before publishing; later errors report version/tag or GitHub publication failure. | release failure |
| `Update stopped after repository settings and topics were applied. Label sync failed, and no rollback was attempted. Fix the error and rerun gh flarebyte repo update. Cause: API error` | Updates stop on remote failure without rollback; later-stage errors include recorded settings/topic progress and the cause. | partial update failure |
| `3 differences found. Review the drift report below, then run gh flarebyte repo update when ready to apply changes.` | Audit reports differences and points to repo update; a clean audit reports no drift. | drift report |

### 06 Success Output

Current success messages and output policies.

#### Success Output

| example_output | expectation | scenario |
| --- | --- | --- |
| `Build complete: Dart library validation passed.` | Successful commands emit command-specific results; --failed-only and devOutput.showPassed can suppress developer success output. | general success |
| `gh-flarebyte v1.2.3 commitId=a1b2c3d4 date=2026-04-30T09:15:00Z os=darwin arch=arm64 goVersion=go1.25.0` | The root `--version` flag prints structured build metadata rather than only a semantic version string. | version success |
| `{"version":"v1.2.3","commitId":"a1b2c3d4","date":"2026-04-30T09:15:00Z","os":"darwin","arch":"arm64","goVersion":"go1.25.0"}` | When `--version --json` is requested the CLI emits machine-readable version metadata using the documented shape. | version json success |
| `created /path/to/repo/.gh-flarebyte.cue from current GitHub state. Next: review the config, then run gh flarebyte repo update.` | Init reports where the config file was created or updated, whether current GitHub state was imported, and what the next command is. | init success |
| `Update complete: 3 repo settings updated, 4 topics synced, 8 labels reconciled.` | Update summarizes what changed, including counts for topics, labels, and repo settings when relevant. | update success |
| `No drift found. GitHub matches .gh-flarebyte.cue.` | Audit summarizes whether drift exists and what the user should do next. | audit success |
| `Build complete: 3 targets written to build/ with checksums in build/checksums.txt.` | Build summarizes the targets produced and where artifacts were written. | build success |
| `Release v1.2.3 published from build with checksums attached.` | Release summarizes the tag published and whether assets were uploaded. | release success |
| `Found 12 repositories in org flarebyte for contributor olivier.` | Repos mine reports how many repositories were found for the requested organization. | discovery success |

### 07 Exit Codes

Stable process exit behavior for humans, scripts, and CI.

#### Command Exit Codes

| command | exit_code | meaning | when |
| --- | --- | --- | --- |
| gh flarebyte | 0 | Success. | successful command completed |
| gh flarebyte | 1 | Command failed after starting work. | unexpected runtime or dependency failure |
| gh flarebyte | 2 | Usage or input problem that the user can fix locally. | invalid invocation or config validation error |
| gh flarebyte repo audit | 0 | Repository matches `.gh-flarebyte.cue`. | no drift found |
| gh flarebyte repo audit | 3 | Command completed successfully and found differences. | drift found |
| gh flarebyte repo update | 4 | Safety gate prevented destructive sync. | deletions blocked without `--confirm-deletions` |
| gh flarebyte repo update | 5 | Safety gate prevented high-consequence visibility change. | visibility change blocked without `--accept-visibility-change-consequences` |
| gh flarebyte build | 6 | Compilation or packaging failed. | build failed for one or more requested targets |
| gh flarebyte release | 7 | GitHub release creation or asset upload failed. | release publish failed after version resolution and build |
| gh flarebyte script | child exit code | Preserve the script exit status; dependency or launch failures use 1. | script process fails |
| gh flarebyte e2e | child exit code | Preserve suite exit status; empty suites or invalid reports use 1. | suite process fails |

### 08 Structured Output

Machine-readable JSON output shapes for script-friendly commands.

#### Audit JSON Output

```ts
export type AuditDiff = {
  field: string;
  local: string | boolean | string[];
  remote: string | boolean | string[];
};

export type AuditReport = {
  repo: string;
  driftCount: number;
  hasDrift: boolean;
  diffs: AuditDiff[];
};

export const auditReportExample: AuditReport = {
  repo: "flarebyte/gh-flarebyte",
  driftCount: 2,
  hasDrift: true,
  diffs: [
    {
      field: "repository.homepage",
      local: "https://github.com/flarebyte/gh-flarebyte",
      remote: "",
    },
    {
      field: "repository.topics",
      local: ["gh-extension", "github-cli", "git", "flarebyte"],
      remote: ["gh-extension", "git"],
    },
  ],
};
```

#### Repos Mine JSON Output

```ts
export type ContributedRepo = {
  owner: string;
  name: string;
  visibility: "public" | "private" | "internal";
  defaultBranch: string;
};

export type ReposMineReport = {
  org: string;
  contributor: string;
  count: number;
  repos: ContributedRepo[];
};

export const reposMineReportExample: ReposMineReport = {
  org: "flarebyte",
  contributor: "olivier",
  count: 2,
  repos: [
    {
      owner: "flarebyte",
      name: "gh-flarebyte",
      visibility: "public",
      defaultBranch: "main",
    },
    {
      owner: "flarebyte",
      name: "baldrick-seer",
      visibility: "public",
      defaultBranch: "main",
    },
  ],
};
```

### 09 Version

How the root CLI version flag exposes embedded build metadata.

#### Version Output

```ts
export type VersionInfo = {
  version: string;
  commitId: string;
  date: string;
  os: string;
  arch: string;
  goVersion: string;
};

export const versionExample: VersionInfo = {
  version: "v1.2.3",
  commitId: "a1b2c3d4",
  date: "2026-04-30T09:15:00Z",
  os: "darwin",
  arch: "arm64",
  goVersion: "go1.25.0",
};
```

#### Version Output Policy

The root `gh flarebyte --version` command prints concise human-readable plain text by default. When `--json` is passed alongside `--version`, it emits machine-readable JSON using the documented version metadata shape. This keeps the default friendly for humans while making automation explicit and stable.

### 10 Build

How build orchestration is driven from config.

#### Build Command

Build from the shared build block: Go binary/library, Dart library, Python package, Rust binary/library/package, Swift binary/library, TypeScript library/binary CLI, or Kotlin JVM library/binary CLI. Kotlin uses a committed Gradle wrapper and root dependency lockfile with strict locking. One root Kotlin/JVM project is supported. ktlintFormat formats; ktlintCheck plus production/test compilation with warnings as errors lints. Tests validate fresh Gradle JUnit XML; coverage reads JaCoCo production LINE counters and applies shared thresholds. Library builds assemble without release assets; CLI builds package application distZip with launch scripts and dependency JARs as repo-version.zip plus checksums. Both modes honor runTests. Reports and build outputs use isolated temporary directories. mainPackage, targets, --target, and artifactTargetSuffix are rejected. JVM CLI distributions require Java; native executables, bundled runtimes, multi-project/composite builds, Android, Multiplatform, and registry publishing are excluded. TypeScript requires a committed package-lock.json and uses npm ci with lifecycle scripts disabled. Project-local Biome and tsc run through Node. npm test uses Node with tsx and fresh JUnit reports. c8 wraps npm test for source-mapped LCOV coverage including unimported production sources at zero coverage. Optional e2e and script sections select independent Bun TypeScript or Python 3 execution. Source roots default to e2e and script; dependency roots default to the repository root and may be isolated in the source folder. Bun supports locked Bun or npm dependencies; Python optionally uses locked uv dependencies. Without dependencies blocks no installation occurs. Bun e2e validates fresh JUnit; Python uses unittest discovery. Empty suites fail. Scripts forward arguments and child exit status without shell interpolation. TypeScript library verification has no entry point or targets and emits no artifacts. CLI builds require an explicit source entry point and match the native Bun host, using existing executable archives and checksum policies. Workspaces, project references, Windows, cross-compilation, npm packaging and npm publication are unsupported. Go targets use os-arch strings; Unix archives use tar.gz and Windows zip with .exe binaries. Go library builds compile package patterns and optionally test. Dart libraries run pub get/analyze and optional tests. Python package builds validate Python 3 and the lockfile, optionally run pytest, then build a wheel and source distribution in a fresh temporary directory and copy them to outputDir with SHA-256 checksums. --output-dir relocates artifacts and checksums. Python test, format, lint, and cov use pytest JUnit XML, Ruff, and coverage.py JSON with shared output and threshold fields. Go binary builds embed version, commitId, date, and runtime metadata for --version and --version --json. Failures identify the target or mode and failing step. Python, Rust, and Swift failures preserve stdout and stderr diagnostics. Swift requires Package.swift and Package.resolved for remote dependencies, enforcing resolved versions without updates. It formats owned files with swift format, lints strictly and compiles test targets with warnings as errors, tests XCTest serial output and Swift Testing JSON schema 0, and measures production line coverage through fresh LLVM JSON exports. Optional build tests run the whole package on the host. Binary builds discover release paths with --show-bin-path and reject resource bundles and package-built shared libraries. Cross-compilation, Windows, Xcode apps, registry publishing, signing, and notarization are unsupported. Swift versions use the shared YAML/JSON sources. Rust commands resolve dependencies with --locked, test the workspace including doctests, format with rustfmt, lint with Clippy and warnings denied, and measure line coverage with cargo-llvm-cov LCOV. Custom Rust harnesses require summary output. Stable Rust coverage excludes doctests and dependency/generated sources. Rust library/package selection uses ./... for all workspace members or exact crate names; binary selection uses mainPackage as a manifest directory and the sole binary or package.default-run. Rust target names map to GNU Linux, Apple Darwin, and MSVC Windows triples; installed target components and linkers are required. Optional build tests execute on the compiler host. Rust packaging keeps verification enabled and copies only fresh .crate files; binary packaging reuses the Go artifact policy and discovers executable paths through Cargo compiler JSON.

#### Build Artifact Rules

| binary_name | checksum_algorithm | notes | package_format | target |
| --- | --- | --- | --- | --- |
| gh-flarebyte-linux-amd64 | sha256 | Unix binary builds produce compressed archives containing the binary. | tar.gz | linux-amd64 |
| gh-flarebyte-darwin-arm64 | sha256 | macOS binary builds produce compressed archives containing the binary. | tar.gz | darwin-arm64 |
| gh-flarebyte-windows-amd64.exe | sha256 | Windows binary builds use `.exe` binaries and ZIP packaging. | zip | windows-amd64 |
| <distribution>-<version>-<python>-<abi>-<platform>.whl | sha256 | uv builds one wheel and source distribution in a fresh temporary directory; only fresh assets are copied and released. | wheel | python-package |
| <distribution>-<version>.tar.gz | sha256 | Python package builds use outputDir and checksumFile with no target suffix override or target matrix. | sdist | python-package |
| <crate>-<version>.crate | sha256 | Cargo package verification stays enabled; only selected crates produced in the fresh target directory are copied and released. No crates.io publication. | crate | rust-package |
| <repo>-<os-arch> | sha256 | Rust binaries reuse Go naming and archive policies. Cargo compiler JSON identifies executables. Windows binary assets end in .exe. | tar.gz or zip | rust-binary |
| <repo>-<os-arch> | sha256 | Native macOS or GNU/Linux only; SwiftPM release executable path is discovered with --show-bin-path. Fresh assets reuse Rust/Go naming. Resource bundles and package-built shared libraries are rejected. Runtime libraries are not bundled. | tar.gz | swift-binary |
| <repo>-<os-arch> | sha256 | Bun compiles one explicit source entry point into a native macOS/Linux CLI. TypeScript library mode only checks types and emits no release artifacts. No npm package archives or registry publication. | tar.gz | typescript-cli |
| <repo>-<version>.zip | sha256 | Gradle application distribution contains executable launch scripts and dependency JARs. Requires a compatible JVM. No platform targets; libraries have no release assets. | zip | kotlin-cli |

### 11 Release

How release publication is driven from config.

#### Release Command

Build first, then publish a GitHub release using the shared release policy. Resolve a YAML version line or JSON top-level version or TOML Python project.version or Rust Cargo package.version with workspace inheritance, derive tagPrefix + version, and fail if the tag exists. includeArtifacts controls uploads; includeChecksums controls the manifest. Python, Rust, Swift, TypeScript, and Kotlin assets come only from the current build and are staged into artifactDir without scanning for stale distributions. notes-file requires releaseNotesFilePath during config validation; --notes-file overrides that configured path. --draft creates a draft release. Failures distinguish build, version/tag, and publication steps.

#### Release Version Resolution

| behavior | on_failure | source_rule |
| --- | --- | --- |
| Read the configured file path without executing a command or evaluating an expression. Relative paths use the working directory. | Version-resolution failures return `2`; a version-source error during a build returns `6`. | `release.versionSource` is a file path |
| Read a YAML version line or JSON top-level version or Python project.version or Rust package.version from Cargo metadata. | Unsupported formats fail version resolution with `2`; build-stage errors return `6`. | Supported formats are YAML JSON and TOML |
| Python rejects dynamic versions; Rust resolves workspace inheritance from a package Cargo.toml; YAML reads the first version line (including tags.version); JSON requires a top-level version. | Missing or invalid versions fail resolution with `2`; build-stage errors return `6`. Virtual Rust workspaces require a member manifest. | Static version field is required |
| The extracted version is validated as a semantic version string such as `1.2.3` or `1.2.3-rc.1`. | Invalid semantic versions fail resolution with `2`; build-stage errors return `6`. | Semantic version required |
| The release tag is computed as `release.tagPrefix + version`. | Fail with exit code `7` if tag creation or release publication cannot proceed. | Tag derivation |
| If the computed tag already exists the command fails rather than mutate the existing release implicitly. | Fail with exit code `7` and explain that the tag already exists. | Existing tag conflict |

### 12 Init

What repo bootstrap does.

#### Init Command

Create .gh-flarebyte.cue in the working directory with imported GitHub repository metadata when available and Go build/release defaults. If metadata cannot be read, print a warning and use defaults. An existing file is rejected unless --overwrite is passed. Init writes local configuration only; repo update performs remote mutations. Success reports the created path, whether metadata was imported, and the next review/update step.

### 13 Update

What reconciliation from cue config means.

#### Update Command

Load local configuration and GitHub state, compute the settings/topics/labels update plan, and check visibility and deletion gates before applying changes. Visibility changes require --accept-visibility-change-consequences; topic or label deletions require --confirm-deletions. Settings, topics, and labels are applied sequentially. The command stops on the first remote error without rollback. Success reports settings changes and configured topic/label counts; failures report the cause and available partial-progress context.

### 14 Audit

What read-only drift checking means.

#### Audit Command

Compare local configuration with GitHub without changing remote state. Plain output reports the number of differences and suggests repo update, or states that no drift was found. --json emits the repository, drift count, and individual diffs. Exit status is 0 for no drift and 3 when drift exists.

### 15 Repos Mine

What repository discovery returns.

#### Repos Mine Command

List repositories the authenticated user contributes to in the organization required by --org. Plain output includes the organization, contributor, count, and repository details. --json emits the same information as a structured report. This command reads GitHub data without changing remote state.

### 16 GitHub Flags

The existing `gh repo edit` knobs that `gh flarebyte repo update` applies from config.

#### Existing gh Repo Edit Flags

| alias | description | disable_syntax | flag | value_type |
| --- | --- | --- | --- | --- |
|  | Accept consequences of changing repository visibility |  | --accept-visibility-change-consequences | boolean |
|  | Add repository topic |  | --add-topic | strings |
|  | Allow forking of an organization repository | --allow-forking=false | --allow-forking | boolean |
|  | Allow PR head branch behind base branch to be updated | --allow-update-branch=false | --allow-update-branch | boolean |
|  | Set default branch name |  | --default-branch | name |
|  | Delete head branch when PRs are merged | --delete-branch-on-merge=false | --delete-branch-on-merge | boolean |
| -d | Repository description |  | --description | string |
|  | Enable GitHub Advanced Security | --enable-advanced-security=false | --enable-advanced-security | boolean |
|  | Enable auto-merge | --enable-auto-merge=false | --enable-auto-merge | boolean |
|  | Enable discussions | --enable-discussions=false | --enable-discussions | boolean |
|  | Enable issues | --enable-issues=false | --enable-issues | boolean |
|  | Enable merge commits for PRs | --enable-merge-commit=false | --enable-merge-commit | boolean |
|  | Enable projects | --enable-projects=false | --enable-projects | boolean |
|  | Enable rebase merging for PRs | --enable-rebase-merge=false | --enable-rebase-merge | boolean |
|  | Enable secret scanning | --enable-secret-scanning=false | --enable-secret-scanning | boolean |
|  | Enable secret scanning push protection | --enable-secret-scanning-push-protection=false | --enable-secret-scanning-push-protection | boolean |
|  | Enable squash merging for PRs | --enable-squash-merge=false | --enable-squash-merge | boolean |
|  | Enable wiki | --enable-wiki=false | --enable-wiki | boolean |
| -h | Repository homepage URL |  | --homepage | URL |
|  | Remove repository topic |  | --remove-topic | strings |
|  | Default squash merge commit message behavior: default\|pr-title\|pr-title-commits\|pr-title-description |  | --squash-merge-commit-message | enum |
|  | Make repository available as a template repository | --template=false | --template | boolean |
|  | Change visibility: public\|private\|internal | requires --accept-visibility-change-consequences | --visibility | enum |

## 04 Existing GitHub Behaviour

GitHub behaviors and command shapes that are examples of the external system, not new extension commands.

### 01 Labels

Label lifecycle and bulk label conventions that the extension synchronizes from cue config.

#### Existing Label Behaviour

| command | important_flags | mode | notes | purpose |
| --- | --- | --- | --- | --- |
| gh label create | --color --description | write | Use force when reapplying an existing label. | create a new label with color and description |
| gh label list | --json | read | Useful for scripting and filtering with jq. | list labels for a repository |
| gh label view | none | read | Shows the current label state. | inspect a single label |
| gh label edit | --name --color --description | write | Supports atomic updates. | rename or recolor a label |
| gh label delete | --yes | write | Keep scripted use non-interactive. | remove a label |

### 02 Releases

Release creation and publication flows that remain existing gh behavior.

#### Existing Release Behaviour

| command | important_flags | mode | notes | purpose |
| --- | --- | --- | --- | --- |
| gh release create | --draft --prerelease --generate-notes | write | Supports notes from file or stdin. | create a release and optionally upload assets |
| gh release upload | --clobber | write | Can replace an existing asset with the same name. | attach assets to an existing release |
| gh release edit | --draft=false | write | Useful for publishing a reviewed draft. | change release state after creation |
| gh release create --discussion-category | --discussion-category | write | Keeps release conversation attached. | open a discussion with the release |
| gh release create --verify-tag | --verify-tag | write | Useful for protected publication flows. | require the release tag to exist already |
| Python package release | --draft --notes-file | write | Uses shared release fields and fresh artifact paths rather than scanning stale files. No PyPI upload. | build fresh wheel and source distribution then create a GitHub release |
| Rust release | --draft --notes-file | write | Uses shared release fields and fresh assets including optional checksums. Library releases have no artifacts. No crates.io upload. | build fresh target binaries or verified crate packages then create a GitHub release |
| Swift release | --draft --notes-file | write | Uses shared YAML/JSON version source and fresh asset staging. Library releases have no assets. No registry upload or signing. | build fresh native executable artifacts or verify a library then create a GitHub release |
| TypeScript release | --draft --notes-file | write | Uses package.json top-level version and shared release policies. Libraries have no assets; CLIs stage only fresh executables and checksums. No npm publishing. | verify a library or build a fresh native CLI then create a GitHub release |
| Kotlin release | --draft --notes-file | write | Uses shared YAML/JSON version sources and fresh ZIP assets with optional checksums. Library releases have no assets. No Maven Central publishing. | verify a JVM library or build a fresh JVM CLI distribution then create a GitHub release |

### 03 Search

Search and content discovery shapes that are examples of current GitHub usage.

#### Existing Search Shapes

```ts
export type SearchQuery = {
  org: string;
  topic?: string;
  language?: string;
  archived?: boolean;
};

export type CodeSearchRequest = {
  org: string;
  filename: string;
  includeContent: boolean;
};

export type RepoContentRequest = {
  repo: string;
  path: string;
  ref?: string;
};
```

## 05 Discovery

Repo discovery for the 'repos mine' workflow, shown as an existing GitHub capability.

### 01 Repos Mine

How the extension discovers repositories the user contributes to by leaning on existing GitHub data.

#### Existing Discovery Shape

```ts
export type RepoDiscovery = {
  org: string;
  cursor: string | null;
  pageSize: number;
  scope: "viewer.repositoriesContributedTo";
};

export const repoDiscovery: RepoDiscovery = {
  org: "my-org",
  cursor: null,
  pageSize: 100,
  scope: "viewer.repositoriesContributedTo",
};
```

## 06 Current Limitations

Boundaries enforced by the current implementation.

### 01 Supported Boundaries

Unsupported features and automation requirements.

#### Current Limitations

Repository sync is push-only. Unattended updates require the same explicit deletion and visibility flags as interactive use. Python dynamic versions and native wheel target matrices, package-registry publication, and custom Rust target triples are unsupported. Release notes support generate-notes, notes-from-tag, and notes-file; no template engine is configured.
