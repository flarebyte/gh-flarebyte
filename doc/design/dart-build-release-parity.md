# Dart Build/Release Parity Plan

## Goal
Add first-class Dart support for `gh flarebyte build` and `gh flarebyte release`, matching current Go ergonomics where practical.

## Current State
- Dart is supported for dev commands:
  - `test`, `format`, `lint`, `cov`
  - `test --style per_test` via Dart JSON reporter
- Dart is not supported for `build` and therefore not for `release` (release shells through build).

## Proposed Scope
1. Build modes
- Support `build.language: "dart"` for `build.mode: "library"` first.
- Keep `build.mode: "binary"` for Dart out of scope initially unless a concrete packaging target is defined.

2. Dart library build contract
- Validate package via:
  - `dart pub get`
  - `dart analyze`
  - `dart test`
- For `library` mode, treat successful validation as build success (similar to Go compile verification semantics).

3. Release contract for Dart
- Allow `release` when:
  - `build.language: "dart"`
  - `build.mode: "library"`
  - `release.includeArtifacts: false`
- Continue to reject `release.includeArtifacts: true` for Dart until artifact strategy is defined.

## Config and Validation Changes
- Keep `build.language` enum as `go|dart`.
- Add cross-field validation:
  - If `language=dart` and `mode=binary`, return usage error with guidance.
  - If `language=dart` and `release.includeArtifacts=true`, return usage error with guidance.

## CLI Changes
1. `cmd_build.go`
- Add Dart branch for `library` mode with explicit step reporting.
- Return clear actionable errors on unsupported Dart binary mode.

2. `cmd_release.go`
- Reuse existing flow after `build` succeeds.
- Keep artifact listing rules unchanged; Dart release path requires `includeArtifacts=false`.

3. Output
- Keep summary style consistent with existing `BUILD PASS/FAIL` and `RELEASE PASS/FAIL` output.

## Test Plan
- Unit tests:
  - Dart library build success/failure command routing
  - Validation failures for unsupported Dart binary mode
  - Release success with `includeArtifacts=false`
  - Release validation failure for `includeArtifacts=true`
- Regression tests:
  - Existing Go build/release behavior unchanged.

## Open Decisions
- Whether to support Flutter apps as a distinct mode later (`flutter build` pipeline).
- Whether Dart binary packaging is needed (e.g., `dart compile exe` with per-target assets).
