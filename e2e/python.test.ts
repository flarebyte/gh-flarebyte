import { describe, expect, test } from 'bun:test';
import { cp, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { makeTempDir, runCLI } from './helpers';

// Real uv integration is explicit: it may download Python and locked dependencies.
describe.skipIf(process.env.GH_FLAREBYTE_PYTHON_E2E !== '1')(
  'gh-flarebyte Python 3 with uv',
  () => {
    test('developer commands, packaging, failures, and simulated release', async () => {
      const repoRoot = process.cwd();
      const tempDir = await makeTempDir(repoRoot);
      await cp(join(repoRoot, 'e2e/fixtures/python'), tempDir, {
        recursive: true,
      });
      const env = {
        UV_CACHE_DIR: join(repoRoot, '.cache/uv'),
        UV_PYTHON_DOWNLOADS: 'never',
      };
      const run = (args: string[]) => runCLI(args, tempDir, repoRoot, env);
      const lockBefore = await readFile(join(tempDir, 'uv.lock'), 'utf8');

      for (const command of ['format', 'lint', 'test', 'cov']) {
        const result = await run([command]);
        expect(result.code, result.stderr).toBe(0);
        expect(result.stdout).toContain(`${command.toUpperCase()} PASS`);
      }
      const perTest = await run(['test', '--style', 'per_test']);
      expect(perTest.code, perTest.stderr).toBe(0);
      expect(perTest.stdout).toContain('✓ tests.test_math::test_add');
      expect(perTest.stdout).toContain('↷ tests.test_math::test_skipped');
      const quiet = await run(['test', '--failed-only']);
      expect(quiet.code, quiet.stderr).toBe(0);
      expect(quiet.stdout).toBe('');

      const build = await run(['build', '--output-dir', 'output with spaces']);
      expect(build.code, build.stderr).toBe(0);
      const assets = await readdir(join(tempDir, 'output with spaces'));
      expect(assets.some((name) => name.endsWith('.whl'))).toBe(true);
      expect(assets.some((name) => name.endsWith('.tar.gz'))).toBe(true);
      expect(assets).toContain('checksums.txt');

      const release = await runCLI(['release', '--draft'], tempDir, repoRoot, {
        ...env,
        GH_FLAREBYTE_FAKE_RELEASE: '1',
      });
      expect(release.code, release.stderr).toBe(0);
      expect(release.stdout).toContain('Release v1.2.3');

      await writeFile(
        join(tempDir, 'tests/test_failure.py'),
        'def test_failure():\n    assert False\n',
      );
      const failure = await run([
        'test',
        '--style',
        'per_test',
        '--failed-only',
      ]);
      expect(failure.code).toBe(1);
      expect(failure.stdout).toContain('✗ tests.test_failure::test_failure');
      expect(failure.stdout).not.toContain('✓');
      expect(failure.stderr).toContain('assert False');

      await writeFile(
        join(tempDir, 'src/flarebyte_python_fixture/__init__.py'),
        'def add(left, right):\n    return left + right\n\ndef untested():\n    return 42\n',
      );
      // Keep the deliberately failing test outside pytest discovery for coverage checks.
      await writeFile(join(tempDir, 'tests/test_failure.py'), '');
      const lowCoverage = await run(['cov', '--min', '100']);
      expect(lowCoverage.code).toBe(1);
      expect(lowCoverage.stderr).toContain('coverage below minimum');
      expect((await run(['cov', '--min', '50'])).code).toBe(0);

      await writeFile(join(tempDir, 'src/lint_failure.py'), 'import os\n');
      const lintFailure = await run(['lint']);
      expect(lintFailure.code).toBe(1);
      expect(lintFailure.stderr).toContain('F401');
      const target = await run(['build', '--target', 'linux-amd64']);
      expect(target.code).toBe(2);
      expect(target.stderr).toContain('--target is not supported');

      const projectPath = join(tempDir, 'pyproject.toml');
      const project = await readFile(projectPath, 'utf8');
      await writeFile(
        projectPath,
        project.replace('version = "1.2.3"', 'version = "1.2.4"'),
      );
      const stale = await run(['test']);
      expect(stale.code).toBe(1);
      expect(stale.stderr).toContain('lockfile');
      expect(await readFile(join(tempDir, 'uv.lock'), 'utf8')).toBe(lockBefore);

      await mkdir(join(tempDir, 'empty-bin'));
      const missingUV = await runCLI(['test'], tempDir, repoRoot, {
        ...env,
        PATH: join(tempDir, 'empty-bin'),
      });
      expect(missingUV.code).toBe(1);
      expect(missingUV.stderr).toContain('uv');
    }, 120_000);
  },
);
