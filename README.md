# gh-flarebyte
Use `gh flarebyte` to keep a GitHub repository aligned with the config checked into the repo.

![gh-flarebyte illustration](doc/gh-flarebyte-hero.png)

## What it does
`gh-flarebyte` is a GitHub CLI extension for managing a repository from a local `.gh-flarebyte.cue` file. It helps you:

- bootstrap a repository config
- update repo settings from config
- audit drift between config and GitHub
- sync topics and labels from config
- build either binary artifacts or library compile checks from the configured language
- publish a GitHub release with or without binary artifacts
- discover repositories you contribute to in an organization

## Commands

### Repo sync
- `gh flarebyte repo init --repo <owner/name> [--overwrite]` creates or seeds `.gh-flarebyte.cue` for a repository.
- `gh flarebyte repo update [--repo <owner/name>] [--confirm-deletions] [--accept-visibility-change-consequences]` applies the repo config to GitHub.
- `gh flarebyte repo audit [--repo <owner/name>] [--json]` compares the local config with the live GitHub repository.
- `gh flarebyte repos mine --org <org>` lists repositories you contribute to in an organization.

### Build and release
- `gh flarebyte build [--target <os-arch>] [--output-dir <path>]` builds the project using the language and mode defined in `.gh-flarebyte.cue`.
- `gh flarebyte release [--draft] [--notes-file <path>]` builds first, then publishes a GitHub release from the configured release policy.

### Runtime info
- `gh flarebyte --version` prints the CLI version metadata, including version, commit id, build date, OS/arch, and Go runtime version.
- `gh flarebyte --version --json` prints the same version metadata in a machine-readable JSON shape.

Use `gh flarebyte --help` for command usage. Error output is designed to include the failing config field or target and a practical next step.

## Config
The repo config lives in `.gh-flarebyte.cue` and is the source of truth for:

- repository metadata such as description, homepage, visibility, and template status
- topics
- labels
- repository feature flags currently enforced by sync/audit: `repository.features.mergeCommit`, `repository.features.rebaseMerge`, `repository.features.squashMerge`, `repository.features.deleteBranchOnMerge`
- build language, mode, and artifact policy
- release settings
- additional repository feature fields may exist in config but are not yet enforced by `repo update` / `repo audit`

Example:

```cue
project: {
  org:  "flarebyte"
  repo: "gh-flarebyte"
}

repository: {
  description: "CLI for landing your git commands right"
  defaultBranch: "main"
  topics: ["gh-extension", "github-cli", "git", "flarebyte"]
  labels: [
    {
      name: "bug"
      color: "B60205"
      description: "Something is broken"
    },
  ]
}

build: {
  language: "go"
  mode: "binary"
  outputDir: "build"
  checksumFile: "build/checksums.txt"
  artifactTargetSuffix: true
  targets: ["linux-amd64", "darwin-arm64"]
}

release: {
  versionSource: "main.project.yaml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  includeArtifacts: true
  artifactDir: "build"
  includeChecksums: true
}
```

Go library example:

```cue
build: {
  language: "go"
  mode: "library"
  packages: ["./..."]
  runTests: true
}

release: {
  versionSource: "main.project.yaml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  includeArtifacts: false
}
```

## Typical workflow
1. Run `gh flarebyte repo init` in a repo that should be managed.
2. Edit `.gh-flarebyte.cue` to match the desired repository state.
3. Run `gh flarebyte repo update` to sync the repo.
4. Run `gh flarebyte repo audit` to check for drift.
5. Run `gh flarebyte build` and `gh flarebyte release` when you are ready to ship.

## Makefile shortcuts
- `make build-go` builds the local CLI binary at `.e2e-bin/gh-flarebyte`.
- `make release` runs `.e2e-bin/gh-flarebyte release` (it depends on `build-go`).
- `GH_FLAREBYTE_FAKE_RELEASE=1 make release` runs the release flow in fake mode (no GitHub mutation).

## Notes
- Topics are managed as a flat list of strings.
- Labels are managed as structured objects with `name`, `color`, and `description`.
- Build is Go-first today, with Dart reserved in the config for later.
- `build.mode` defaults to `binary`. Use `library` for multi-package libraries (compile verification with `go build`, and optional `go test` when `runTests: true`).
- `build.artifactTargetSuffix` controls whether artifact names include `-os-arch` suffixes.
  - when `false` with multiple targets, artifacts are written under per-target subdirectories to avoid filename collisions.
- In `library` mode, `--target` applies `GOOS/GOARCH` cross-compile checks and does not force artifact generation.
- `release.includeArtifacts` defaults to `true`. Set it to `false` to publish tag and notes without uploading binaries or checksums.
