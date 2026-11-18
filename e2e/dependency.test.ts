import { describe, expect, test } from 'bun:test';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { makeTempDir, runCLI } from './helpers';

describe('dependency assessment safety', () => {
  test('help exposes only presentation flags', async () => {
    const result = await runCLI(['dep', 'check', '--help'], process.cwd());
    expect(result.code).toBe(0);
    expect(result.stdout).toContain('--json');
    expect(result.stdout).toContain('--explain');
    expect(result.stdout).not.toContain('--min-age-days');
    expect(result.stdout).not.toContain('--project-dir');
  });

  test('install rejects presentation flags before changing project files or using network', async () => {
    const root = process.cwd();
    const dir = await makeTempDir(root);
    await mkdir(join(dir, '.git'));
    const manifest = 'name: local\nenvironment:\n  sdk: ^3.0.0\n';
    await writeFile(join(dir, 'pubspec.yaml'), manifest);
    // Install flags are rejected before assessment or native Dart launch.
    const result = await runCLI(
      ['dep', 'install', 'pub', 'archive', '--json'],
      dir,
      root,
    );
    expect(result.code).toBe(2);
    expect(result.stdout).toBe('');
    expect(result.stderr).toContain('--json and --explain are check-only');
    expect(await readFile(join(dir, 'pubspec.yaml'), 'utf8')).toBe(manifest);
  });

  test('invalid exact input returns a structured failure before network', async () => {
    const result = await runCLI(
      ['dep', 'check', 'npm', 'lodash@^4', '--json'],
      process.cwd(),
    );
    expect(result.code).toBe(2);
    expect(JSON.parse(result.stdout).error.code).toBe('INVALID_INPUT');
  });
});
