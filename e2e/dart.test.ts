import { describe, expect, test } from 'bun:test';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { cwd } from 'node:process';
import { makeTempDir, runCLI } from './helpers';

const DART_CONFIG = `package ghflarebyte

project: { org: "flarebyte", repo: "gh-flarebyte" }
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
  language: "dart"
  outputDir: "build"
  checksumFile: "build/checksums.txt"
  targets: ["linux-amd64"]
}
devOutput: {
  color: "false"
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
  artifactDir: "build"
  includeChecksums: true
}
`;

async function installDartStub(tempDir: string): Promise<string> {
  const binDir = join(tempDir, 'fake-bin');
  await mkdir(binDir, { recursive: true });
  const stubPath = join(binDir, 'dart');
  const script = `#!/bin/sh
set -eu
printf '%s\n' "$*" >> .dart-invocations.log
if [ "$1" = "format" ]; then
  exit 0
fi
if [ "$1" = "analyze" ]; then
  exit 0
fi
if [ "$1" = "test" ]; then
  if [ "\${2:-}" = "-r" ] && [ "\${3:-}" = "json" ]; then
    printf '%s\n' '{"type":"testStart","test":{"id":1,"name":"adds numbers"}}'
    printf '%s\n' '{"type":"testDone","test":{"id":1,"name":"adds numbers"},"result":"success","hidden":false}'
    exit 0
  fi
  if [ "\${2:-}" = "--coverage" ]; then
    mkdir -p .dart_tool/coverage
    cat > .dart_tool/coverage/lcov.info <<'EOF'
SF:lib/a.dart
LF:10
LH:10
end_of_record
EOF
    exit 0
  fi
  exit 7
fi
exit 1
`;
  await writeFile(stubPath, script, { mode: 0o755 });
  return binDir;
}

describe('gh-flarebyte dart e2e', () => {
  async function setupDartE2E() {
    const repoRoot = cwd();
    const tempDir = await makeTempDir(repoRoot);
    await writeFile(join(tempDir, '.gh-flarebyte.cue'), DART_CONFIG);
    const fakeBinDir = await installDartStub(tempDir);
    const pathEnv = `${fakeBinDir}:${process.env.PATH ?? ''}`;
    return { repoRoot, tempDir, pathEnv };
  }

  test('format runs with dart language config', async () => {
    const { repoRoot, tempDir, pathEnv } = await setupDartE2E();
    const formatRes = await runCLI(['format'], tempDir, repoRoot, {
      PATH: pathEnv,
    });
    expect(formatRes.code).toBe(0);
    expect(formatRes.stdout).toContain('FORMAT PASS');
  });

  test('test --style per_test runs with dart language config', async () => {
    const { repoRoot, tempDir, pathEnv } = await setupDartE2E();
    const testRes = await runCLI(
      ['test', '--style', 'per_test'],
      tempDir,
      repoRoot,
      {
        PATH: pathEnv,
      },
    );
    expect(testRes.code).toBe(0);
    const invocations = await Bun.file(
      join(tempDir, '.dart-invocations.log'),
    ).text();
    expect(invocations).toContain('test -r json');
    expect(testRes.stdout).toContain('✓ adds numbers');
    expect(testRes.stdout).toContain('TEST PASS');
  });

  test('lint runs with dart language config', async () => {
    const { repoRoot, tempDir, pathEnv } = await setupDartE2E();
    const lintRes = await runCLI(['lint'], tempDir, repoRoot, {
      PATH: pathEnv,
    });
    expect(lintRes.code).toBe(0);
    expect(lintRes.stdout).toContain('LINT PASS');
  });

  test('cov runs with dart language config', async () => {
    const { repoRoot, tempDir, pathEnv } = await setupDartE2E();
    const covRes = await runCLI(['cov'], tempDir, repoRoot, {
      PATH: pathEnv,
    });
    expect(covRes.code).toBe(0);
    expect(covRes.stdout).toContain('COV PASS');
    expect(covRes.stdout).toContain('total=100.00%');
  });
});
