# Dependency checks and installation

`gh flarebyte dep` assesses a named public package. Run it inside a local Git
repository; configuration comes from `.gh-flarebyte.cue` at that repository's root.
Checks require neither a manifest/lockfile nor the language's package manager.

```sh
gh flarebyte dep check npm lodash --json
gh flarebyte dep check pypi requests@2.32.0 --explain
gh flarebyte dep check swift https://github.com/apple/swift-nio --json
```

Versions in examples are input syntax, not safety recommendations. Exact requests
are checked as supplied, with documented ecosystem normalization. A vulnerable
requested version stays rejected even when a newer candidate is acceptable.

## Decisions and version choice

| Text | JSON decision | Meaning |
| --- | --- | --- |
| RECOMMEND | allow | Required named-package evidence satisfies the adoption policy |
| REVIEW | unknown | Required evidence is missing or ambiguous |
| AVOID | deny | Known advisory, withdrawal or another adoption-policy rejection |

The default policy waits 14 days after publication. Configure only that waiting
period through `dependencies: { minAgeDays: 14 }`; explicit zero disables it.
Checks consider stable releases newest-first, with at most four versions assessed.
An unresolved newer eligible candidate prevents a newest-version recommendation.

Known direct advisories and withdrawn releases reject a version. A corroborated
repository with fewer than 100 stars, or a complete single-user registry owner
roster, rejects adoption under a heuristic policy; neither proves malware.
Lookalike screening uses a small curated list and is not exhaustive. Missing
maintainer activity/count or stale/unavailable Scorecard adds warnings.

A package cannot borrow reputation through an unverified repository declaration.
`candidate_version` can identify a mature release with no returned known advisories
while a source/reputation gap remains; it does not authorize installation. Dart's
current pub.dev adapter provides only declared source links, so it produces review
or rejection rather than an installable recommendation. No production override or
new identity-verification subsystem is provided.

## Ecosystem coverage

OSV supplies direct advisory findings; available GitHub reputation and optional
OpenSSF Scorecard observations provide repository context. All calls use bounded
requests, timeouts and response sizes. No package archives or source code are run.

| Selector and coordinate | Catalog/lifecycle source | Important limits |
| --- | --- | --- |
| `npm lodash` | deps.dev; npm registry release and current owners | Repository ownership must be corroborated; numeric stable versions and supported build metadata |
| `cargo serde` | deps.dev; crates.io release/yanking and current owners | Teams do not establish user counts; no features/MSRV or Cargo graph checks |
| `pypi requests` | deps.dev; PyPI release/yanking and registry roles | Public epochs/post releases supported; local versions and unsupported ordering remain review |
| `go github.com/spf13/cobra` | deps.dev release metadata | GitHub module-path association is path-based evidence; vanity paths can remain unverified; pseudo-version ordering and retraction checks are unsupported |
| `maven org.slf4j:slf4j-api` | deps.dev indexed release/deprecation metadata | One to four numeric components; unsupported qualifiers remain review; lifecycle coverage is limited |
| `pub http` | pub.dev package catalog and optional publisher context | Retraction/discontinuation checked; publisher domain is not owner count or source proof; three-part numeric ordering only, excluding build metadata |
| `swift https://github.com/apple/swift-nio` | Public GitHub releases and repository metadata | GitHub only; at most five pages of 100 releases; publication means GitHub release date, not tag creation; bare tags are not discovered |

Swift accepts an optional `.git` suffix and normalizes a single `v` version prefix.
SwiftURL advisory queries use canonical scheme-less repository coordinates;
short-name advisory aliases are not probed. Swift maintainer count remains unknown.
Empty, ambiguous or incomplete release catalogs cannot establish a recommendation.

PyPI may make up to four additional availability probes for unsupported catalog
versions. Confirmed unavailable/fully yanked entries can be excluded; explain
output records those exclusions. The four assessed-candidate budget still applies.

## Output

`--json` and `--explain` are the check presentation flags. JSON preserves the existing
report schema: `package`, `description`, `requested`, `scope`, `decision`, `reasons`,
`recommendation`, `alternatives`, `evidence`, `policy`, `policy_digest`, `checked_at`
and `schema`. Package advice adds `coverage: "direct-package-only"`, optional
`warnings` and optional `candidate_version`. Recommendations are objects containing
`version`, `age_days` and `compatibility: "package-only"`; an absent recommendation
is null. This is not a schema with `recommended_version` or an install-command object.

Text labels a candidate as `Candidate (review required)` and a better recommendation
for an exact request as `Recommended alternative`. Descriptions are bounded plain
text. `--explain` adds sources/timestamps, checked versions and evidence details;
it can be combined with `--json`. An empty advisory result means no returned known
advisories within that provider's coverage, not proof of universal safety.

## Installation

```sh
gh flarebyte dep install npm lodash
```

Installation first checks the named package. Review/avoid or a missing complete
recommendation starts no package manager. An explicit requested version must pass
its own check and is never silently replaced by an alternative.

| Selector | After approval |
| --- | --- |
| npm | `npm install --save-exact -- NAME@VERSION` |
| cargo | `cargo add NAME@=VERSION` |
| pypi | `uv add NAME==VERSION` |
| go | `go get MODULE@VERSION` |
| pub | `dart pub add NAME:VERSION` — adapter implemented; current source-identity gap blocks approval |
| maven | Print an exact declaration for a single regular local Gradle/Maven build file, or manual guidance if missing/ambiguous |
| swift | Print an exact `.package(url: ..., exact: ...)` declaration for `Package.swift` |

Maven and Swift state `PRINT ONLY — nothing installed`; no files change or tools
run. Other adapters directly execute fixed tools with validated argument arrays,
from the invocation directory, without a shell or automatic SDK installation.
Native stdout and stderr stream to stdout; gh-flarebyte's diagnostics use stderr.
Stdin and the normal environment/configuration are inherited. Custom native
registries remain native configuration: checks cover the public registries above,
not a custom host selected by the package manager.

`--json` and `--explain` are rejected for install before assessment or launch.
The native manager owns dependency resolution, transitive choices, compatibility,
hooks and manifest/lockfile updates. Failure can leave normal partial changes.
There is no staging, rollback, transitive preflight or installation-certification
requirement in this workflow. Native output normalization is deferred.

## Exit status and validation

Checks use 0 for recommendation, 1 for rejection, 2 for invalid input/configuration
or an exact version absent from the catalog, and 8 for insufficient evidence.
Code 9 is reserved for unsupported coverage/installation paths; 10 denotes wrapper
output/operational failures. Native delegation returns the child's exit status;
missing tools fail with 10 and cancellation returns 130. Native status values may
therefore overlap check codes.

See [contributor validation guidance](../CONTRIBUTING.md#dependency-advice-and-delegation).
Default tests use synthetic providers and native-tool stubs; that is not a live
cross-ecosystem acceptance or real-installation certification. The older proof
machinery is documented separately in [retained-component notes](dependency-legacy.md).
