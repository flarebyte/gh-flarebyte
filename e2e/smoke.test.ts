import { describe, expect, test } from 'bun:test';
import { readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { cwd } from 'node:process';
import { makeTempDir, runCLI } from './helpers';

describe('gh-flarebyte smoke', () => {
  test('--help returns usage text', async () => {
    const repoRoot = cwd();
    await makeTempDir(repoRoot);
    const result = await runCLI(['--help'], repoRoot);
    expect(result.code).toBe(0);
    expect(result.stderr).toBe('');
    expect(result.stdout).toContain('Usage:');
    expect(result.stdout).toContain('gh flarebyte --version [--json]');
  });

  test('--version --json returns documented shape', async () => {
    const repoRoot = cwd();
    const result = await runCLI(['--version', '--json'], repoRoot);
    expect(result.code).toBe(0);
    expect(result.stderr).toBe('');
    const payload = JSON.parse(result.stdout) as Record<string, unknown>;
    for (const key of [
      'version',
      'commitId',
      'date',
      'os',
      'arch',
      'goVersion',
    ]) {
      expect(payload[key]).toBeDefined();
    }
  });

  test('repo init writes config with defaults on metadata import failure', async () => {
    const repoRoot = cwd();
    const tempDir = await makeTempDir(repoRoot);
    const result = await runCLI(
      ['repo', 'init', '--repo', 'flarebyte/gh-flarebyte'],
      tempDir,
      repoRoot,
    );

    expect(result.code).toBe(0);
    expect(result.stdout).toContain('created ');
    expect(
      result.stdout.includes('from defaults') ||
        result.stdout.includes('from current GitHub state'),
    ).toBeTrue();
    const content = await readFile(join(tempDir, '.gh-flarebyte.cue'), 'utf-8');
    expect(content).toContain('org:  "flarebyte"');
    expect(content).toContain('repo: "gh-flarebyte"');
  });

  test('repo init blocks overwrite unless --overwrite is passed', async () => {
    const repoRoot = cwd();
    const tempDir = await makeTempDir(repoRoot);
    await writeFile(
      join(tempDir, '.gh-flarebyte.cue'),
      'package ghflarebyte\n',
    );
    const result = await runCLI(
      ['repo', 'init', '--repo', 'flarebyte/gh-flarebyte'],
      tempDir,
      repoRoot,
    );

    expect(result.code).toBe(2);
    expect(result.stderr).toContain('already exists');
    expect(result.stderr).toContain('--overwrite');
  });

  test('repo audit --json returns stable report shape', async () => {
    const repoRoot = cwd();
    const tempDir = await makeTempDir(repoRoot);
    await writeFile(
      join(tempDir, '.gh-flarebyte.cue'),
      `package ghflarebyte

project: { org: "flarebyte", repo: "gh-flarebyte" }
sync: { mode: "push", managedFields: ["topics"] }
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
    {
      name: "enhancement"
      color: "0E8A16"
      description: "New feature"
    },
  ]
}
build: {
  language: "go"
  outputDir: "build"
  checksumFile: "build/checksums.txt"
  targets: ["linux-amd64"]
}
release: {
  versionSource: "main.project.yaml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  artifactDir: "build"
  includeChecksums: true
}
`,
    );
    const result = await runCLI(['repo', 'audit', '--json'], tempDir, repoRoot);
    expect(result.code).toBe(0);
    const payload = JSON.parse(result.stdout) as Record<string, unknown>;
    expect(payload.repo).toBe('flarebyte/gh-flarebyte');
    expect(payload.driftCount).toBeDefined();
    expect(payload.hasDrift).toBeDefined();
    expect(Array.isArray(payload.diffs)).toBeTrue();
  });

  test('repos mine --json returns report shape', async () => {
    const repoRoot = cwd();
    const tempDir = await makeTempDir(repoRoot);
    const result = await runCLI(
      ['repos', 'mine', '--org', 'flarebyte', '--json'],
      tempDir,
      repoRoot,
    );
    expect(result.code).toBe(0);
    const payload = JSON.parse(result.stdout) as Record<string, unknown>;
    expect(payload.org).toBe('flarebyte');
    expect(payload.contributor).toBeDefined();
    expect(payload.count).toBeDefined();
    expect(Array.isArray(payload.repos)).toBeTrue();
  });
});
