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
| sync.mode | enum | extension | Push-only for now. Remote state is audited but not imported back into config. | read-only |
| repository.description | string | .gh-flarebyte.cue | Primary repo metadata field. | local-to-remote |
| repository.defaultBranch | string | .gh-flarebyte.cue | Usually `main` for flarebyte repositories. | local-to-remote |
| repository.homepage | string | .gh-flarebyte.cue | Repository homepage URL. | local-to-remote |
| repository.visibility | enum | .gh-flarebyte.cue | Repository visibility: public, private, or internal. | local-to-remote |
| repository.template | boolean | .gh-flarebyte.cue | Make the repository available as a template. | local-to-remote |
| repository.topics | list | .gh-flarebyte.cue | Topics are managed as the complete desired set. Missing remote topics are deletions. | local-to-remote |
| repository.labels | list | .gh-flarebyte.cue | Labels are managed as the complete desired set by label name. Missing remote labels are deletions, and renaming a label is modeled as delete-plus-create. | local-to-remote |
| repository.features.issues | boolean | .gh-flarebyte.cue | GitHub issues enabled or disabled. | local-to-remote |
| repository.features.wiki | boolean | .gh-flarebyte.cue | GitHub wiki enabled or disabled. | local-to-remote |
| repository.features.projects | boolean | .gh-flarebyte.cue | GitHub projects enabled or disabled. | local-to-remote |
| repository.features.discussions | boolean | .gh-flarebyte.cue | GitHub discussions enabled or disabled. | local-to-remote |
| repository.features.autoMerge | boolean | .gh-flarebyte.cue | Auto-merge policy. | local-to-remote |
| repository.features.mergeCommit | boolean | .gh-flarebyte.cue | Merge commit policy. | local-to-remote |
| repository.features.rebaseMerge | boolean | .gh-flarebyte.cue | Rebase merge policy. | local-to-remote |
| repository.features.squashMerge | boolean | .gh-flarebyte.cue | Squash merge policy. | local-to-remote |
| repository.features.squashMergeCommitMessage | enum | .gh-flarebyte.cue | Default squash merge commit message behavior. | local-to-remote |
| repository.features.deleteBranchOnMerge | boolean | .gh-flarebyte.cue | Head branch cleanup policy. | local-to-remote |
| repository.features.allowForking | boolean | .gh-flarebyte.cue | Forking policy for organization repos. | local-to-remote |
| repository.features.allowUpdateBranch | boolean | .gh-flarebyte.cue | Allow PR head branch updates when behind the base branch. | local-to-remote |
| repository.features.advancedSecurity | boolean | .gh-flarebyte.cue | GitHub Advanced Security. | local-to-remote |
| repository.features.secretScanning | boolean | .gh-flarebyte.cue | Secret scanning for the repository. | local-to-remote |
| repository.features.secretScanningPushProtection | boolean | .gh-flarebyte.cue | Secret scanning push protection. | local-to-remote |
| build.language | enum | .gh-flarebyte.cue | Build language used by gh flarebyte build: go initially, dart later. | local-only |
| build.mode | enum | .gh-flarebyte.cue | Build mode: `binary` for target artifacts or `library` for package compile/test verification. | local-only |
| build.packages | list | .gh-flarebyte.cue | Package patterns used in `library` mode, defaulting to `["./..."]`. | local-only |
| build.runTests | boolean | .gh-flarebyte.cue | Whether library mode runs `go test` after `go build`. | local-only |
| build.outputDir | string | .gh-flarebyte.cue | Directory for built artifacts. | local-only |
| build.checksumFile | string | .gh-flarebyte.cue | Checksum manifest for built artifacts. | local-only |
| build.targets | list | .gh-flarebyte.cue | Target matrix for the build command using `os-arch` strings such as `linux-amd64` or `windows-amd64`. Each project declares only the targets it supports. | local-only |
| go.cgo.enabled | boolean | .gh-flarebyte.cue | Explicit CGO mode for Go builds. When true build sets `CGO_ENABLED=1`; when false build sets `CGO_ENABLED=0`. | local-only |
| go.cgo.cc | string | .gh-flarebyte.cue | Optional C compiler override for CGO builds. Requires `go.cgo.enabled: true`. | local-only |
| go.cgo.cxx | string | .gh-flarebyte.cue | Optional C++ compiler override for CGO builds. Requires `go.cgo.enabled: true`. | local-only |
| release.versionSource | string | .gh-flarebyte.cue | File or source that provides the release version. | local-only |
| release.tagPrefix | string | .gh-flarebyte.cue | Prefix used for release tags. | local-only |
| release.notesMode | enum | .gh-flarebyte.cue | Release note strategy, such as generate-notes or notes-file. | local-only |
| release.releaseNotesFilePath | string | .gh-flarebyte.cue | Path required when notesMode is `notes-file`. | local-only |
| release.includeArtifacts | boolean | .gh-flarebyte.cue | Whether release uploads build artifacts or publishes tag/notes only. | local-only |
| release.artifactDir | string | .gh-flarebyte.cue | Directory scanned for release assets. | local-only |
| release.includeChecksums | boolean | .gh-flarebyte.cue | Whether to include the checksum manifest as a release asset. | local-only |

### 03 Topics

Repository topics are a flat sync target in the cue config.

#### Topic Sync

Topics are kept as a flat string list in the cue config and synchronized as the complete desired repository topics set. If applying the config would delete remote topics, `gh flarebyte repo update` should fail unless the user explicitly confirms deletions.

### 04 Labels

Repository labels have their own structured sync shape in the cue config.

#### Label Sync

Labels use a structured list of objects in the cue config so name, color, and description can be reconciled separately from repository topics. The config represents the complete desired label set; labels missing from config should be treated as deletions and require explicit confirmation before update. Because labels are keyed by name, renaming a label is modeled explicitly as delete-plus-create.

### 05 Build

Build language selection for gh flarebyte build.

#### Build Config

Build orchestration is driven by a top-level `build` block in the Cue config. `gh flarebyte build` reads the configured language and uses a Go implementation initially, with Dart allowed later as a supported language. In `binary` mode the command writes per-target binaries and archives under the configured output directory and emits the checksum manifest. In `library` mode the command performs compile verification across configured packages (`go build`) and optionally runs tests (`go test`) without producing binary artifacts. Targets are expressed as `os-arch` strings such as `linux-amd64` or `windows-amd64`; in library mode `--target` maps to `GOOS/GOARCH` validation.

### 06 Release

Release publication settings for gh flarebyte release.

#### Release Config

Release publication is driven by a top-level `release` block in the Cue config. `gh flarebyte release` uses the configured release tag prefix, source version, artifact policy, and release notes policy to publish a GitHub release. When `includeArtifacts` is true it uploads files from `artifactDir`; when false it publishes tag and notes without asset uploads. `releaseNotesFilePath` is required only when `notesMode` is `notes-file`. The implementation treats `release.versionSource` as a repository-local YAML or JSON file path containing a top-level `version` string.

### 07 Sync Types

TypeScript shapes for the sync contract.

#### Sync Types

```ts
export type SyncMode = "push";

export type RepositoryFeatures = {
  issues: boolean;
  wiki: boolean;
  projects: boolean;
  discussions: boolean;
  autoMerge: boolean;
  mergeCommit: boolean;
  rebaseMerge: boolean;
  squashMerge: boolean;
  squashMergeCommitMessage: "default" | "pr-title" | "pr-title-commits" | "pr-title-description";
  deleteBranchOnMerge: boolean;
  allowForking: boolean;
  allowUpdateBranch: boolean;
  advancedSecurity: boolean;
  secretScanning: boolean;
  secretScanningPushProtection: boolean;
};

export type LabelConfig = {
  name: string;
  color: string;
  description: string;
};

export type BuildConfig =
  | {
      language: "go" | "dart";
      mode: "binary";
      outputDir: string;
      checksumFile: string;
      targets: BuildTarget[];
      artifactTargetSuffix?: boolean;
    }
  | {
      language: "go" | "dart";
      mode: "library";
      packages: string[];
      runTests?: boolean;
    };

export type BuildTarget = `${"linux" | "darwin" | "windows"}-${"amd64" | "arm64"}`;

export type ReleaseConfigBase = {
  versionSource: string;
  tagPrefix: string;
  includeArtifacts: boolean;
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
  features: RepositoryFeatures;
};

export type GhFlarebyteConfig = {
  project: ProjectConfig;
  sync: SyncPlan;
  repository: RepositoryConfig;
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

The Cue sync config now models homepage, visibility, template, advanced security, secret scanning, push protection, allow-update-branch, and squash-merge commit-message settings.

## 03 Commands

User-facing extension commands and the config-driven sync path.

### 01 Command Matrix

User-facing extension actions and their purpose.

#### Extension Command Matrix

| command | config_touchpoints | output | purpose | read_write |
| --- | --- | --- | --- | --- |
| gh flarebyte build | build.* -> binary artifacts or compile/test verification | language-specific build output | build the project according to the configured language and build mode | write |
| gh flarebyte release | build.* and release.* -> GitHub release with optional assets | versioned GitHub release | run build then publish a GitHub release from the configured release settings | write |
| gh flarebyte repo init | .gh-flarebyte.cue created or seeded | initialized repo state | bootstrap a repo-local config and initial GitHub defaults | write |
| gh flarebyte repo update | .gh-flarebyte.cue -> GitHub repo state with `--confirm-deletions` and `--accept-visibility-change-consequences` when needed | updated remote repo state | reconcile GitHub repo metadata from local config with explicit safety flags | write |
| gh flarebyte repo audit | .gh-flarebyte.cue and GitHub state | drift report | compare local config with remote GitHub state | read |
| gh flarebyte repos mine | none | relevant repo list | list repositories the user contributes to within an org | read |

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
    command: "gh flarebyte build --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Build local artifacts from the top-level build block.",
    syncEffect: "Write versioned binaries and checksums under the configured output directory.",
  },
  {
    name: "release",
    command: "gh flarebyte release --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Build first, then publish a GitHub release from the configured release block.",
    syncEffect: "Run the build flow and upload release assets to GitHub.",
  },
  {
    name: "init",
    command: "gh flarebyte repo init --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Bootstrap repo-local config and seed the GitHub repo settings plus local build and release defaults.",
    syncEffect: "Create or update `.gh-flarebyte.cue` and apply initial GitHub defaults.",
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

CLI flags that define the initial command surface for the extension.

#### Extension Command Flags

| automation_mode | command | flag | kind | purpose | required_when |
| --- | --- | --- | --- | --- | --- |
| script-friendly | gh flarebyte | --version | boolean | Print CLI version metadata including version, commitId, and build date. | optional |
| script-friendly | gh flarebyte | --json | boolean | When combined with `--version`, print version metadata as JSON instead of plain text. | optional |
| safety-gate | gh flarebyte repo init | --overwrite | boolean | Replace an existing `.gh-flarebyte.cue` file instead of failing. | optional |
| script-friendly | gh flarebyte build | --repo | string | Identify the repository whose checked-in config drives the build. | always |
| script-friendly | gh flarebyte build | --target | string | Limit the build to one configured target such as `linux-amd64`. | optional |
| script-friendly | gh flarebyte build | --output-dir | string | Override the configured output directory for one invocation. | optional |
| script-friendly | gh flarebyte release | --repo | string | Identify the repository whose checked-in config drives the release. | always |
| script-friendly | gh flarebyte release | --notes-file | string | Provide release notes explicitly for one invocation. | required when `release.notesMode` is `notes-file` and the config path is absent |
| script-friendly | gh flarebyte release | --draft | boolean | Publish the release as a draft instead of a final release. | optional |
| script-friendly | gh flarebyte repo update | --repo | string | Identify the repository whose local config should be pushed to GitHub. | always |
| safety-gate | gh flarebyte repo update | --confirm-deletions | boolean | Allow exact-set reconciliation to remove remote topics or labels missing from config. | required when topic or label deletions are detected |
| safety-gate | gh flarebyte repo update | --accept-visibility-change-consequences | boolean | Allow visibility transitions that need explicit acknowledgement. | required when repository visibility changes |
| script-friendly | gh flarebyte repo audit | --json | boolean | Emit the audit report as stable machine-readable JSON. | optional |
| script-friendly | gh flarebyte repos mine | --json | boolean | Emit discovered repositories as stable machine-readable JSON. | optional |

### 04 Automation

Which flags are safe for scripts and which are explicit safety gates.

#### Automation Policy

Flags marked `script-friendly` are intended for repeatable local scripts and CI jobs. Flags marked `safety-gate` exist to make destructive or high-consequence changes explicit. In practice, `gh flarebyte repo update` should fail in unattended runs when deletions or visibility changes are detected unless the corresponding safety flag is passed deliberately.

### 05 Guidance And Errors

How the CLI should help users succeed and recover from mistakes.

#### Guidance And Error Feedback

| example_feedback | expectation | scenario |
| --- | --- | --- |
| Run `gh flarebyte repo update --help` to see required safety flags and examples. | Every command should provide discoverable usage guidance via `--help` with a short purpose line and the key flags. | general help |
| `.gh-flarebyte.cue already exists in /path/to/repo. Re-run with --overwrite if replacing it is intentional.` | If `repo init` finds an existing `.gh-flarebyte.cue` file it should fail safely and explain the overwrite option. | init overwrite blocked |
| `No .gh-flarebyte.cue found in /path/to/repo. Run gh flarebyte repo init or pass --repo to target a managed repository.` | If `.gh-flarebyte.cue` is missing or unreadable the CLI should explain what file was expected and which command should create or fix it. | missing config |
| `Invalid build.targets entry "linux/x64". Expected os-arch format such as linux-amd64 or windows-amd64.` | If config parsing or validation fails the CLI should identify the field and why it is invalid. | config validation error |
| `Update would delete 2 labels and 1 topic. Re-run with --confirm-deletions if that is intentional.` | If update detects topic or label deletions without confirmation the CLI should explain what would be deleted and which flag is required. | deletion blocked |
| `Visibility would change from public to private. Re-run with --accept-visibility-change-consequences if that is intentional.` | If update detects a visibility change without acknowledgement the CLI should explain the current and target visibility and the required flag. | visibility blocked |
| `Build failed for windows-amd64 during go build. Re-run with --target windows-amd64 to isolate the failure.` | If build fails the CLI should identify the target and failing step and point to the next debugging action. | build failure |
| `Build completed, but release upload failed for tag v1.2.3. Check GitHub authentication and retry gh flarebyte release.` | If release fails after build the CLI should say whether artifacts were built successfully and which release step failed. | release failure |
| `Update stopped after repository settings and topics were applied. Label sync failed, and no rollback was attempted. Fix the error and rerun gh flarebyte repo update.` | If `repo update` fails after applying some changes it should say what was already applied and that no rollback occurred. | partial update failure |
| `3 differences found. Review the drift report below, then run gh flarebyte repo update when ready to apply changes.` | Audit output should summarize drift clearly and point to the next command to resolve it. | drift report |

### 06 Success Output

How successful commands should report outcomes and next steps.

#### Success Output

| example_output | expectation | scenario |
| --- | --- | --- |
| `Repository settings updated successfully.` | Successful commands should confirm what was done in plain language, not just exit silently. | general success |
| `gh-flarebyte v1.2.3 commitId=a1b2c3d4 date=2026-04-30T09:15:00Z os=darwin arch=arm64 goVersion=go1.25.0` | The root `--version` flag should print structured build metadata rather than only a semantic version string. | version success |
| `{"version":"v1.2.3","commitId":"a1b2c3d4","date":"2026-04-30T09:15:00Z","os":"darwin","arch":"arm64","goVersion":"go1.25.0"}` | When `--version --json` is requested the CLI should emit machine-readable version metadata using the documented shape. | version json success |
| `Created .gh-flarebyte.cue in /path/to/repo from current GitHub state. Next: review the config, then run gh flarebyte repo update.` | Init should say where the config file was created or updated, whether current GitHub state was imported, and what the next command is. | init success |
| `Update complete: 3 repo settings updated, 4 topics synced, 8 labels reconciled.` | Update should summarize what changed, including counts for topics, labels, and repo settings when relevant. | update success |
| `No drift found. GitHub matches .gh-flarebyte.cue.` | Audit should summarize whether drift exists and what the user should do next. | audit success |
| `Build complete: 3 targets written to build/ with checksums in build/checksums.txt.` | Build should summarize the targets produced and where artifacts were written. | build success |
| `Release v1.2.3 published from build with checksums attached.` | Release should summarize the tag published and whether assets were uploaded. | release success |
| `Found 12 repositories for contributor olivier in flarebyte.` | Repos mine should report how many repositories were found for the requested organization. | discovery success |

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

The root `gh flarebyte --version` command should print concise human-readable plain text by default. When `--json` is passed alongside `--version`, it should emit machine-readable JSON using the documented version metadata shape. This keeps the default friendly for humans while making automation explicit and stable.

### 10 Build

How build orchestration is driven from config.

#### Build Command

Build the project from the top-level `build` block. Start with Go only, but keep the config shape open for Dart so the command can grow without changing its contract. Support `build.mode: "binary"` for deterministic target artifacts and `build.mode: "library"` for package compile verification. In binary mode, target names are expressed as `os-arch` strings such as `linux-amd64` or `windows-amd64` and driven from config rather than shell scripts. Unix targets are packaged as `tar.gz`, Windows targets as `zip`, Windows binaries end in `.exe`, and checksums use SHA-256. In library mode, compile configured package patterns with `go build`, optionally run `go test`, and do not require a synthetic single executable artifact. CGO behavior is driven by `go.cgo` using camelCase fields only (`enabled`, `cc`, `cxx`): when enabled, build uses `CGO_ENABLED=1`; when disabled, it uses `CGO_ENABLED=0` and should fail with a clear policy error if CGO-backed dependencies are detected. Build output should also embed version metadata so the compiled CLI can report `version`, `commitId`, `date`, and related runtime details via `--version`, and the same metadata should be available as JSON with `--version --json`. When the command fails it should report the target or mode, effective CGO settings, failing command, and next useful action.

#### Build Artifact Rules

| binary_name | checksum_algorithm | notes | package_format | target |
| --- | --- | --- | --- | --- |
| gh-flarebyte-linux-amd64 | sha256 | Unix targets should be published as compressed archives containing the binary. | tar.gz | linux-amd64 |
| gh-flarebyte-darwin-arm64 | sha256 | macOS targets should be published as compressed archives containing the binary. | tar.gz | darwin-arm64 |
| gh-flarebyte-windows-amd64.exe | sha256 | Windows targets should use `.exe` binaries and zip packaging. | zip | windows-amd64 |

### 11 Release

How release publication is driven from config.

#### Release Command

Run `gh flarebyte build` first, then publish a GitHub release from the configured release policy. Use the top-level `release` block to choose the tag, whether artifacts are included, and release note behavior, requiring `releaseNotesFilePath` when `notesMode` is `notes-file`. When `includeArtifacts` is true, upload build outputs from `artifactDir`; when false, publish tag and notes only. Resolve the version from a repository-local YAML or JSON file containing a top-level `version` string, derive the tag as `tagPrefix + version`, and fail if the target tag already exists. On failure, the CLI should distinguish between build failure, tag/version resolution failure, and release upload failure. On success it should confirm the published tag and whether assets were uploaded.

#### Release Version Resolution

| behavior | on_failure | source_rule |
| --- | --- | --- |
| The initial implementation should read a repository-local file path rather than an arbitrary command or expression. | Fail with exit code `2` if the path is missing or unreadable. | `release.versionSource` is a file path |
| The initial implementation should parse YAML or JSON only. | Fail with exit code `2` if the file format is unsupported. | Supported format is YAML or JSON |
| The initial implementation should extract a single top-level string field named `version`. | Fail with exit code `2` if `version` is missing or not a string. | Top-level `version` field is required |
| The extracted version should already be normalized as a semantic version string such as `1.2.3` or `1.2.3-rc.1`. | Fail with exit code `2` if the version string is invalid. | Semantic version required |
| The release tag should be computed as `release.tagPrefix + version`. | Fail with exit code `7` if tag creation or release publication cannot proceed. | Tag derivation |
| If the computed tag already exists the command should fail rather than mutate the existing release implicitly. | Fail with exit code `7` and explain that the tag already exists. | Existing tag conflict |

### 12 Init

What repo bootstrap does.

#### Init Command

Bootstrap a repository by seeding `.gh-flarebyte.cue` with repository, build, and release defaults and then applying the initial syncable repo settings. The initial implementation should import current GitHub repository state into the generated config where available, then fill remaining fields from flarebyte defaults. If `.gh-flarebyte.cue` already exists, the command should fail by default rather than merge implicitly; replacement should require `--overwrite`. The command should explain what file it created or updated and point users to `gh flarebyte repo update --help` for the next step. Success output should make the next action obvious.

### 13 Update

What reconciliation from cue config means.

#### Update Command

Reconcile the live GitHub repository from `.gh-flarebyte.cue`, including repo settings, topics, and label definitions. Topics and labels are exact-set sync targets, so remote items missing from config should be treated as deletions. The command must fail unless the user explicitly confirms deletions with `--confirm-deletions`. Visibility changes should also require explicit CLI confirmation with `--accept-visibility-change-consequences` rather than a committed config flag. The command should compute and validate the full plan before mutating GitHub, then apply changes sequentially and stop at the first remote mutation failure. It should not attempt rollback for changes already applied; instead it should report partial progress clearly so the user can fix the issue and rerun. Failure output should explain what would change, why it was blocked, and the exact next command or flag to use. Success output should summarize what changed rather than only saying the command succeeded.

### 14 Audit

What read-only drift checking means.

#### Audit Command

Compare the checked-in Cue config with GitHub and report drift without changing remote state. Output should summarize the number of differences and point users to `gh flarebyte repo update` when remediation is appropriate. A clean run should say clearly that no drift was found. With `--json`, the command should emit a stable machine-readable report of the repo, drift count, and individual diffs. The command should exit `0` for no drift and `3` when drift is found.

### 15 Repos Mine

What repository discovery returns.

#### Repos Mine Command

List repositories the current user contributes to within an organization so the extension can discover target repos before sync. Success output should include the organization queried and how many repositories were found. With `--json`, the command should emit a stable machine-readable report containing the requested organization, contributor, count, and discovered repositories.

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

## 06 Open Questions

Pending decisions that should stay visible in the spec.

### 01 Decisions Pending

What still needs agreement before the sync contract hardens.

#### Open Questions

Clarify the non-interactive CI story for deletion confirmation, the first Dart build contract once it lands, and whether release notes should eventually support templates beyond generated notes and a single notes file path.
