# Dart library builds and releases

Dart supports `test`, `format`, `lint`, and `cov`, plus library verification through
`build` and GitHub releases without assets. See the [Dart configuration example](../../README.md#dart)
for the shared project, output, and release settings.

## Build behavior

Use `build.language: "dart"` and `build.mode: "library"`. A build runs:

1. `dart pub get`
2. `dart analyze`
3. `dart test` when `build.runTests: true`

Success prints `Build complete: Dart library validation passed.` The build verifies
the project without packaging a binary or writing a checksum manifest.
`build.runTests` defaults to `false`; the standalone `test` command runs tests
independently of that setting.

Dart binary and package builds are unsupported. `build --target` is rejected;
there is no Dart cross-compilation or executable artifact matrix.

## Release behavior

Set `release.includeArtifacts: false`. `release` first runs the library build, then
resolves the configured version and tag, checks for an existing tag, and creates a
GitHub release with the configured notes and optional `--draft` flag. It publishes
without artifact uploads. `release.includeArtifacts: true` fails for Dart.

Release notes use the shared `generate-notes`, `notes-from-tag`, or `notes-file`
policy. `notes-file` requires `release.releaseNotesFilePath` in the configuration;
`--notes-file` can override that configured path. Version files use the shared
YAML/JSON/TOML readers; `pubspec.yaml` is usable when it has a semantic `version`
field accepted by the existing YAML reader.

## Developer commands and coverage

- `test` runs `dart test`, using JSON events for `--style per_test`.
- `format` runs `dart format .`.
- `lint` runs `dart analyze`.
- `cov` runs `dart test --coverage .dart_tool/coverage`, reads LCOV, and applies
  `coverage.min`, `coverage.enforceMin`, and `--min`. If LCOV is absent, it attempts
  generation with the Dart coverage package.

The shared `devOutput` and failure-only options apply to tests, lint, and coverage.
The current developer flows do not invoke Flutter or `dart compile exe`.

## Implementation and verification

[Build dispatch](../../internal/cli/cmd_build.go) and
[release orchestration](../../internal/cli/cmd_release.go) implement this behavior.
Command routing, build/release gates, output, and coverage policies have Go tests
under [internal/cli](../../internal/cli). The
[Dart E2E fixture](../../e2e/dart.test.ts) uses a stub Dart executable;
`make test-dart-e2e` does not verify against a real Dart SDK.
