# Contributing

This repository builds the Go `gh-flarebyte` executable. Its language adapters run
external toolchains; they are not separately published libraries. Usage and managed
project configuration are documented in the [README](README.md).

## Local development

Use Go 1.24.1 or newer (see [go.mod](go.mod)), GNU Make or compatible `make`, and Bun
1.3.6, the version pinned in CI. The release-preflight tests additionally need Python
3.11+; CI uses Python 3.12. The core Go CLI can be built without the optional language
SDKs or GitHub credentials.

Run these commands from the repository root:

```sh
make build-go
.e2e-bin/gh-flarebyte --help
.e2e-bin/gh-flarebyte config validate
```

Use `.e2e-bin/gh-flarebyte` to run the current checkout. The Makefile builds there;
`gh flarebyte` invokes whichever version is installed as a GitHub CLI extension.
Builds use repository-local Go caches. Developer commands inherit their caller's
environment, so keep those cache paths when running checks:

```sh
export GOCACHE="$PWD/.gocache"
export GOMODCACHE="$PWD/.gomodcache"
make e2e-deps
make test
make lint
python3 -m unittest discover -s script -p 'test_*.py'
```

`make e2e-deps` installs locked Bun dependencies for Biome. Go may download module
dependencies when building or testing. `make test` runs Go tests and the default Bun
smoke/stub suites; real language integrations are opt-in. `make lint` runs Go vet and
Biome. `make format` formats Go and Biome-managed files; `make review` formats before
running tests and lint. For focused Go tests, for example:

```sh
go test ./internal/config
go test ./internal/cli -run 'TestKotlin|TestLanguageDeveloperContract'
```

`make coverage` runs configured coverage checks; `make test-race` runs Go's race
detector. Use `make check-tools` for executable discovery and `make install-tools-help`
for optional tooling guidance. Integration prerequisites are listed below. Installing
or running the CLI does not require the SDKs for every supported language.

## Where to make changes

| Path | Responsibility |
|---|---|
| [cmd/gh-flarebyte](cmd/gh-flarebyte) | Process entry point and runtime metadata |
| [internal/config](internal/config) | Literal config parsing, defaults, validation |
| [internal/dependencies](internal/dependencies) | Bounded provider clients, dependency policy and partial evidence reports |
| [internal/cli](internal/cli) | Command wiring, tool execution, reports, repository and release flows |
| [internal/cli/language_contract_test.go](internal/cli/language_contract_test.go) | Shared output, flag, and coverage policies across languages |
| [e2e](e2e) and [fixtures](e2e/fixtures) | Smoke tests and opt-in real-toolchain suites |
| [script](script) | Release preflight and its Python tests |
| [doc/design-meta](doc/design-meta) | Authored inputs for generated design documentation |

Commands enter through `cli.Run` and Cobra, load and validate `.gh-flarebyte.cue`,
then dispatch to repository operations or a language adapter. The adapters use shared
execution, output, coverage, checksum, and artifact-staging helpers. For a language
change, keep help, config validation, README examples, and the shared contract tests
consistent; add real-toolchain coverage in the corresponding fixture when needed.

Dependency tests use fake HTTP transports and temporary repositories; they never
call live registries or install third-party packages. Run focused checks with
`go test ./internal/dependencies ./internal/config` and
`go test ./internal/cli -run '^TestDep'`.
The new dependency core has a 90% statement coverage floor in CI; security failure
cases still require explicit tests regardless of that percentage. Seeded fuzz
tests cover package inputs, descriptions, dependency policy parsing, project JSON
and npm range/Python version parsing. Python ordering tests cover epochs, numeric
post releases, zero padding, prerelease/dev exclusions, integer overflow and
unsupported local versions. Test malformed/changed lockfiles, missing transitive edges,
artifact or registry metadata disagreement, conflicting provenance, advisory aliases,
snapshot selection and evidence budgets. Concrete plan checks must never execute
package tooling. The guarded installer has an opt-in synthetic integration:
`GH_FLAREBYTE_DEP_NPM_INTEGRATION=1 go test ./internal/cli -run '^TestDependencyNPMOffline.*Integration$'`.
It uses a checked-in synthetic artifact and no registry network access, and verifies
that its lifecycle hook does not run during addition or upgrade. Upgrade tests must
retain all other nodes' findings, reject incompatible incoming constraints and
preserve the original manifest, lockfile and installed version after staged failure.
SDK flags, environment isolation, artifact
preflight, staged failures and conditional rollback have offline unit tests. Keep
identity, maintainer and resolution gaps blocking; do not enable installation by
assuming stars, contributors or a high Scorecard score establish safety.

Scorecard transport tests must cover the public API's `YYYY-MM-DD` scan dates as
well as RFC3339 timestamps. Interpret date-only values at midnight UTC, preserve
all other response fields, and keep malformed dates and missing/future/stale scans
blocking. A successful HTTP response is not proof that the payload was decoded.
Live checks should record exact package versions, provider statuses and local
resolution limitations; do not weaken authority or policy gates to produce an allow.

Cargo assessment tests use TOML fixtures and fake crates.io/deps.dev/OSV/GitHub
responses. Cover root-lock agreement, version/source ambiguity, checksum and edge
disagreement, build/dev dependencies, renamed crates, feature/compiler uncertainty,
configuration changes and malformed or linked files. The existing TOML dependency
is reused. Fuzz Cargo requirement parsing and bounded manifest/lock parsing without
invoking Cargo. Keep Cargo native installation gated independently of check success.
Compiler-baseline tests cover numeric/partial Rust versions, transitive requirements,
edition floors, missing/invalid declarations, unchanged security verdicts and
malicious upstream text that must not enter explained evidence. Keep the comparison
against the manifest's declared `rust-version`; do not infer an installed toolchain
or silently raise the project's baseline to permit a dependency.

Feature tests cover defaults, namespaced/implicit optional activation, renaming,
strong/weak forwarding, cycles, metadata gaps, event budgets and unchanged security
gates. The opt-in offline oracle requires a standalone Cargo SDK:
`GH_FLAREBYTE_DEP_CARGO_INTEGRATION=1 go test ./internal/dependencies -run '^TestCargoFeatureOracle$'`.
It compares generated path-only fixtures with Cargo metadata, checks that build
scripts do not run, and preserves an unknown verdict for inactive weak contexts
where metadata reports a broader union. These controlled path fixtures do not
enable path dependencies in production assessment. Default tests never invoke Cargo.

Cargo archive tests must bind downloads to sealed approval facts, reject changed
projects and unsafe origins, compare dependency kinds/targets/features/runtime
metadata, and cover archive escapes, aliases, links, embedded configuration,
trailer corruption and resource budgets. Packaged `Cargo.lock`, `Cargo.toml.orig`
and VCS information are inert and must not replace the approved resolution or
provenance. Run the controlled archive oracle with
`GH_FLAREBYTE_DEP_CARGO_INTEGRATION=1 go test ./internal/dependencies -run '^TestCargoArchiveOracle$'`.
It packages only generated fixtures offline with `--no-verify`, checks the resulting
crate, and asserts that its build script did not run. Archive preflight never extracts
files or authorizes a native install.

Cargo staging tests cover private external directories, exclusive writes, archive
budgets, vendor checksum generation and unchanged source inputs. Native metadata
must match approved identities, registry sources, manifest locations, graph edges
and default features; modified staged inputs must fail. SDK tests verify the fixed
metadata command, bounded output, compatible versions, host validation and removal
of credentials, inherited options and compiler wrappers. Refuse ancestor Cargo
configuration and Rustup proxies. Run both offline staging/SDK oracles with:

```bash
GH_FLAREBYTE_DEP_CARGO_INTEGRATION=1 go test ./internal/dependencies ./internal/cli -run '^TestCargoStageOracle$|^TestDependencyCargoSDKIntegration$'
```

Only generated fixtures and a standalone SDK are used. Build-script/proc-macro
fixtures must remain unexecuted. The synthetic root proves a staged resolution;
it does not prove original project target/unit equivalence or authorize installation.

The guarded Cargo installer additionally binds the original pinned manifest and
default source target shape. Keep its initial scope to ordinary runtime library
graphs without custom targets, root dev/build dependencies, build scripts or proc
macros. Test string, inline, dotted and dependency-table pins without rewriting
comments/options. Preserve the locked graph, refuse stale/changed policy and
inputs, and retain user edits and backups on rollback conflicts. Synthetic provider
responses and archives are in `internal/dependencies/testdata/cargo-install.json`;
the approval boundary uses the real assessor with a fake HTTP transport.
Run the complete offline native install oracle with:

```bash
GH_FLAREBYTE_DEP_CARGO_INTEGRATION=1 go test ./internal/cli -run '^TestCargoDependencyInstallIntegration$'
```

It covers library, binary, mixed and renamed dependencies with the standalone SDK.
No registry network access or shared Cargo cache is used.

[Design notes](doc/design/gh-flarebyte.md) are generated from
[app.cue](doc/design-meta/app.cue) and its example inputs. Change the authored inputs
and use `make doc-design` (requires `flyb`) to validate and regenerate them. Do not
hand-edit generated Markdown. `make thoth-meta` refreshes code metadata with `thoth`.

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
