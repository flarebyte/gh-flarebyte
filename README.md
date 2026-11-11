# gh-flarebyte

Use `gh flarebyte` to keep a GitHub repository aligned with the config checked into the repo.

![gh-flarebyte illustration](doc/gh-flarebyte-hero.png)

## What it does

`gh-flarebyte` is a GitHub CLI extension for managing a repository from a local `.gh-flarebyte.cue` file. It helps you:

- bootstrap a repository config
- update repo settings from config
- audit drift between config and GitHub
- sync topics and labels from config
- build binary artifacts, library compile checks, Python distributions, Rust crate packages, or native Swift executables
- publish a GitHub release with or without build artifacts
- discover repositories you contribute to in an organization

## Commands

### Repo sync

- `gh flarebyte repo init --repo <owner/name> [--overwrite]` creates or seeds `.gh-flarebyte.cue` for a repository.
- `gh flarebyte repo update [--repo <owner/name>] [--confirm-deletions] [--accept-visibility-change-consequences]` applies the repo config to GitHub.
- `gh flarebyte repo audit [--repo <owner/name>] [--json]` compares the local config with the live GitHub repository.
- `gh flarebyte repos mine --org <org> [--json]` lists repositories you contribute to in an organization.

### Build and release

- `gh flarebyte build [--target <os-arch>] [--output-dir <path>]` builds the project using the language and mode defined in `.gh-flarebyte.cue`.
- `gh flarebyte release [--draft] [--notes-file <path>]` builds first, then publishes a GitHub release from the configured release policy.
- `build`: Go supports `binary` and `library`; Dart supports `library`; Python 3 with uv supports `package`; Rust supports all three modes; Swift supports `binary` and `library`; TypeScript supports `library` verification and `binary` CLI builds; Kotlin/JVM supports `library` and `binary` CLI distributions.
- `release`: Dart currently supports release with `release.includeArtifacts: false`.

### Dev commands

- `gh flarebyte test [--style summary|per_test] [--color auto|true|false] [--failed-only]` runs tests for the configured build language, with optional per-invocation output overrides.
- `gh flarebyte format` formats source files for the configured build language.
- `gh flarebyte lint [--color auto|true|false] [--failed-only]` runs lint/static checks for the configured build language.
- `gh flarebyte cov [--min <percent>] [--color auto|true|false] [--failed-only]` computes coverage and can fail below a threshold.

### Runtime info

- `gh flarebyte --version` prints the CLI version metadata, including version, commit id, build date, OS/arch, and Go runtime version.
- `gh flarebyte --version --json` prints the same version metadata in a machine-readable JSON shape.

### Configuration checks

- `gh flarebyte config validate [--config <path>]` validates the local configuration without updating GitHub.

Use `gh flarebyte --help` for command usage. Error output is designed to include the failing config field or target and a practical next step.

## Config

The repo config lives in `.gh-flarebyte.cue` and is the source of truth for:

- repository metadata such as description, homepage, visibility, and template status
- topics
- labels
- repository feature flags currently enforced by sync/audit: `repository.features.mergeCommit`, `repository.features.rebaseMerge`, `repository.features.squashMerge`, `repository.features.deleteBranchOnMerge`
- build language, mode, and artifact policy
- go command execution env for dev flows (`go.cacheDir`, `go.modCacheDir`, `go.toolchain`)
- go build CGO contract (`go.cgo.enabled`, `go.cgo.cc`, `go.cgo.cxx`)
- dev command output controls (`devOutput.color`, `devOutput.style`, `devOutput.showPassed`)
- coverage threshold policy (`coverage.min`, `coverage.enforceMin`)
- release settings
- additional repository feature fields may exist in config but are not yet enforced by `repo update` / `repo audit`

Complete configuration example:

```cue
package ghflarebyte

project: {
  org:  "flarebyte"
  repo: "gh-flarebyte"
}

sync: { mode: "push" }

repository: {
  description: "CLI for landing your git commands right"
  defaultBranch: "main"
  homepage: "https://github.com/flarebyte/gh-flarebyte"
  visibility: "public"
  template: false
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

go: {
  cacheDir: "./.gocache"
  modCacheDir: "./.gomodcache"
  toolchain: "local"
  cgo: {
    enabled: false
  }
}

devOutput: {
  color: "auto"
  style: "summary"
  showPassed: true
}

coverage: {
  min: 80
  enforceMin: true
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

This example expects `main.project.yaml` to contain a top-level `version`, such as
`version: 1.2.3`. Run `gh flarebyte config validate` after editing the configuration.
Configuration validation checks fields and combinations; builds and developer commands
also check the installed toolchain and project files.

## Language workflows

The following examples replace the corresponding top-level blocks in the complete
configuration above. Keep the shared `project`, `sync`, and `repository` blocks.
The `go` block configures Go tooling and can be omitted for other languages.

### Go

Library verification:

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

### Dart

Library validation:

```cue
build: {
  language: "dart"
  mode:     "library"
  packages: ["./..."]
  runTests: true
}

coverage: {
  min: 80
  enforceMin: true
}

release: {
  versionSource: "main.project.yaml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  includeArtifacts: false
}
```

### Python with uv

Package distributions:

```cue
build: {
  language: "python"
  mode: "package"
  runTests: true
  outputDir: "build"
  checksumFile: "build/checksums.txt"
}

devOutput: {
  color: "auto"
  style: "summary"
  showPassed: true
}

coverage: {
  min: 80
  enforceMin: true
}

release: {
  versionSource: "pyproject.toml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  artifactDir: "build"
  includeArtifacts: true
  includeChecksums: true
}
```

Install uv and declare Python support with `[project].requires-python` in `pyproject.toml`.
Use `.python-version` if you want to pin the interpreter. Add the developer tools with
`uv add --dev pytest ruff coverage`, configure their rules in `pyproject.toml`, and commit `uv.lock`.
Commands run through `uv run --locked`: uv may install the locked dependencies into `.venv`,
but a missing or stale lockfile fails rather than being updated. Missing tools are reported as failures.

`test` uses pytest JUnit XML for summaries and per-test output. `format` uses Ruff formatting;
`lint` uses Ruff checks without automatic fixes. `cov` runs pytest under coverage.py and applies
the same `coverage.min`, `coverage.enforceMin`, and `--min` policy as Go and Dart. Configure
`[tool.coverage.run].source` to include your package and measure untested modules.

`build` produces a wheel and source distribution using `uv build --no-sources`, plus SHA-256
checksums. Use an explicit `[build-system]` and pin its build dependencies for repeatable
isolated builds; those dependencies are resolved separately from `uv.lock`.
`--output-dir` relocates both distributions and the checksum manifest.
`release` attaches only files from its fresh build, respecting `release.artifactDir` and
`release.includeChecksums`; `release.includeArtifacts: false` publishes without assets.

Initial Python support covers one package per repository, with no binary mode, target matrix,
or PyPI upload. Keep `build.targets` unset. Package filenames follow Python distribution naming,
so `build.artifactTargetSuffix` does not apply. TOML version sources require a static
`[project].version` following the existing semantic-version rule (for example `1.2.3`).
Dynamic versions and Python-specific versions such as `1.2.3rc1` are not supported yet.

### Rust with Cargo

Library verification:

```cue
build: {
  language: "rust"
  mode: "library"
  packages: ["./..."]
  runTests: true
}

coverage: { min: 80, enforceMin: true }
devOutput: { color: "auto", style: "summary", showPassed: true }

release: {
  versionSource: "Cargo.toml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  includeArtifacts: false
}
```

Rust executable build block:

```cue
build: {
  language: "rust"
  mode: "binary"
  mainPackage: "."
  runTests: true
  outputDir: "build"
  checksumFile: "build/checksums.txt"
  artifactTargetSuffix: true
  targets: ["linux-amd64", "darwin-arm64"]
}
```

For `.crate` distributions, use `mode: "package"`, `packages: ["./..."]`, `outputDir`, and
`checksumFile`, omitting `targets`. For executable or package releases, set
`release.includeArtifacts: true`, `release.artifactDir: "build"`, and optionally
`release.includeChecksums: true`. All artifact-producing Rust builds honor `--output-dir`.

Install a stable Rust toolchain with rustfmt and Clippy. Keep toolchain selection in
`rust-toolchain.toml`, Rust version/edition/lints in `Cargo.toml`, and Cargo configuration in
`.cargo/config.toml`. Commit `Cargo.lock` at the workspace root. Dependency-resolving commands
use `--locked`; a missing or stale lock fails without being rewritten. Formatting also requires
a project manifest and an existing lockfile. Cargo may download locked dependencies and consult the registry during packaging.

`test` runs the whole workspace, including unit, integration, and documentation tests. Standard
harness output supports summaries, `per_test`, ignored tests, and `--failed-only`.
Custom `harness = false` targets support summary style only. `format` uses `cargo fmt --all`;
`lint` uses Clippy across the workspace and all targets, treats warnings as errors, and does not fix code.

`cov` requires `cargo-llvm-cov` (`cargo install cargo-llvm-cov --locked`) and the active toolchain's
`llvm-tools-preview` component (`rustup component add llvm-tools-preview`). It runs workspace
tests and computes total line coverage from LCOV line counts, excluding sources outside the workspace
and generated/dependency directories (`target`, `.cargo`, `.cache`, `vendor`). It uses isolated
temporary coverage artifacts and the shared threshold/output settings. Stable coverage excludes
doctests; ordinary `test` still runs them. Missing coverage components fail with installation guidance.

`build.packages` defaults to `["./..."]`, selecting all workspace members for library/package builds;
explicit values are exact Cargo crate names. Developer commands operate on the whole workspace.
Binary builds use `build.mainPackage`, the directory containing the package's `Cargo.toml`, defaulting
to `"."`. The selected package must have one binary or declare `package.default-run`.
Cargo compiler JSON identifies the executable, avoiding assumptions about Cargo's output filenames.

Library builds compile selected crates without release assets. Binary builds use Cargo's release
profile and the same archive, binary naming, checksums, and target-suffix policy as Go.
Package builds run verified `cargo package --locked` and produce one `.crate` per selected crate;
Cargo's clean-source and registry packaging rules apply. `build.runTests` enables host tests of the
selected crates before any build mode, including cross-compilation.

Rust preserves the shared target names with these mappings:

| Target | Rust triple |
|---|---|
| `linux-amd64` | `x86_64-unknown-linux-gnu` |
| `linux-arm64` | `aarch64-unknown-linux-gnu` |
| `darwin-amd64` | `x86_64-apple-darwin` |
| `darwin-arm64` | `aarch64-apple-darwin` |
| `windows-amd64` | `x86_64-pc-windows-msvc` |
| `windows-arm64` | `aarch64-pc-windows-msvc` |

Install the target components and suitable linkers before cross-building. `--target` filters binary
targets or selects a library compile check; package mode rejects it. Custom triples and musl builds
are outside the initial target mapping.

`release.versionSource` can point to a package's `Cargo.toml`, including workspace-inherited
`package.version`. A virtual workspace manifest has no package release version: point to a member
manifest instead. Releases use fresh build assets only, preserve subdirectories when staging into
`release.artifactDir`, and support notes, drafts, and releases without assets. crates.io publishing
is outside the initial scope.

### Swift with Swift Package Manager

Swift support requires a Swift 6+ toolchain and the matching platform SDK. On macOS,
use Xcode or its command-line tools; Linux installations need the Swift toolchain and
its documented system dependencies. Older `swift-tools-version` manifests can still
be used with Swift 6. Keep language versions, compiler settings, dependencies, and
platform requirements in `Package.swift`; keep formatter rules in `.swift-format`.

Reuse the existing repository, output, coverage, and release configuration:

```cue
build: {
  language: "swift"
  mode: "library"
  packages: ["./..."]
  runTests: true
}
devOutput: { color: "auto", style: "summary", showPassed: true }
coverage: { min: 80, enforceMin: true }
release: {
  versionSource: "main.project.yaml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  includeArtifacts: false
}
```

`build.packages: ["./..."]` verifies all root-package build targets. Exact target names
such as `["MyLibrary"]` select individual build targets; test and plugin targets are
not valid explicit build selections. Developer commands operate on the whole root
package, independently of build selection. `build.runTests` runs the complete package
test suite before library or binary builds.

Executable projects use the same artifact fields as Go and Rust:

```cue
build: {
  language: "swift"
  mode: "binary"
  mainPackage: "my-cli"
  targets: ["darwin-arm64"]
  runTests: true
  outputDir: "build"
  checksumFile: "build/checksums.txt"
  artifactTargetSuffix: true
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

`build.mainPackage` identifies a SwiftPM executable **product**, rather than a source
folder. Omit it when the package has exactly one executable product. Initial builds
support the active compiler's native host on macOS or GNU/Linux with amd64 or arm64:
`darwin-amd64`, `darwin-arm64`, `linux-amd64`, or `linux-arm64`. `build.targets` and
`--target` must match that host. Cross-compilation and Windows builds are not included.
Binary builds use release configuration, discover the output path through SwiftPM,
and produce the existing executable/archive/checksum layout. `--output-dir` relocates
artifacts and checksums. Packaging resource bundles or package-built shared libraries is not supported and fails explicitly.
Executables depend on compatible Swift runtime libraries on their destination systems;
archives do not bundle the Swift runtime or external system libraries.

Swift requires a root `Package.swift`. Packages with remote dependencies also require
`Package.resolved`; generate it with `swift package resolve` and commit it. Dependency-free
and local-only packages do not need an empty lockfile. Resolving commands use
`--force-resolved-versions` so missing or stale resolution fails instead of updating
versions. A preflight check also inspects local dependency manifests so transitive
remote dependencies require the root lockfile before SwiftPM runs resolution.

Developer commands:

- `test` runs XCTest and Swift Testing. XCTest uses serial results checked against its
  summary; Swift Testing uses JSON event schema version 0. Missing or incomplete reports
  fail explicitly. Parameterized Swift Testing functions appear as one aggregated test.
- `format` runs `swift format` on manifest and target-owned Swift files, excluding
  dependencies and generated build files.
- `lint` runs strict formatter lint, then compiles sources and test targets with Swift
  compiler warnings treated as errors. It does not run tests or require SwiftLint.
- `cov` runs instrumented tests in a fresh build directory and reads SwiftPM's LLVM JSON
  export. Coverage counts production-source lines only, excluding tests, dependencies,
  and generated sources. Totals are weighted by line counts and honor `coverage.min`,
  `coverage.enforceMin`, `devOutput.style`, `--min`, and `--failed-only`.

Release versions come from the existing YAML/JSON top-level `version` field, because
`Package.swift` does not carry the root package's release version. Releases stage only
fresh executable assets and optional checksums, or publish library tags and notes without
assets. Package archives, registry publication, XCFrameworks, Xcode app projects,
signing, and notarization are outside this initial support.

Swift compatibility checks run through `.github/workflows/swift.yml`. They prioritize
macOS arm64 development with Xcode 16.2 (Swift 6.0.3) and Xcode 26.2, plus one GNU/Linux
amd64 pipeline check using the official Swift 6.2.4 container. Each job reports its
actual toolchain and host and runs `make test-swift-e2e`. Hosted matrix results are
available after the workflow is pushed and run; a local pass validates only the local host.
The fixture checks both test frameworks, aggregated parameterized test successes and
failures, coverage thresholds, direct and transitive dependency lock enforcement,
executable packaging, and simulated releases. Releases in this suite never publish
to GitHub. Real Swift integration remains opt-in for local `make test` runs.

### Independent e2e tests and scripts

Optional `e2e` and `script` sections select Bun TypeScript or Python 3 independently
of `build.language`. A Go or Rust project can use either runtime. Unit tests, build,
and release do not automatically run e2e suites.

```cue
e2e: {
  runtime: "bun"
  root: "e2e"
  dependencies: {
    manager: "npm"
    root: "."
  }
}
script: {
  runtime: "python3"
  root: "script"
}
```

Run `gh flarebyte e2e` or `gh flarebyte script release_preflight.py -- --help`.
`script` takes a file relative to its configured folder and forwards arguments after
`--`. Bun accepts `.ts`, `.tsx`, `.mts`, and `.cts`; Python accepts `.py`.
Both commands execute with the repository root as their working directory and preserve
child failures and output. Files and configured folders must remain inside the repository,
including after resolving symlinks. Scripts must remain inside their script folder.

All config paths are relative to the repository root containing `.gh-flarebyte.cue`;
run commands from that root. Default source folders are `e2e` and `script`.
`dependencies.root: "."` uses shared root dependency files. Set it to the configured
source folder to isolate dependencies there. Omit `dependencies` when no installation
is required; Bun automatic dependency installation is disabled.

Within `dependencies`, `root` defaults to `"."`; `manager` defaults to `"bun"` for
Bun. Select `"npm"` explicitly to share a TypeScript project's npm dependencies.
Python dependencies require an explicit `manager: "uv"`. Supported preparations are:

| Manager | Required files in dependency root | Preparation |
|---|---|---|
| Bun | `package.json`, `bun.lock` | `bun install --frozen-lockfile --ignore-scripts` |
| npm | `package.json`, `package-lock.json` | `npm ci --ignore-scripts --no-audit --no-fund` |
| uv | `pyproject.toml`, `uv.lock` | `uv sync --locked`, then `uv run --project … --no-sync python3` |

Use one JavaScript dependency manager for a shared folder. Separate folders may use
different managers. Bun e2e uses `bun test` with a fresh validated JUnit report;
Python e2e uses standard-library `unittest` discovery of `test_*.py`, with the usual
package discovery rules for subdirectories. Empty suites fail. No additional test
framework is required. Building the application under test belongs in the suite or
an explicitly invoked preparation script.

### Kotlin/JVM with Gradle

Kotlin support uses a committed, executable `gradlew`, its wrapper JAR and properties,
a root `build.gradle.kts` or `build.gradle`, and a root `gradle.lockfile`. Install the
JDK selected by the project's Gradle toolchain. The integration fixture pins Gradle
8.14.6, Kotlin 2.2.21, and JDK 21. Command execution supports macOS and Linux.

Apply `org.jetbrains.kotlin.jvm`, `org.jlleitschuh.gradle.ktlint`, and `jacoco` for
all developer commands. CLI projects also apply `application` and set
`application.mainClass`. Keep plugin versions, dependencies, compiler options, and
JVM toolchain selection in Gradle. Generate the wrapper and resolve dependency
locks before using these commands; the [fixture](e2e/fixtures/kotlin/build.gradle.kts)
includes a `resolveLocks` task. Commit locks for quality-tool configurations too.
Commands enforce strict dependency locking without updating locks.

Library configuration:

```cue
build: {
  language: "kotlin"
  mode: "library"
  packages: ["./..."]
  runTests: true
}
release: {
  versionSource: "main.project.yaml"
  includeArtifacts: false
}
```

For a CLI, replace the build block with:

```cue
build: {
  language: "kotlin"
  mode: "binary"
  runTests: true
  outputDir: "dist"
  checksumFile: "dist/checksums.txt"
}
```

Set `release.includeArtifacts: true`, `release.artifactDir: "dist"`, and
`release.includeChecksums: true` to attach the CLI distribution to GitHub releases.
The snippets extend the shared project/repository/release configuration above.
Use existing YAML/JSON version sources; `build.gradle.kts` is not a version source.

- `test` runs the root Gradle `test` task and validates fresh JUnit XML totals.
  It supports the same summary, per-test, color, skipped-test, and failure-only policies.
  Missing reports and empty test suites fail explicitly.
- `format` runs `ktlintFormat`; `lint` runs `ktlintCheck`, `classes`, and
  `testClasses`, with Kotlin and Java compiler warnings treated as errors.
- `cov` runs `test` and `jacocoTestReport`. Production line coverage comes from
  JaCoCo XML, weighted by line counts, with the shared `--min` and enforcement policy.
  Coverage scope follows Gradle's JaCoCo source/class configuration; this is JVM
  bytecode line coverage and may differ from source-level Kotlin tools.
- `build` assembles libraries without release assets, or uses `distZip` for CLIs.
  Both modes honor `runTests`. Builds and reports use an isolated temporary directory.
- CLI archives contain launch scripts and runtime dependency JARs and require a
  compatible Java runtime. Their name is `<repo>-<version>.zip`, with SHA-256 checksums.
  `--output-dir` relocates archives and checksums. Releases attach only fresh assets.

Initial support covers one root Gradle JVM project, with `packages: ["./..."]`.
`mainPackage`, `targets`, `--target`, and `artifactTargetSuffix` are rejected:
JVM distributions are platform-independent. Multi-project/composite builds, Android,
Multiplatform, native executables, bundled JVMs, and Maven Central publishing are
outside this scope. `e2e` and `script` remain independent of the build language.

### TypeScript with npm and Node

TypeScript unit tests use npm and Node 24+ with `tsx`. Keep a committed
`package-lock.json`, root `package.json`, and `tsconfig.json`. Commands prepare
locked dependencies with `npm ci --ignore-scripts --no-audit --no-fund` and run
project-local tools through Node. Missing or stale lockfiles fail without being
rewritten. Lifecycle scripts are disabled.

Add `typescript` and `tsx` to dev dependencies. Add `c8` for coverage, and
`@biomejs/biome` with `biome.json` or `biome.jsonc` for formatting/linting.
Runtime type definitions belong to the project. For example:

```json
{
  "scripts": {
    "test": "node --import tsx --test test/**/*.test.ts"
  }
}
```

`test` must invoke Node's built-in test runner and inherit `NODE_OPTIONS`:
gh-flarebyte selects a fresh JUnit report through that environment variable.
`tsx` handles execution; `tsc --noEmit` handles type checking. Bun is required
for standalone CLI compilation and Bun-based scripts/e2e suites.
The CUE snippets below replace the build and release sections in a complete config.

**Libraries** use verification mode. This checks types without emitting JavaScript,
declarations, binaries, or release assets. `build.runTests` optionally runs the test suite.

```cue
build: {
  language: "typescript"
  mode: "library"
  packages: ["./..."]
  runTests: true
}
release: {
  versionSource: "package.json"
  tagPrefix: "v"
  notesMode: "generate-notes"
  includeArtifacts: false
}
```

**CLIs** use binary mode with an explicit source entry point included in `tsconfig.json`.
The build first checks types, optionally runs tests, then uses `bun build --compile` to
produce a standalone executable with Bun's runtime embedded. It reuses the existing
archive, executable naming, checksum, `--output-dir`, and fresh release asset policies.

```cue
build: {
  language: "typescript"
  mode: "binary"
  mainPackage: "src/cli.ts"
  targets: ["darwin-arm64"]
  runTests: true
  outputDir: "build"
  checksumFile: "build/checksums.txt"
}
release: {
  versionSource: "package.json"
  tagPrefix: "v"
  notesMode: "generate-notes"
  includeArtifacts: true
  artifactDir: "build"
  includeChecksums: true
}
```

CLI targets must match the active Bun host: native macOS or GNU/Linux on amd64/arm64.
Library builds reject `mainPackage`, target matrices, and `--target`; they do not silently
become CLI builds. Initial support covers one root package with `packages: ["./..."]`.
Workspaces, TypeScript project references, Windows, cross-compilation, npm package
archives/publication, and automatic copying of external runtime resources are outside
this initial scope. Compiled CLIs disable automatic runtime loading of `.env` and Bun config.

Developer commands operate on the root project:

| Command | Behavior |
|---|---|
| `format` | Project-local Biome `format --write .` |
| `lint` | Biome `check .` without fixes, followed by `tsc --project tsconfig.json --noEmit` |
| `test` | `npm test` with Node JUnit XML and shared summary/per-test output |
| `cov` | Project-local c8 wrapping `npm test`, with source-mapped LCOV line coverage |

Keep compiler settings in `tsconfig.json`, lint/format rules in Biome, and test
discovery/preloads in the npm test script. Reports use fresh temporary directories.
`coverage.min`, `coverage.enforceMin`, and `--min` control the shared threshold policy.
`--color`, `--failed-only`, `showPassed`, and `devOutput.style` follow the same policies
as the other languages. Existing `bunfig.toml` files do not configure Node unit tests.

Coverage selects production files from `tsconfig.json`, excluding test files/directories,
script/e2e folders, declaration files, dependencies, generated sources, and build outputs.
c8's `--all` includes unimported production sources at zero coverage. Totals are weighted
by executable line counts. Coverage fails if selected sources are absent from its report.

Both library and CLI releases read the configured `release.versionSource`; the examples
use `package.json`'s existing top-level `version`. YAML version sources also work.
Library releases contain tags and notes without assets; CLI releases stage only fresh
executables, archives, and optional checksums. No npm publishing occurs.

Run `gh flarebyte format`, `gh flarebyte lint`, `gh flarebyte test`, and
`gh flarebyte cov --min 80` from the project root, then `gh flarebyte build`.
For this repository's real npm/tsx and Bun CLI integration checks, run `make test-typescript-e2e`.
That fixture exercises library verification and native CLI execution, coverage policies,
failure handling, checksums, and simulated releases. It runs on macOS in CI; local passes
validate the local host, while hosted results are available after the workflow runs.

## CI coverage budget

Both compatibility workflows run on pull requests, pushes to `main`, and manual dispatch.
There are **8 jobs per PR/push: 6 macOS and 2 Linux**, within the budget of at most 8.
Go tests run once in the core job, including the shared contracts for all seven languages.
The core job also runs the Python release-preflight unit tests.

| Checks | macOS arm64 | Linux amd64 | Jobs |
|---|---|---|---|
| Swift integration | Xcode 16.2 and 26.2 | Swift 6.2.4 | 3 |
| Rust and Kotlin integration | Stable Rust; JDK 21 / Gradle JVM | Stable Rust; JDK 21 / Gradle JVM | 2 |
| Python integration | Python 3.12 with uv | — | 1 |
| TypeScript integration | npm/tsx library and native Bun CLI | — | 1 |
| Core Go and default E2E | Go from `go.mod`; includes Dart stub tests | — | 1 |
| **Total** | **6** | **2** | **8** |

`.github/workflows/languages.yml` owns the Go/default E2E, Python, Rust, TypeScript, and Kotlin jobs;
`.github/workflows/swift.yml` owns the three Swift jobs. Each integration job runs the
existing Makefile fixture with simulated releases. Dart currently has stub coverage;
this matrix does not claim real Dart SDK integration. Swift Intel macOS, Linux arm64,
and a full OS/toolchain cross-product are outside this routine CI selection.
The count measures platform/toolchain jobs; each job contains multiple assertions.
Test CLI builds omit optional Go VCS stamping, checkout credentials are not persisted,
and repository permissions remain read-only.
All workflow actions are pinned to full commit SHAs, with their original version or
branch noted in comments. Updating an action requires verifying and updating its SHA.

## Manual GitHub releases

`.github/workflows/release.yml` runs only through **Actions → Manual release → Run workflow**.
Select the default branch; there are no version or draft inputs. Bump and commit
`tags.version` in `main.project.yaml` before starting a new release. The preflight reads
that version through `.gh-flarebyte.cue`'s `release.versionSource`, just like the CLI,
and adds the configured tag prefix automatically.
The workflow must first be present on the default branch to appear in the Actions UI.

The workflow runs these stages in order:

1. Run the release-preflight unit tests, resolve the version from the configured file,
   and check that the configured tag and visible GitHub releases do not already exist.
2. Run `make lint test`. Any failure blocks the release.
3. Build the configured CLI targets and checksums through `gh-flarebyte build`.
4. Transfer those artifacts to a separate publishing job, recheck the resolved version,
   tested commit, and availability, then verify checksums and unique asset filenames.
   Reserve a new tag at the tested SHA and publish the release using that tag.
   An existing tag cannot be overwritten.

The workflow publishes immediately after all checks pass; it does not create a draft.
An existing tag, published release, or draft visible to the publishing token blocks reuse of that version.
The final availability check uses the publishing job's credentials so it can also see
draft releases. API authentication, rate-limit, and network failures stop the workflow;
they are never interpreted as an unused version. If publication leaves a tag or partial
draft, inspect it manually before retrying; the workflow does not overwrite existing releases.

This repository publishes GitHub Release binaries and checksums. It does not publish
to PyPI, crates.io, or another package registry. Builds use the existing release policy;
the manual workflow currently requires generated release notes, artifacts, and checksums.
Only the publishing job has `contents: write`, checkout credentials are not retained,
and concurrent release attempts are serialized. These two manual jobs are separate from
the eight automatic compatibility jobs and add no jobs to pull-request or push runs.

The standard-library Python preflight can also be run read-only locally with Python 3.11+:

```sh
python3 script/release_preflight.py --repository flarebyte/gh-flarebyte
python3 -m unittest discover -s script -p 'test_*.py'
```

Set `GH_TOKEN` or `GITHUB_TOKEN` to inspect private or draft releases permitted by that
token. Without a token, only public API data is checked. The script never creates a tag,
release, or upload; publication is a separate, final workflow step.
The preflight connects only to `https://api.github.com` and rejects redirects and other
API hosts; this repository's manual release workflow targets GitHub.com.

## Typical workflow

1. Run `gh flarebyte repo init --repo <owner/name>` in a repo that should be managed.
2. Edit `.gh-flarebyte.cue` to match the desired repository state.
3. Run `gh flarebyte config validate` to check the configuration.
4. Run `gh flarebyte repo update` to sync the repo, then `gh flarebyte repo audit` to check for drift.
5. Run `gh flarebyte test`, `gh flarebyte format`, `gh flarebyte lint`, and `gh flarebyte cov` for local quality checks.
6. Run `gh flarebyte build` and `gh flarebyte release` when you are ready to ship.

## Makefile shortcuts

- `make build-go` builds the local CLI binary at `.e2e-bin/gh-flarebyte`.
- `make release` runs `.e2e-bin/gh-flarebyte release` (it depends on `build-go`).
- `GH_FLAREBYTE_FAKE_RELEASE=1 make release` runs the release flow in fake mode (no GitHub mutation).
- `make review` finishes formatting before running tests and lint, including with `make -j`.
- `make e2e` aliases the default smoke and stub-tool suite; `make test-dart-e2e` runs the Dart stub fixture alone.
- `make test-integrations` runs all real Python, Rust, Swift, TypeScript, and Kotlin fixtures; these remain opt-in.
- `make e2e-deps` installs dependencies from `bun.lock`; formatting and lint use the locked local Biome.
- The legacy `*-dart` shortcuts and `review-dart` follow the root `.gh-flarebyte.cue`; they do not select Dart. This repository configures Go.
- `make check-tools` reports executable availability; `biome` checks the local installation and `npx` is the launcher used for jscpd.
- `make doc-decision` is a legacy target whose generation script is currently absent.
- `make test-python-e2e` runs the real uv fixture (requires uv and an installed Python 3; may download dependencies).
- `make test-rust-e2e` runs the real Rust workspace fixture (requires Cargo, rustfmt, Clippy, cargo-llvm-cov, and llvm-tools-preview; may access crates.io).
- `make test-swift-e2e` runs the real SwiftPM fixture (requires Swift 6+ and its SDK).
- `make test-kotlin-e2e` runs the real Kotlin/JVM Gradle library and CLI fixture (requires JDK 21; downloads pinned Gradle and locked dependencies).
- `make test-typescript-e2e` runs the real TypeScript library and CLI fixture (requires Node 24+, npm, and Bun 1.3.6+ and may download locked dependencies).

## Language support snapshot

| Language | Build modes | Release artifacts | Developer commands |
|---|---|---|---|
| Go | `binary`, `library` | Executables, archives, checksums; library releases have no assets | `test`, `format`, `lint`, `cov` |
| Dart | `library` | No assets; requires `release.includeArtifacts: false` | `test`, `format`, `lint`, `cov` |
| Python 3 with uv | `package` | Wheels, source distributions, checksums | `test`, `format`, `lint`, `cov` |
| Rust with Cargo | `binary`, `library`, `package` | Executables, archives, `.crate` packages, checksums; library releases have no assets | `test`, `format`, `lint`, `cov` |
| Swift with SwiftPM | `binary`, `library` | Native macOS/Linux executables, archives, checksums; library releases have no assets | `test`, `format`, `lint`, `cov` |
| Kotlin/JVM with Gradle | `binary` CLI, `library` verification | JVM distribution ZIP and checksums; library releases have no assets | `test`, `format`, `lint`, `cov` |
| TypeScript with npm/Node | `binary` CLI, `library` verification | Native macOS/Linux standalone CLIs, archives, checksums; library releases have no assets | `test`, `format`, `lint`, `cov` |

All languages support per-test output and coverage thresholds. Artifact-producing builds
can release without assets by setting `release.includeArtifacts: false`. Language-specific
toolchains, target restrictions, and test-report limitations are described above.

## Notes

- Topics are managed as a flat list of strings.
- Labels are managed as structured objects with `name`, `color`, and `description`.
- Build: Go supports `binary` and `library`; Dart supports `library`; Python supports `package`; Rust supports all three; Swift supports `binary` and `library`; TypeScript supports `library` verification and `binary` CLI builds; Kotlin/JVM supports `library` and `binary` CLI distributions.
- Release: Dart currently supports `release.includeArtifacts: false`.
- `build.mode` defaults to `binary`. Use `library` for Go/Dart/Rust/Swift/TypeScript/Kotlin verification or `package` for Python/Rust distributions. `build.runTests` applies to Go/Dart library builds, Python package builds, and every Rust/Swift/TypeScript/Kotlin build mode.
- `build.mainPackage` controls the Go main package (default `./cmd/<project.repo>`) or Rust manifest directory (default `.`) used for binary builds. Swift uses an executable product name, inferred when unique. TypeScript CLI builds require an explicit source entry point; library builds reject it.
- For Go/Rust/Swift/TypeScript binary builds with one target, `build.artifactTargetSuffix: false` removes the target suffix from the archive name. Multiple-target archive names always include the target; setting this field to `false` also places each target's assets in its own subdirectory. Standalone executable asset names always include `-os-arch`.
- In `library` mode, `--target` selects Go cross-compile checks through `GOOS/GOARCH`, a Rust target triple, or the native Swift host. Dart, TypeScript, and Kotlin reject it. Library checks do not produce release artifacts.
- `go.cgo` uses camelCase keys only: `enabled`, `cc`, `cxx`.
- `go.cacheDir` and `go.modCacheDir` should be project-relative (for example `./.gocache`); `gh flarebyte` resolves them to absolute paths at runtime before invoking Go.
- Absolute `go.cacheDir` and `go.modCacheDir` values still work, but emit portability warnings.
- When `go.cgo.enabled: true`, `gh flarebyte build` sets `CGO_ENABLED=1` (and applies `CC`/`CXX` when configured).
- When `go.cgo.enabled: false`, the build keeps `CGO_ENABLED=0` and fails with a policy error if CGO-backed dependencies are detected.
- `release.includeArtifacts` defaults to `true`. Set it to `false` to publish tag and notes without uploading release assets.
- `gh flarebyte cov --min 90` overrides config coverage threshold for that invocation.
- For Dart projects, `cov` computes total line coverage from `.dart_tool/coverage/lcov.info` and enforces `coverage.min` / `--min` when `coverage.enforceMin` is enabled.
- If `.dart_tool/coverage/lcov.info` is missing after `dart test --coverage`, `cov` attempts to generate it via `coverage:format_coverage`.
- `devOutput.style` supports `summary` and `per_test`.
- `--color` overrides `devOutput.color` for `test`, `lint`, and `cov`.
- `--failed-only` suppresses PASS summary output for `test`, `lint`, and `cov`.
- In `per_test` style, `test` prints `✓/↷/✗` per test for all seven languages and `cov` prints `✓/✗` per coverage entry; with `--failed-only`, only failing entries are shown.
