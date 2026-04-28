# gh-flarebyte Design Notes

Repository sync model for the flarebyte GitHub CLI extension.

## 01 Overview

What the extension manages and why the repo-local CUE file exists.

### 01 Intent

Why the checked-in config file exists.

#### Project Summary

Flarebyte's `gh` extension manages GitHub repository state from a checked-in `.gh-flarebyte.cue` file so repo metadata can be synchronized deterministically.

### 02 Scope

The operational boundary for the extension.

#### Project Scope

The extension is centered on repo bootstrap, reconciliation, audit, and repository discovery. It should keep local config and GitHub state aligned without requiring manual repetition of the same `gh repo edit` flags.

## 02 Configuration

Canonical repo configuration and the field map used for sync.

### 01 Config Example

The repo-local `.gh-flarebyte.cue` file.

#### Repo Config Example

```cue
package ghflarebyte

project: {
	org:  "flarebyte"
	repo: "gh-flarebyte"
}

sync: {
	mode: "bidirectional"
	managedFields: [
		"description",
		"defaultBranch",
		"topics",
		"features.issues",
		"features.wiki",
		"features.projects",
		"features.discussions",
		"features.autoMerge",
		"features.mergeCommit",
		"features.rebaseMerge",
		"features.squashMerge",
		"features.deleteBranchOnMerge",
	]
}

repository: {
	description: "CLI for landing your git commands right"
	defaultBranch: "main"
	topics: [
		"gh-extension",
		"github-cli",
		"git",
		"flarebyte",
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
		deleteBranchOnMerge:  true
		allowForking:         false
	}
}
```

### 02 Field Map

How config fields map onto GitHub repository settings.

#### Repo Config Field Map

| field | kind | managed_by | notes | sync_direction |
| --- | --- | --- | --- | --- |
| project.org | string | local config | The owner org anchors discovery and bootstrap. | read-only |
| project.repo | string | local config | The repository name binds the config to one GitHub repo. | read-only |
| sync.mode | enum | extension | Default policy for reconcile and audit commands. | bidirectional |
| repository.description | string | .gh-flarebyte.cue | Primary repo metadata field. | local-to-remote |
| repository.defaultBranch | string | .gh-flarebyte.cue | Usually `main` for flarebyte repositories. | local-to-remote |
| repository.topics | list | .gh-flarebyte.cue | Topics should stay stable and sorted. | local-to-remote |
| repository.features.issues | boolean | .gh-flarebyte.cue | GitHub issues enabled or disabled. | local-to-remote |
| repository.features.wiki | boolean | .gh-flarebyte.cue | GitHub wiki enabled or disabled. | local-to-remote |
| repository.features.projects | boolean | .gh-flarebyte.cue | GitHub projects enabled or disabled. | local-to-remote |
| repository.features.discussions | boolean | .gh-flarebyte.cue | GitHub discussions enabled or disabled. | local-to-remote |
| repository.features.autoMerge | boolean | .gh-flarebyte.cue | Auto-merge policy. | local-to-remote |
| repository.features.mergeCommit | boolean | .gh-flarebyte.cue | Merge commit policy. | local-to-remote |
| repository.features.rebaseMerge | boolean | .gh-flarebyte.cue | Rebase merge policy. | local-to-remote |
| repository.features.squashMerge | boolean | .gh-flarebyte.cue | Squash merge policy. | local-to-remote |
| repository.features.deleteBranchOnMerge | boolean | .gh-flarebyte.cue | Head branch cleanup policy. | local-to-remote |
| repository.features.allowForking | boolean | .gh-flarebyte.cue | Forking policy for organization repos. | local-to-remote |

### 03 Sync Types

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
  deleteBranchOnMerge: boolean;
  allowForking: boolean;
};

export type RepositoryConfig = {
  org: string;
  repo: string;
  description: string;
  defaultBranch: string;
  topics: string[];
  features: RepositoryFeatures;
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

## 03 Commands

User-facing extension commands and the underlying GitHub operations.

### 01 Command Matrix

User-facing extension actions and their purpose.

#### Command Matrix

| command | config_touchpoints | output | purpose | read_write |
| --- | --- | --- | --- | --- |
| gh flarebyte repo init | .gh-flarebyte.cue created or seeded | initialized repo state | bootstrap a repo-local config and initial GitHub defaults | write |
| gh flarebyte repo update | .gh-flarebyte.cue -> GitHub repo state | updated remote repo state | reconcile GitHub repo metadata from local config | write |
| gh flarebyte repo audit | .gh-flarebyte.cue and GitHub state | drift report | compare local config with remote GitHub state | read |
| gh flarebyte repos mine | none | relevant repo list | list repositories the user contributes to within an org | read |
| gh repo edit | remote repo flags only | updated repository settings | apply low-level GitHub repo mutations | write |

### 02 Command Flows

TypeScript examples that show the intended command sequences.

#### Command Flows

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

### 03 GitHub Flags

The lower-level `gh repo edit` knobs the extension maps onto.

#### GitHub Repo Edit Flags

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

## 04 Discovery

Repo discovery for the 'repos mine' workflow.

### 01 Repos Mine

How the extension discovers repositories the user contributes to.

#### Repo Discovery

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

## 05 Open Questions

Pending decisions that should stay visible in the spec.

### 01 Decisions Pending

What still needs agreement before the sync contract hardens.

#### Open Questions

Clarify which repo fields are fully managed by the extension, whether sync should be one-way or bidirectional by default, and how conflicts should be surfaced when remote state diverges from `.gh-flarebyte.cue`.

