# gh-flarebyte

Use `gh flarebyte` to keep a GitHub repository aligned with the config checked into the repo.

![gh-flarebyte illustration](doc/gh-flarebyte-hero.png)

## What it does

`gh-flarebyte` is a GitHub CLI extension for managing a repository from a local `.gh-flarebyte.cue` file. It helps you:

- bootstrap a repository config
- update repo settings from config
- audit drift between config and GitHub
- sync topics and labels from config
- verify libraries and build CLI artifacts, Python distributions, Rust crate packages, and Kotlin JVM distributions
- publish a GitHub release with or without build artifacts
- discover repositories you contribute to in an organization

## Install and quick start

Install [GitHub CLI](https://cli.github.com/) and authenticate with `gh auth login`
for commands that read or update GitHub. Build and developer commands also need the
toolchain for the configured language; see [Language workflows](#language-workflows).

Install the extension using [GitHub CLI's extension installer](https://cli.github.com/manual/gh_extension_install):

```sh
gh extension install flarebyte/gh-flarebyte
gh flarebyte --version
```

The repository's release configuration builds macOS arm64 and Linux amd64 binaries.
To run from a source checkout, see [local development](CONTRIBUTING.md#local-development).

From the root of a repository you want to manage, replace `OWNER/REPO` with its
GitHub coordinates and initialize the local configuration:

```sh
gh flarebyte repo init --repo OWNER/REPO
```

This writes `.gh-flarebyte.cue` using existing GitHub metadata when available, with
Go binary build defaults. Review the file and choose the appropriate language and
mode before validating it:

```sh
gh flarebyte config validate
gh flarebyte repo audit
```

Successful validation prints `config is valid:` followed by the absolute config
path. Audit reports differences without updating GitHub and exits with code `3`
when drift exists. Use `gh flarebyte repo update` when ready to apply the config.

Run commands from the directory containing `.gh-flarebyte.cue`; the CLI does not
search parent directories. `config validate --config <path>` validates another file
without changing the configuration used by subsequent commands.

Documentation: [language support](#language-support-snapshot),
[contributing](CONTRIBUTING.md), [design notes](doc/design/gh-flarebyte.md),
and [license](LICENSE).

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
- `gh flarebyte cc` finds duplicate source and test code using an installed jscpd, showing at most 10 pairs.
- `gh flarebyte meta` runs root-level Thoth/Maat pipelines and summarizes metadata changes.
- `gh flarebyte diff [--against main|head] [--format markdown|json|yaml] [--context <lines>]` reports code changes with existing Thoth metadata.
- `gh flarebyte cov [--min <percent>] [--color auto|true|false] [--failed-only]` computes coverage and can fail below a threshold.
- `gh flarebyte e2e` runs an independently configured Bun or Python suite.
- `gh flarebyte script <file> -- [args...]` runs a file from the configured script folder.

See [independent e2e tests and scripts](#independent-e2e-tests-and-scripts) for their configuration.

### Code review reports

```sh
gh flarebyte diff
gh flarebyte diff --against head
gh flarebyte diff --format json --context 20
```

`diff` defaults to the merge base of HEAD and local `main`, including branch commits,
staged and unstaged edits, and nonignored untracked files. Use `--against head` for
uncommitted changes only. Run it from anywhere inside a Git working tree; no project
config, GitHub authentication, or network access is required.

Markdown output lists created, updated, and deleted paths alphabetically, then shows
file history, matching `thoth-meta/**/*.thoth.yaml` documents, and contextual diffs.
New text files are shown in full. File age measures days since the last committed
update; local changes are indicated separately. The command reads metadata without
regenerating it. JSON and YAML provide the same versioned report for automation.
A clean comparison prints `No changes.` in Markdown. Both changed and clean reports
exit successfully; comparison failures exit `1` and invalid flags exit `2`.

### Runtime info

- `gh flarebyte --version` prints the CLI version metadata, including version, commit id, build date, OS/arch, and Go runtime version.
- `gh flarebyte --version --json` prints the same version metadata in a machine-readable JSON shape.

### Configuration checks

- `gh flarebyte config validate [--config <path>]` validates the local configuration without updating GitHub.

Use `gh flarebyte --help` or a command's `--help` for usage. Normal results go to
stdout; diagnostics and developer failure summaries go to stderr. Per-test failure
entries can appear on stdout. `script` and `e2e` preserve child output and exit status.

| Exit code | Meaning |
|---|---|
| `0` | Success |
| `1` | Developer check or command failure |
| `2` | Invalid invocation or configuration |
| `3` | Repository drift found by audit |
| `4` | Update blocked pending deletion confirmation |
| `5` | Update blocked pending visibility-change acceptance |
| `6` | Build failure |
| `7` | Release failure |

These are the extension's own status codes; scripts and e2e tools can return other
child exit codes. Error details identify the failing configuration field, tool, or target.

## Config

The repo config lives in `.gh-flarebyte.cue`. The CLI reads literal fields in a
constrained CUE-like format; it does not evaluate CUE imports or expressions. The
file is the source of truth for:

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
- additional repository feature fields may exist in config but are not enforced by `repo update` / `repo audit`

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

### Dependency assessment

Run dependency checks from a local Git repository:

```sh
gh flarebyte dep check npm lodash@4.17.21 --json
gh flarebyte dep check cargo serde --explain
```

The only command flags are `--json` and `--explain`. Checks query deps.dev, OSV
and OpenSSF Scorecard with bounded requests. Registry descriptions are included
when available. Configure the release waiting period in `.gh-flarebyte.cue`:

```cue
dependencies: {
  minAgeDays: 14
}
```

The default is 14 days; explicit zero removes only that waiting period. Invalid
policy values fail validation. The configuration is discovered at the local Git
repository root, including when invoked from a subdirectory.

Current coverage is partial: npm, Cargo, PyPI, Go and Maven metadata can be checked.
Swift checks accept public GitHub repository URLs and use a bounded GitHub release
catalog, repository stars/description, OSV SwiftURL and optional Scorecard. Dates
mean GitHub release publication dates; bare tags, non-GitHub hosts and catalogs
beyond 500 releases remain unverified. Swift maintainer count is unknown; advisory
matching uses the canonical repository URL, with alias coverage explicitly limited.
For example, `gh flarebyte dep check swift https://github.com/apple/swift-nio --json`.
Swift install prints an exact `Package.swift` declaration and changes no files.
Exact Dart/pub versions can receive OSV advisory findings. Registry
lookups corroborate npm, Cargo and PyPI release availability, deprecation or yanking.
Verified provenance can corroborate a GitHub source repository; self-declared links
cannot borrow its reputation. Advisory aliases are grouped, and withdrawal conflicts
remain blocking evidence gaps. Candidate ordering handles numeric stable releases
and npm/Cargo build metadata. PyPI ordering also handles public epochs, longer
numeric release tuples and stable `.postN` releases; canonical prerelease and
development releases are excluded. Local versions and unsupported spellings remain
unknown. Unsupported ordering prevents a newest-version claim.
For PyPI catalog entries that cannot be ordered, checks make at most four distinct
registry availability probes. An entry confirmed unavailable by HTTP 404 or fully
yanked can be excluded from recommendation ordering; `--explain` records it under
`catalog_exclusions`. Present or unresolved entries remain blocking. Exact-version
checks preserve the original string and still assess the requested release.

Npm now uses package-level policy `package-advice-v1`. It checks direct advisories,
release age/lifecycle, repository identity/reputation and the current registry owner
roster without reading a manifest/lockfile, resolving dependencies or querying
privileged GitHub maintenance feeds. Missing maintainer activity or Scorecard data
is a warning; a known single registry owner or fewer than 100 verified-repository
stars denies adoption. Uncorroborated repository identity stays unknown and may
include a mature advisory-clean `candidate_version`. Reports label coverage
`direct-package-only`; recommendations use compatibility `package-only`.

```sh
gh flarebyte dep check npm lodash --json
gh flarebyte dep install npm lodash
```

Npm installation delegates the recommended exact version to
`npm install --save-exact -- NAME@VERSION` in the invocation's working directory.
An explicit version must itself pass the check; it is never silently replaced.
Native stdout and stderr stream unchanged to stdout; stdin, normal npm configuration
and environment are inherited, and the native exit status is preserved. `--json`
and `--explain` are check-only for npm. A denied/unknown assessment starts no tool.
Npm owns hooks, resolution and file changes; there is no staging, rollback or
transitive-installation safety claim. Go and Python delegation are described below.

Go module checks now use the same package-advice policy and direct-package-only
coverage. GitHub module paths corroborate their repository association with
`evidence.identity_basis: "github-module-path"`; this is path-based evidence, not
artifact provenance. Conflicting source associations deny; vanity paths remain
unknown unless existing provenance corroborates them. Checks use deps.dev and OSV,
without reading go.mod/go.sum, fetching vanity metadata or resolving a module graph.
Maintainer counts remain unknown warnings. `GO_RETRACTIONS_UNCHECKED` makes clear
that indexed availability/deprecation evidence does not establish retraction status.
Pseudo-version ordering is unsupported: explicit pseudo-versions stay review and
cannot authorize installation. Stable releases retain their exact `v` prefix and
`+incompatible` suffix where supported.

```sh
gh flarebyte dep check go github.com/spf13/cobra --json
gh flarebyte dep install go github.com/spf13/cobra
```

Go installation uses `go get MODULE@VERSION` through the same native runner as npm,
with the caller's working directory, stdin, normal Go configuration/environment,
raw output to stdout and native exit status. Go owns go.mod/go.sum updates,
retraction warnings, toolchain selection and compatibility failures. Like npm,
`--json` and `--explain` are check-only for Go. Other ecosystem paths are unchanged.

Kotlin/JVM dependencies now use package advice through the `maven` selector and
`group:artifact` coordinates. Numeric releases with one to four components are
ordered numerically; clearly marked alpha/beta/milestone/RC/SNAPSHOT releases are
excluded from stable recommendations. Unrecognized qualifiers (for example `.Final`
or `-jre`) leave ordering unresolved rather than claiming full Maven ordering.
Checks use indexed deps.dev release/deprecation evidence and Maven OSV findings,
with explicit `MAVEN_LIFECYCLE_COVERAGE_LIMITED` warnings. Repository corroboration
and reputation follow the same advice policy; no group-to-repository guess is made.

```sh
gh flarebyte dep check maven org.slf4j:slf4j-api --json
gh flarebyte dep install maven org.slf4j:slf4j-api
```

Maven installation is **print-only**: after a recommendation, a single regular
`build.gradle.kts`, `build.gradle` or `pom.xml` in the current directory selects a
Kotlin DSL, Groovy DSL or XML declaration. Missing/ambiguous formats and symlinks
produce labelled manual guidance. No files are edited and no build tool is run.
The printed version is an explicit request; native dependency mediation remains
Maven/Gradle's responsibility. `--json` and `--explain` are check-only here too.

Python/PyPI now uses the same package-advice policy and native runner. Package
names are normalized; supported public epochs/post releases retain their exact
version spelling. Checks use direct PyPI release/yanking evidence and available
registry roles without inspecting pyproject.toml, pylock.toml, wheels, dependency
markers or the local Python environment. Missing/null yanking evidence stays
unknown. An organization with one visible account is not classified as a confirmed
single-maintainer package; unknown counts remain warnings.

```sh
gh flarebyte dep check pypi requests --json
gh flarebyte dep install pypi requests
```

Installation delegates `uv add NAME==VERSION` from the invocation directory with
normal uv configuration/environment, stdin, raw stdout/stderr to stdout and native
exit status. Names are canonicalized before constructing the literal argument;
there is no shell. `--json` and `--explain` are check-only. Missing uv fails clearly
without pip fallback or tool installation. uv owns Python selection, compatibility,
resolution, lockfile changes and backend execution. Retained deep Python inspection
components below are not prerequisites for this default workflow.

Rust/Cargo now uses the shared package-advice policy and native runner. Checks
reuse exact crates.io release/yanking evidence, direct advisories and repository
reputation without inspecting Cargo.toml/Cargo.lock, computing features/MSRV,
reading archives or staging an SDK. Current owner context is fetched once; no
publisher-history or privileged activity investigation is required. A confirmed
single user owner denies adoption. Teams, incomplete/ambiguous rosters and unknown
team membership remain unknown-count warnings rather than single-developer claims.

```sh
gh flarebyte dep check cargo serde --json
gh flarebyte dep install cargo serde
```

Installation delegates `cargo add NAME@=VERSION` in the caller's directory using
normal Cargo/Rustup configuration, stdin and raw native output/status. Explicit
versions must pass their own check; `--json`/`--explain` are check-only. Missing
Cargo fails without tool bootstrap. Cargo owns feature/workspace handling,
resolution, lockfile updates and any toolchain/compatibility failures. The strict
Cargo archive/staging/install components below are retained separately and do not
gate this default path.

The retained legacy npm assessor inspects local lockfiles v2/v3 without executing tooling. They support an
already locked runtime version or a dependency-free addition that preserves existing
versions and peer constraints. Dependency-free runtime packages can also be upgraded
when both versions have no outgoing dependency or peer edges, no nested subtree
exists, and every retained incoming constraint still holds. Every other locked
artifact remains pinned and assessed. Checks reject stale manifests, unsafe artifact origins,
weak hashes, symlinks, workspaces and unsupported resolution forms. Transitive findings
include lockfile paths, and registry metadata is compared with the proposed graph.
Supported constraints include exact versions, full caret/tilde versions, comparator
conjunctions, bare partial versions (`1`, `1.2`), trailing wildcards and `||`
alternatives. Every alternative must use supported syntax; prereleases remain gated.
General upgrades and runtime/platform constraints may still require further evidence.

The retained legacy PyPI assessor can audit a restricted `pyproject.toml`/`pylock.toml` inventory:
exact root pins, lock format 1.0, and one public PyPI wheel with SHA-256 per
package. Each locked version receives advisory checks; wheel URLs, filenames,
hashes, optional sizes and yanking are corroborated with PyPI. A locked problem
denies with `LOCKED_INVENTORY_VIOLATION`, even if the requested package's catalog
cannot be read. Checks neither download wheels nor execute Python.
Explained output uses `resolution: "locked-inventory"`,
`inventory_complete: true` and `complete: false`. This inventory does not prove
dependency resolution or Python/platform compatibility: a clean inventory still
returns `unknown`, with no recommended version or installation approval. Markers,
extras, groups, alternate sources, source distributions and multiple wheel choices
remain unsupported. The lock's informational dependency tables do not establish
an installation graph. These components do not implement a strict PyPI installer.

For `project.requires-python = ">=3.9"` (or another numeric minor/patch lower
bound), the retained PyPI assessor compares that declared baseline with the selected wheel's
`requires_python` and the lock's optional `requires-python`. A definite mismatch
denies with `PYTHON_VERSION_INCOMPATIBLE` or
`PYTHON_LOCK_BASELINE_INCOMPATIBLE`. Explained per-package `runtime` evidence uses
the basis `manifest-python-lower-bound`. Wheel/lock requirements support numeric
comparisons joined by commas; unsupported syntax or absent wheel evidence stays
unknown. A compatible baseline does not verify the installed interpreter, the
full supported Python range, wheel platform tags or dependency resolution.

The retained PyPI components also read hashed per-wheel core metadata advertised by the public
Simple API. They verify the selected wheel's URL and SHA-256, then download only
its bounded `.metadata` sidecar and verify that separate SHA-256 and package
identity. Unconditional dependency declarations are compared with the inventory;
missing dependencies or incompatible numeric constraints deny. Unresolved markers, extras,
direct URLs and unsupported version syntax keep metadata evidence incomplete.
Explained per-package `distribution` evidence reports its basis, hash, dependency
count and status. This never downloads a wheel or resolves a new graph; a clean
inventory still returns `unknown`; this retained path is separate from default uv delegation.

Corroborated wheel filenames are also checked for package identity and generic
Python tags against the declared lower bound. Pure-Python alternatives such as
`py3-none-any` and `py39-none-any` can establish tag compatibility; a supported
generic mismatch denies with `PYPI_WHEEL_TAG_INCOMPATIBLE`. Explained `wheel`
evidence includes the parsed Python/ABI/platform tags and comparison status.
Native ABI/platform tags, unsupported alternatives and ambiguous declarations
stay unknown. The CLI host is never assumed to be the Python target, and matching
tags do not clear environment or project-resolution gaps.

Wheel dependency markers support numeric `python_version` and
`python_full_version` comparisons, `and`/`or`, parentheses and reversed operands.
They are evaluated conservatively across the manifest's entire inclusive Python
lower-bound range. Only definitely inactive dependencies are skipped; partially
applicable conditions remain unresolved. Platform and extras values are never
inferred from the CLI host. Explained `distribution` evidence includes active,
inactive and unresolved dependency counts with marker basis
`manifest-python-range`. These counts describe applicability; all locked packages
retain their security assessments within this retained inspection path.

Explained `project.python_graph` now reports structural closure separately. A
`closed` graph requires registry-bound hashed metadata and compatible generic
wheel tags for every assessed node, matching active dependencies, valid manifest
roots and reachability of every locked package. Missing evidence, unresolved
conditions, native targets and unused lock entries keep it incomplete. Closure
does not change `project.complete`, resolve an environment, recommend a version
or approve installation; vulnerabilities and other security failures still deny.

The retained legacy Cargo assessor inspects `Cargo.toml` and lockfiles v3/v4 for single-package crates.io
projects and already locked runtime dependencies. Every locked crate is assessed,
including build/development dependencies, as a conservative superset rather than
an activated target/feature graph. Checks compare registry SHA-256 checksums and
dependency declarations with the lock. Explained output identifies entries as
`Cargo.lock#name@version` and labels the resolution `locked-superset`.
Workspaces, target tables, patches, path/Git dependencies, alternate registries and
local Cargo configuration remain unsupported. Feature activation, missing optional
resolution and Rust compiler requirements can leave the verdict unknown. Cargo
bare versions use caret semantics; unsupported ranges remain blocking. Checks
never execute Cargo or package code.

In that retained path, each locked crate's declared minimum Rust version is compared
with the project's `[package].rust-version` baseline. A newer requirement denies
with `RUST_VERSION_INCOMPATIBLE`; a missing baseline or unrecognised requirement
stays unknown. Invalid edition/baseline combinations are blocked. `--explain`
includes `runtime_baseline` and per-crate `runtime` evidence with the basis
`manifest-rust-version`. This compares declared requirements; it does not verify
the installed compiler, compilation or feature activation.

The retained Cargo components resolve a bounded default-feature union, including explicit
features, disabled defaults, optional dependencies, renamed crates and forwarding.
`--explain` includes `project.features` with enabled names, resolution status and
any minimum Cargo syntax requirement. Cycles, ambiguous mappings, missing metadata
and inactive weak-feature contexts stay unknown. Missing optional locked packages
remain blocking, and inactive crates retain all security findings. This union does
not establish a general target-specific install plan. Installation uses the
narrower supported context described below.

Cargo archive preflight is now implemented as a read-only backend component. It
binds SHA-256 checksums and normalized manifests to an approved, unchanged graph,
rejects links, escapes, control files and path collisions, and bounds archive
expansion and metadata. It records build-script/proc-macro declarations without
executing code. The initial verifier accepts a restricted portable filename and
manifest subset.

The backend can now stage an approved graph outside the repository, extract only
verified crates with private permissions, and generate Cargo directory-source
checksums. An isolated standalone Cargo 1.85+ SDK can validate the staged default
feature graph with `metadata --offline --locked`. The adapter clears inherited
credentials/options/wrappers, rejects ancestor Cargo configuration and Rustup
proxies, and checks the actual Rust version against declared requirements. It
does not compile packages.

The retained legacy Cargo installer supports an already locked runtime
dependency in a single-package project with default `src/lib.rs` and/or
`src/main.rs` targets. It validates the original pinned manifest and target shape
offline, then changes only that dependency's version value to `=recommended-version`.
Comments, aliases, feature options and unrelated formatting are preserved;
`Cargo.lock` stays byte-for-byte unchanged. Applying a library dependency updates
the manifest; it does not compile code or populate a shared Cargo cache.

This initial install scope excludes custom targets, root dev/build dependencies,
build scripts, proc macros, inherited Cargo configuration/workspaces, and graph
changes. Both the project and every crate need compatible declared Rust minimums.
Unsupported contexts fail closed. Freshness, policy, input and target checks run
again before commit; concurrent edits trigger rollback or retained recovery data.

The retained strict policy `conservative-v2` requires 100 stars, two active accounts with corroborated
maintenance authority and a Scorecard scan no older than 30 days. Individual Scorecard minimums
are Branch-Protection 6, Code-Review 7, Dangerous-Workflow 10, Maintained 5 and
Token-Permissions 8. Scores are evaluated from one applicable snapshot. Public
Scorecard date-only scan dates are interpreted as midnight UTC; RFC3339 timestamps
are also accepted, and the same 30-day freshness limit applies. Limited
lookalike screening uses a versioned reference set; it is not exhaustive.

Authenticated GitHub lookups can corroborate two distinct non-bot user accounts with
current write authority and recent PR or independent review activity. Existing
credentials are read lazily; permissions are never expanded. Protected or incomplete
role data stays unknown, and contributor counts do not substitute for authority.
[GitHub restricts access to collaborator data](https://docs.github.com/en/rest/collaborators/collaborators#list-repository-collaborators).
That endpoint requires repository write/maintain/admin privileges and, for an
organization repository, organization membership. Authentication alone cannot
close this evidence gap for arbitrary third-party packages.
The retained strict Cargo path additionally supports anonymous registry evidence: current crates.io user-owner
IDs matched to release `published_by.id` records within the last 180 days. It samples
at most 100 recent releases and rechecks the owner roster before qualifying the
evidence. Two current owners must each have a matched recent upload. Teams, unrelated
publishers, stale activity and incomplete evidence do not qualify; insufficient
evidence stays unknown, without a single-developer claim. Distinct accounts do not
prove distinct people. Retained strict components keep these authority checks;
default package advice uses the lighter policy above.

When the strict path cannot establish active maintenance, it also reports
current registry roles as context. npm uses the package's current top-level owner
list; PyPI uses `ownership.roles`. Author credits, version-level maintainer fields
and unverified publisher names are ignored. `evidence.maintenance` reports observed
named-account counts with `active_authority_accounts: null`; `--explain` includes
the account/role list and opaque organization access. These observations cannot
satisfy the active-maintainer gate or independently permit installation. An empty
organization-managed role list stays unknown, without a zero-maintainer claim.

Retained strict Cargo reports can include `evidence.maintenance` with the basis
`cargo-registry-publishers` and observed authority/active-authority counts.
`--explain` adds registry account IDs, matched releases and remaining gaps under
`details.maintenance`. Public evidence is cached per crate, never borrowed from
another package sharing its repository. A new policy digest prevents earlier
policy approvals from being reused for installation. All other safety gates apply.
Checks recommend versions only when every required check passes. Unsupported
ordering or unresolved newer candidates prevent a newest-version recommendation.

The retained legacy npm installer (not the default npm command route) installs
only a fully approved project plan. It requires Node 24+, npm 10+, lockfile v3 and a supported
locked version, dependency-free addition or the guarded upgrade described above.
Executable-bin packages, links, bundled
dependencies and embedded shrinkwrap are unsupported. Artifacts are downloaded from
the public npm registry, checked against SHA-512 and inspected for archive escapes,
unexpected metadata and expansion limits before native tooling runs. npm uses an
isolated cache, offline mode and disabled hooks; credential variables and Node preload
options are not inherited. Installation stages the tree before committing files and
preserves recovery files if rollback encounters concurrent edits. Avoid running other
package managers concurrently. These retained strict execution components cover the limited npm and Cargo
scopes described here; default npm delegation is described above.

`--explain` reports candidate findings, grouped advisories, source timestamps,
concrete npm plans and transitive paths. JSON reports
`scope: "project-resolution"` only for supported plans; partial package findings
use `scope: "package"`. Npm package advice authorizes native delegation only; it
does not approve or verify the resulting project installation. Provider work is
bounded to 128 requests and 32 MiB of input per assessment, with one bounded retry
for transient HTTP statuses. Exhausted budgets remain unknown.

Dependency exit codes are `0` for approval/success,
`1` for policy denial, `2` for invalid input/configuration or an unindexed exact
version, `8` for insufficient evidence, `9` for unsupported coverage/installation,
and `10` for output/operational failure. After npm, Cargo, Go or uv starts, the native exit status
is returned instead; interrupted/cancelled delegation returns 130. Other existing
command exit codes are unchanged.

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

Python support covers one package per repository, with no binary mode, target matrix,
or PyPI upload. Keep `build.targets` unset. Package filenames follow Python distribution naming,
so `build.artifactTargetSuffix` does not apply. TOML version sources require a static
`[project].version` following the existing semantic-version rule (for example `1.2.3`).
Dynamic versions and Python-specific versions such as `1.2.3rc1` are unsupported.

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
are outside the supported target mapping.

`release.versionSource` can point to a package's `Cargo.toml`, including workspace-inherited
`package.version`. A virtual workspace manifest has no package release version: point to a member
manifest instead. Releases use fresh build assets only, preserve subdirectories when staging into
`release.artifactDir`, and support notes, drafts, and releases without assets. crates.io publishing
is unsupported.

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
folder. Omit it when the package has exactly one executable product. Builds
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
signing, and notarization are unsupported.

Swift compatibility checks run through `.github/workflows/swift.yml`. They prioritize
macOS arm64 development with Xcode 16.2 (Swift 6.0.3) and Xcode 26.2, plus one GNU/Linux
amd64 pipeline check using the official Swift 6.2.4 container. Each job reports its
actual toolchain and host and runs `make test-swift-e2e`. Hosted matrix results are
available after the workflow is pushed and run; a local pass validates only the local host.
The fixture checks both test frameworks, aggregated parameterized test successes and
failures, coverage thresholds, direct and transitive dependency lock enforcement,
executable packaging, and simulated releases. Releases in this suite never publish
to GitHub. Real Swift integration remains opt-in for local `make test` runs.

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
  tagPrefix: "v"
  notesMode: "generate-notes"
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
Keep the library example's `versionSource`, `tagPrefix`, and `notesMode` fields when
changing its release block for a CLI. Both examples use the shared project and
repository configuration above.
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

Support covers one root Gradle JVM project, with `packages: ["./..."]`.
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
become CLI builds. Support covers one root package with `packages: ["./..."]`.
Workspaces, TypeScript project references, Windows, cross-compilation, npm package
archives/publication, and automatic copying of external runtime resources are unsupported. Compiled CLIs disable automatic runtime loading of `.env` and Bun config.

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

## Independent e2e tests and scripts

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

## Metadata pipelines

Run `gh flarebyte meta` from the repository root to execute
`pipeline-*-maat.thoth.cue` files in filename order. Install both tools first:

```sh
brew install flarebyte/tap/thoth-ostraca
npm install --global maat-ostraca
gh flarebyte meta
```

See [Thoth](https://github.com/flarebyte/thoth-ostraca) and
[Maat](https://github.com/flarebyte/maat-ostraca) for installation alternatives
and Node.js requirements. Missing tools produce installation guidance; the
command does not install them.

Each pipeline gets an `OK` or `ERROR` result. Successful tool output is suppressed;
failures include bounded diagnostics. The final summary counts content changes
under `thoth-meta/`, including created and deleted files and partial changes from
failed pipelines. Timestamp-only rewrites do not count as updates. Any failed
pipeline or metadata inspection returns exit code 1; other pipelines continue
after an individual failure. Interrupting the command stops further launches.

Discovery is limited to the current directory and is independent of the build
language. `.gh-flarebyte.cue` is optional and only informs a filename suggestion
when no pipelines exist. That case exits successfully as an explicit no-op and
creates no files. Symlink pipelines and symlinks within the metadata tree are
rejected.

Run only trusted pipelines: Thoth configurations can execute code. Existing
pipelines may invoke `npx maat-ostraca`, whose package resolution and possible
downloads remain controlled by those pipelines, even when `maat` is installed.

## Carbon-copy checks

Run `gh flarebyte cc` from the repository root (or `make dup` in this checkout).
Install [jscpd](https://github.com/kucherenko/jscpd) v4 or v5 on your PATH first:
`brew install jscpd` on macOS or Linux with Homebrew. See the
[official installation guide](https://jscpd.dev/start/installation) for other Linux options.
The command never installs tools or invokes npx.

The only optional setting is:

```cue
cc: { minLines: 10 }
```

`minLines` defaults to 10 and must be a positive integer. The scan covers common
source formats across the repository, including root files, production folders,
and tests; it compares files across folders in the same run. Git ignores and
built-in exclusions remove dependencies, caches, build outputs, documentation,
and common generated filenames. Unrecognized generated filenames may still be scanned.
Repository `.jscpd.json` settings are replaced by the command's fixed policy.
Detection uses jscpd's mild mode and a minimum of 50 tokens as well as `minLines`.

Output lists relative paths and line ranges for the largest 10 duplicate pairs,
with stable ordering and a total/remaining count. Fix those pairs and rerun to
see the rest. Source snippets and raw tool output are omitted. Temporary JSON
reports are cleaned up after each run. A clean scan exits 0; duplicates, missing
tooling, unsupported versions, or scan/report failures exit 1 with diagnostics.

## CI coverage budget

The [contributor guide](CONTRIBUTING.md#ci-coverage-budget) describes the eight-job compatibility matrix, including macOS and Linux Kotlin checks.

## Manual GitHub releases

Maintainers can follow the [manual release workflow](CONTRIBUTING.md#manual-github-releases). It publishes immediately after verification; library and CLI release behavior for managed projects is described in the language guides above.

## Typical workflow

1. Run `gh flarebyte repo init --repo <owner/name>` in a repo that should be managed.
2. Edit `.gh-flarebyte.cue` to match the desired repository state.
3. Run `gh flarebyte config validate` to check the configuration.
4. Run `gh flarebyte repo update` to sync the repo, then `gh flarebyte repo audit` to check for drift.
5. Run `gh flarebyte test`, `gh flarebyte format`, `gh flarebyte lint`, and `gh flarebyte cov` for local quality checks.
6. Run `gh flarebyte build` and `gh flarebyte release` when you are ready to ship.

## Makefile shortcuts

See [local development and checks](CONTRIBUTING.md#local-development) and the [Makefile shortcuts](CONTRIBUTING.md#makefile-shortcuts). Run `make help` to list available targets.

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
