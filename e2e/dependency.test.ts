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

  test('unsupported install leaves project files unchanged without network', async () => {
    const root = process.cwd();
    const dir = await makeTempDir(root);
    await mkdir(join(dir, '.git'));
    const manifest = '{"name":"local","private":true}\n';
    await writeFile(join(dir, 'package.json'), manifest);
    // Pub without an exact version has no upstream catalog to query.
    const result = await runCLI(
      ['dep', 'install', 'pub', 'archive', '--json'],
      dir,
      root,
    );
    expect(result.code).toBe(9);
    expect(result.stderr).toBe('');
    const report = JSON.parse(result.stdout);
    expect(report.decision).toBe('unknown');
    expect(report.error.code).toBe('INSTALL_UNSUPPORTED');
    expect(report.recommendation).toBeNull();
    expect(await readFile(join(dir, 'package.json'), 'utf8')).toBe(manifest);
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
