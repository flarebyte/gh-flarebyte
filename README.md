# gh-flarebyte
Use `gh flarebyte` to keep a GitHub repository aligned with the config checked into the repo.

![gh-flarebyte illustration](doc/gh-flarebyte-hero.png)

## What it does
`gh-flarebyte` is a GitHub CLI extension for managing a repository from a local `.gh-flarebyte.cue` file. It helps you:

- bootstrap a repository config
- update repo settings from config
- audit drift between config and GitHub
- sync topics and labels from config
- build release artifacts from the configured language
- publish a GitHub release from the build outputs
- discover repositories you contribute to in an organization

## Commands

### Repo sync
- `gh flarebyte repo init --repo <owner/name> [--overwrite]` creates or seeds `.gh-flarebyte.cue` for a repository.
- `gh flarebyte repo update [--repo <owner/name>] [--confirm-deletions] [--accept-visibility-change-consequences]` applies the repo config to GitHub.
- `gh flarebyte repo audit [--repo <owner/name>] [--json]` compares the local config with the live GitHub repository.
- `gh flarebyte repos mine --org <org>` lists repositories you contribute to in an organization.

### Build and release
- `gh flarebyte build [--target <os-arch>] [--output-dir <path>]` builds the project using the language defined in `.gh-flarebyte.cue`.
- `gh flarebyte release [--draft] [--notes-file <path>]` builds first, then publishes a GitHub release from the build outputs.

### Runtime info
- `gh flarebyte --version` prints the CLI version metadata, including version, commit id, and build date.
- `gh flarebyte --version --json` prints the same version metadata in a machine-readable JSON shape.

Use `gh flarebyte --help` for command usage. Error output is designed to include the failing config field or target and a practical next step.

## Config
The repo config lives in `.gh-flarebyte.cue` and is the source of truth for:

- repository metadata such as description, homepage, visibility, and template status
- topics
- labels
- build language and artifact layout
- release settings
- repository feature flags

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
  outputDir: "build"
  checksumFile: "build/checksums.txt"
  artifactTargetSuffix: true
  targets: ["linux-amd64", "darwin-arm64"]
}

release: {
  versionSource: "main.project.yaml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  artifactDir: "build"
  includeChecksums: true
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
- `build.artifactTargetSuffix` controls whether artifact names include `-os-arch` suffixes.
  - when `false` with multiple targets, artifacts are written under per-target subdirectories to avoid filename collisions.
- Release publication uses the build outputs already produced by `gh flarebyte build`.
