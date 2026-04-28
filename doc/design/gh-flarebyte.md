# gh-flarebyte Design Notes

Repository sync model for the flarebyte GitHub CLI extension.

## 01 Overview

What the extension manages and why the repo-local CUE file exists.

### 01 Intent

Why the checked-in config file exists.

#### Project Summary

Flarebyte's `gh` extension manages GitHub repository state from a checked-in `.gh-flarebyte.cue` file so repo metadata, topics, labels, and repo settings can be synchronized deterministically. `gh flarebyte repo update` applies the config-driven `gh repo edit` changes.

### 02 Scope

The operational boundary for the extension.

#### Project Scope

The extension is centered on repo bootstrap, reconciliation, audit, and repository discovery. Topics are flat strings, labels are structured objects, and repo-edit mutations are applied by `gh flarebyte repo update` rather than by manually repeating `gh repo edit` flags.

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
	mode: "bidirectional"
	visibilityChangeConsequenceAccepted: true
	managedFields: [
		"description",
		"defaultBranch",
		"homepage",
		"visibility",
		"template",
		"build.language",
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
	description: "CLI for landing your git commands right"
	defaultBranch: "main"
	homepage: "https://github.com/flarebyte/gh-flarebyte"
	visibility: "public"
	template: false
	build: {
		language: "go"
	}
	buildPlan: {
		outputDir: "build"
		checksumFile: "build/checksums.txt"
		targets: [
			"linux/amd64",
			"darwin/arm64",
		]
	}
	topics: [
		"gh-extension",
		"github-cli",
		"git",
		"flarebyte",
	]
	labels: [
		{
			name: "bug"
			color: "B60205"
			description: "Something is broken"
		},
		{
			name: "enhancement"
			color: "0E8A16"
			description: "New feature"
		},
	]
	features: {
		issues:               true
		wiki:                 false
		projects:             false
		discussions:          false
		autoMerge:            true
		mergeCommit:          false
		rebaseMerge:          false
		squashMerge:          true
		squashMergeCommitMessage: "pr-title"
		deleteBranchOnMerge:  true
		allowForking:         false
		allowUpdateBranch:    false
		advancedSecurity:     true
		secretScanning:       true
		secretScanningPushProtection: true
	}
}
```

### 02 Field Map

How config fields map onto GitHub repository settings.

#### Config Sync Field Map

| field | kind | managed_by | notes | sync_direction |
| --- | --- | --- | --- | --- |
| project.org | string | local config | The owner org anchors discovery and bootstrap. | read-only |
| project.repo | string | local config | The repository name binds the config to one GitHub repo. | read-only |
| sync.mode | enum | extension | Default policy for reconcile and audit commands. | bidirectional |
| repository.description | string | .gh-flarebyte.cue | Primary repo metadata field. | local-to-remote |
| repository.defaultBranch | string | .gh-flarebyte.cue | Usually `main` for flarebyte repositories. | local-to-remote |
| repository.homepage | string | .gh-flarebyte.cue | Repository homepage URL. | local-to-remote |
| repository.visibility | enum | .gh-flarebyte.cue | Repository visibility: public, private, or internal. | local-to-remote |
| repository.template | boolean | .gh-flarebyte.cue | Make the repository available as a template. | local-to-remote |
| repository.build.language | enum | .gh-flarebyte.cue | Build language used by gh flarebyte build: go initially, dart later. | local-to-remote |
| repository.buildPlan.outputDir | string | .gh-flarebyte.cue | Directory for built artifacts. | local-to-remote |
| repository.buildPlan.checksumFile | string | .gh-flarebyte.cue | Checksum manifest for built artifacts. | local-to-remote |
| repository.buildPlan.targets | list | .gh-flarebyte.cue | Target matrix for the build command. | local-to-remote |
| repository.topics | list | .gh-flarebyte.cue | Topics should stay stable and sorted. | local-to-remote |
| repository.labels | list | .gh-flarebyte.cue | Structured label definitions managed separately from topics. | local-to-remote |
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
| sync.visibilityChangeConsequenceAccepted | boolean | extension | Guardrail for changing repository visibility. | read-only |

### 03 Topics

Repository topics are a flat sync target in the cue config.

#### Topic Sync

Topics are kept as a flat string list in the cue config and synchronized directly to the repository topics list.

### 04 Labels

Repository labels have their own structured sync shape in the cue config.

#### Label Sync

Labels use a structured list of objects in the cue config so name, color, and description can be reconciled separately from repository topics.

### 05 Build

Build language selection for gh flarebyte build.

#### Build Config

Build orchestration is driven by the Cue config. `gh flarebyte build` reads the configured language and uses a Go implementation initially, with Dart allowed later as a supported language. The command should write language-specific binaries under `build/`, emit a `build/checksums.txt` manifest, and keep the output layout stable across languages. The separate `buildPlan` block describes artifact paths and target matrix.

### 06 Sync Types

TypeScript shapes for the sync contract.

#### Sync Types

```ts
export type SyncMode = "push" | "pull" | "bidirectional";

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
};

export type BuildTarget = {
  os: "linux" | "darwin";
  arch: "amd64" | "arm64";
  label: string;
};

export type BuildPlan = {
  outputDir: string;
  checksumFile: string;
  targets: BuildTarget[];
};

export type RepositoryConfig = {
  org: string;
  repo: string;
  description: string;
  defaultBranch: string;
  homepage: string;
  visibility: "public" | "private" | "internal";
  template: boolean;
  build: BuildConfig;
  buildPlan: BuildPlan;
  topics: string[];
  labels: LabelConfig[];
  features: RepositoryFeatures;
};

export type SyncPlan = {
  mode: SyncMode;
  managedFields: string[];
  dryRun: boolean;
  visibilityChangeConsequenceAccepted: boolean;
};

export type DriftItem = {
  field: string;
  local: string | boolean | string[];
  remote: string | boolean | string[];
};
```

### 07 Config Coverage

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
| gh flarebyte build | repository.build.language -> build artifacts | language-specific build output | build the project according to the configured language | write |
| gh flarebyte repo init | .gh-flarebyte.cue created or seeded | initialized repo state | bootstrap a repo-local config and initial GitHub defaults | write |
| gh flarebyte repo update | .gh-flarebyte.cue -> GitHub repo state | updated remote repo state | reconcile GitHub repo metadata from local config | write |
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
    name: "init",
    command: "gh flarebyte repo init --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Bootstrap repo-local config and seed the GitHub repo settings.",
    syncEffect: "Create or update `.gh-flarebyte.cue` and apply initial defaults.",
  },
  {
    name: "update",
    command: "gh flarebyte repo update --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Reconcile GitHub repository state from the local config.",
    syncEffect: "Push local desired state to GitHub.",
  },
  {
    name: "audit",
    command: "gh flarebyte repo audit --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Report drift between the checked-in config and GitHub.",
    syncEffect: "Read-only comparison.",
  },
];
```

### 03 Build

How build orchestration is driven from config.

#### Build Command

Build the project from the configured language. Start with Go only, but keep the config shape open for Dart so the command can grow without changing its contract. The first implementation should produce `build/<name>-<os>-<arch>` artifacts and a `build/checksums.txt` file, with the target matrix and output paths driven from config rather than shell scripts.

### 04 Init

What repo bootstrap does.

#### Init Command

Bootstrap a repository by seeding `.gh-flarebyte.cue` and applying the initial syncable repo settings.

### 05 Update

What reconciliation from cue config means.

#### Update Command

Reconcile the live GitHub repository from `.gh-flarebyte.cue`, including repo settings, topics, and label definitions.

### 06 Audit

What read-only drift checking means.

#### Audit Command

Compare the checked-in Cue config with GitHub and report drift without changing remote state.

### 07 Repos Mine

What repository discovery returns.

#### Repos Mine Command

List repositories the current user contributes to within an organization so the extension can discover target repos before sync.

### 08 GitHub Flags

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

Clarify which repo fields are fully managed by the extension, whether sync should be one-way or bidirectional by default, and how conflicts should be surfaced when remote state diverges from `.gh-flarebyte.cue`.

