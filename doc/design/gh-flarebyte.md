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
	managedFields: [
		"description",
		"defaultBranch",
		"homepage",
		"visibility",
		"template",
		"topics",
		"labels",
		"features.issues",
		"features.wiki",
		"features.projects",
		"features.discussions",
		"features.autoMerge",
		"features.mergeCommit",
		"features.rebaseMerge",
		"features.squashMerge",
		"features.squashMergeCommitMessage",
		"features.deleteBranchOnMerge",
		"features.allowForking",
		"features.allowUpdateBranch",
		"features.advancedSecurity",
		"features.secretScanning",
		"features.secretScanningPushProtection",
	]
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
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
		"darwin-arm64",
		"windows-amd64",
	]
}

release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
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
| repository.labels | list | .gh-flarebyte.cue | Labels are managed as the complete desired set by label name. Missing remote labels are deletions. | local-to-remote |
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
| build.outputDir | string | .gh-flarebyte.cue | Directory for built artifacts. | local-only |
| build.checksumFile | string | .gh-flarebyte.cue | Checksum manifest for built artifacts. | local-only |
| build.targets | list | .gh-flarebyte.cue | Target matrix for the build command using `os-arch` strings such as `linux-amd64` or `windows-amd64`. Each project declares only the targets it supports. | local-only |
| release.versionSource | string | .gh-flarebyte.cue | File or source that provides the release version. | local-only |
| release.tagPrefix | string | .gh-flarebyte.cue | Prefix used for release tags. | local-only |
| release.notesMode | enum | .gh-flarebyte.cue | Release note strategy, such as generate-notes or notes-file. | local-only |
| release.releaseNotesFilePath | string | .gh-flarebyte.cue | Path required when notesMode is `notes-file`. | local-only |
| release.artifactDir | string | .gh-flarebyte.cue | Directory scanned for release assets. | local-only |
| release.includeChecksums | boolean | .gh-flarebyte.cue | Whether to include the checksum manifest as a release asset. | local-only |

### 03 Topics

Repository topics are a flat sync target in the cue config.

#### Topic Sync

Topics are kept as a flat string list in the cue config and synchronized as the complete desired repository topics set. If applying the config would delete remote topics, `gh flarebyte repo update` should fail unless the user explicitly confirms deletions.

### 04 Labels

Repository labels have their own structured sync shape in the cue config.

#### Label Sync

Labels use a structured list of objects in the cue config so name, color, and description can be reconciled separately from repository topics. The config represents the complete desired label set; labels missing from config should be treated as deletions and require explicit confirmation before update.

### 05 Build

Build language selection for gh flarebyte build.

#### Build Config

Build orchestration is driven by a top-level `build` block in the Cue config. `gh flarebyte build` reads the configured language and uses a Go implementation initially, with Dart allowed later as a supported language. The command should write language-specific binaries under the configured output directory, emit the configured checksum manifest, and keep the output layout stable across languages. Targets are expressed as `os-arch` strings such as `linux-amd64` or `windows-amd64`, and each project lists only the targets it supports.

### 06 Release

Release publication settings for gh flarebyte release.

#### Release Config

Release publication is driven by a top-level `release` block in the Cue config. `gh flarebyte release` should use the configured release tag prefix, source version, artifact layout, and release notes policy to publish a GitHub release from the build outputs. `releaseNotesFilePath` is required only when `notesMode` is `notes-file`.

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

export type BuildConfig = {
  language: "go" | "dart";
  outputDir: string;
  checksumFile: string;
  targets: BuildTarget[];
};

export type BuildTarget = `${"linux" | "darwin" | "windows"}-${"amd64" | "arm64"}`;

export type ReleaseConfigBase = {
  versionSource: string;
  tagPrefix: string;
  artifactDir: string;
  includeChecksums: boolean;
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
  managedFields: string[];
  dryRun: boolean;
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
| gh flarebyte build | build.* -> build artifacts | language-specific build output | build the project according to the configured language and target matrix | write |
| gh flarebyte release | build.* and release.* -> GitHub release assets | versioned GitHub release | run build then publish a GitHub release from the configured release settings | write |
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
| script-friendly | gh flarebyte build | --repo | string | Identify the repository whose checked-in config drives the build. | always |
| script-friendly | gh flarebyte build | --target | string | Limit the build to one configured target such as `linux-amd64`. | optional |
| script-friendly | gh flarebyte build | --output-dir | string | Override the configured output directory for one invocation. | optional |
| script-friendly | gh flarebyte release | --repo | string | Identify the repository whose checked-in config drives the release. | always |
| script-friendly | gh flarebyte release | --notes-file | string | Provide release notes explicitly for one invocation. | required when `release.notesMode` is `notes-file` and the config path is absent |
| script-friendly | gh flarebyte release | --draft | boolean | Publish the release as a draft instead of a final release. | optional |
| script-friendly | gh flarebyte repo update | --repo | string | Identify the repository whose local config should be pushed to GitHub. | always |
| safety-gate | gh flarebyte repo update | --confirm-deletions | boolean | Allow exact-set reconciliation to remove remote topics or labels missing from config. | required when topic or label deletions are detected |
| safety-gate | gh flarebyte repo update | --accept-visibility-change-consequences | boolean | Allow visibility transitions that need explicit acknowledgement. | required when repository visibility changes |

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
| `No .gh-flarebyte.cue found in /path/to/repo. Run gh flarebyte repo init or pass --repo to target a managed repository.` | If `.gh-flarebyte.cue` is missing or unreadable the CLI should explain what file was expected and which command should create or fix it. | missing config |
| `Invalid build.targets entry "linux/x64". Expected os-arch format such as linux-amd64 or windows-amd64.` | If config parsing or validation fails the CLI should identify the field and why it is invalid. | config validation error |
| `Update would delete 2 labels and 1 topic. Re-run with --confirm-deletions if that is intentional.` | If update detects topic or label deletions without confirmation the CLI should explain what would be deleted and which flag is required. | deletion blocked |
| `Visibility would change from public to private. Re-run with --accept-visibility-change-consequences if that is intentional.` | If update detects a visibility change without acknowledgement the CLI should explain the current and target visibility and the required flag. | visibility blocked |
| `Build failed for windows-amd64 during go build. Re-run with --target windows-amd64 to isolate the failure.` | If build fails the CLI should identify the target and failing step and point to the next debugging action. | build failure |
| `Build completed, but release upload failed for tag v1.2.3. Check GitHub authentication and retry gh flarebyte release.` | If release fails after build the CLI should say whether artifacts were built successfully and which release step failed. | release failure |
| `3 differences found. Review the drift report below, then run gh flarebyte repo update when ready to apply changes.` | Audit output should summarize drift clearly and point to the next command to resolve it. | drift report |

### 06 Success Output

How successful commands should report outcomes and next steps.

#### Success Output

| example_output | expectation | scenario |
| --- | --- | --- |
| `Repository settings updated successfully.` | Successful commands should confirm what was done in plain language, not just exit silently. | general success |
| `gh-flarebyte v1.2.3 commitId=a1b2c3d4 date=2026-04-30T09:15:00Z os=darwin arch=arm64 goVersion=go1.25.0` | The root `--version` flag should print structured build metadata rather than only a semantic version string. | version success |
| `{"version":"v1.2.3","commitId":"a1b2c3d4","date":"2026-04-30T09:15:00Z","os":"darwin","arch":"arm64","goVersion":"go1.25.0"}` | When `--version --json` is requested the CLI should emit machine-readable version metadata using the documented shape. | version json success |
| `Created .gh-flarebyte.cue in /path/to/repo. Next: review the config, then run gh flarebyte repo update.` | Init should say where the config file was created or updated and what the next command is. | init success |
| `Update complete: 3 repo settings updated, 4 topics synced, 8 labels reconciled.` | Update should summarize what changed, including counts for topics, labels, and repo settings when relevant. | update success |
| `No drift found. GitHub matches .gh-flarebyte.cue.` | Audit should summarize whether drift exists and what the user should do next. | audit success |
| `Build complete: 3 targets written to build/ with checksums in build/checksums.txt.` | Build should summarize the targets produced and where artifacts were written. | build success |
| `Release v1.2.3 published from build/ with checksums attached.` | Release should summarize the tag published and the asset source directory. | release success |
| `Found 12 repositories for contributor olivier in flarebyte.` | Repos mine should report how many repositories were found for the requested organization. | discovery success |

### 07 Version

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

### 08 Build

How build orchestration is driven from config.

#### Build Command

Build the project from the top-level `build` block. Start with Go only, but keep the config shape open for Dart so the command can grow without changing its contract. The first implementation should produce `<outputDir>/<name>-<target>` artifacts and a configured checksum file, with target names expressed as `os-arch` strings such as `linux-amd64` or `windows-amd64` and driven from config rather than shell scripts. The target list is explicit per project rather than globally mandatory. Build output should also embed version metadata so the compiled CLI can report `version`, `commitId`, `date`, and related runtime details via `--version`, and the same metadata should be available as JSON with `--version --json`. When the command fails it should report the target, failing step, and next useful action. On success it should summarize which targets were built and where artifacts and checksums were written.

### 09 Release

How release publication is driven from config.

#### Release Command

Run `gh flarebyte build` first, then publish a GitHub release from the resulting build outputs. Use the top-level `release` block to choose the tag, artifacts, and release note behavior, requiring `releaseNotesFilePath` when `notesMode` is `notes-file`, and implement the command in Go rather than the current Bun helper. On failure, the CLI should distinguish between build failure, tag/version resolution failure, and release upload failure. On success it should confirm the published tag and the artifact source used.

### 10 Init

What repo bootstrap does.

#### Init Command

Bootstrap a repository by seeding `.gh-flarebyte.cue` with repository, build, and release defaults and then applying the initial syncable repo settings. The command should explain what file it created or updated and point users to `gh flarebyte repo update --help` for the next step. Success output should make the next action obvious.

### 11 Update

What reconciliation from cue config means.

#### Update Command

Reconcile the live GitHub repository from `.gh-flarebyte.cue`, including repo settings, topics, and label definitions. Topics and labels are exact-set sync targets, so remote items missing from config should be treated as deletions. The command must fail unless the user explicitly confirms deletions with `--confirm-deletions`. Visibility changes should also require explicit CLI confirmation with `--accept-visibility-change-consequences` rather than a committed config flag. Failure output should explain what would change, why it was blocked, and the exact next command or flag to use. Success output should summarize what changed rather than only saying the command succeeded.

### 12 Audit

What read-only drift checking means.

#### Audit Command

Compare the checked-in Cue config with GitHub and report drift without changing remote state. Output should summarize the number of differences and point users to `gh flarebyte repo update` when remediation is appropriate. A clean run should say clearly that no drift was found.

### 13 Repos Mine

What repository discovery returns.

#### Repos Mine Command

List repositories the current user contributes to within an organization so the extension can discover target repos before sync. Success output should include the organization queried and how many repositories were found.

### 14 GitHub Flags

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

