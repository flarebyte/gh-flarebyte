# Retained dependency inspection components

These implementation notes describe retained strict assessors, planners and
installers. The default `dep check` and `dep install` commands use the simpler
[named-package workflow](dependencies.md). They do not run these graph, artifact,
runtime or staged-installation checks. There is no legacy-mode flag or separate
public CLI entry point promised for these components.

Their regression tests remain supported; extending this machinery is outside the
current feature scope. References below to explained fields describe retained
component reports, not the default CLI response.

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
default package advice uses the [lighter policy](dependencies.md#decisions-and-version-choice).

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
scopes described here; default native delegation is described in the [command guide](dependencies.md#installation).

`--explain` reports candidate findings, grouped advisories, source timestamps,
concrete npm plans and transitive paths. JSON reports
`scope: "project-resolution"` only for supported plans; partial package findings
use `scope: "package"`. Npm package advice authorizes native delegation only; it
does not approve or verify the resulting project installation. Provider work is
bounded to 128 requests and 32 MiB of input per assessment, with one bounded retry
for transient HTTP statuses. Exhausted budgets remain unknown.

