import { describe, expect, test } from 'bun:test';
import { createHash } from 'node:crypto';
import {
  cp,
  mkdir,
  readdir,
  readFile,
  rename,
  stat,
  unlink,
  writeFile,
} from 'node:fs/promises';
import { join } from 'node:path';
import { makeTempDir, runCLI } from './helpers';

describe.skipIf(process.env.GH_FLAREBYTE_TYPESCRIPT_E2E !== '1')(
  'gh-flarebyte TypeScript with Bun',
  () => {
    test('library verification, native CLI artifacts, reports, coverage, and failures', async () => {
      const repo = process.cwd();
      const dir = await makeTempDir(repo);
      await cp(join(repo, 'e2e/fixtures/typescript'), dir, { recursive: true });
      // The template is not discovered by the repository's default Bun test run.
      await rename(
        join(dir, 'test/math.fixture.ts'),
        join(dir, 'test/math.test.ts'),
      );
      const run = (args: string[]) => runCLI(args, dir, repo);
      const configPath = join(dir, '.gh-flarebyte.cue');
      const config = await readFile(configPath, 'utf8');
      const lockPath = join(dir, 'bun.lock');
      const lock = await readFile(lockPath, 'utf8');
      const bunfigPath = join(dir, 'bunfig.toml');
      const bunfig = await readFile(bunfigPath, 'utf8');

      for (const command of ['format', 'lint', 'test', 'build']) {
        const result = await run([command]);
        expect(result.code, result.stderr).toBe(0);
      }
      expect(await Bun.file(join(dir, 'build/checksums.txt')).exists()).toBe(
        false,
      );
      const report = await run(['test', '--style', 'per_test']);
      expect(report.code, report.stderr).toBe(0);
      expect(report.stdout).toContain('✓ test/math.test.ts::adds numbers');
      expect(report.stdout).toContain('↷ test/math.test.ts::skipped test');
      expect((await run(['test', '--failed-only'])).stdout).toBe('');
      expect((await run(['build', '--target', 'linux-amd64'])).code).toBe(2);

      const low = await run(['cov', '--min', '100']);
      expect(low.code).toBe(1);
      expect(low.stderr).toContain('coverage below minimum');
      await writeFile(
        configPath,
        config.replace('style: "summary"', 'style: "per_test"'),
      );
      const coverage = await run(['cov', '--min', '0']);
      expect(coverage.code, coverage.stderr).toBe(0);
      expect(coverage.stdout).toContain('src/math.ts');
      expect(coverage.stdout).not.toContain('test/math.test.ts');
      await writeFile(configPath, config);
      expect(await readFile(bunfigPath, 'utf8')).toBe(bunfig);

      const unusedPath = join(dir, 'src/unused.ts');
      await writeFile(unusedPath, 'export function unused() { return 7; }\n');
      const missingCoverage = await run(['cov', '--min', '0']);
      expect(missingCoverage.code).toBe(1);
      expect(missingCoverage.stderr).toContain(
        'Bun coverage omitted production sources: src/unused.ts',
      );
      await writeFile(
        bunfigPath,
        `${bunfig}\ncoveragePathIgnorePatterns = ["src/unused.ts"]\n`,
      );
      const excluded = await run(['cov', '--min', '0']);
      expect(excluded.code, excluded.stderr).toBe(0);
      await unlink(unusedPath);
      await writeFile(bunfigPath, bunfig);

      const libraryRelease = await runCLI(['release', '--draft'], dir, repo, {
        GH_FLAREBYTE_FAKE_RELEASE: '1',
      });
      expect(libraryRelease.code, libraryRelease.stderr).toBe(0);
      expect(libraryRelease.stdout).toContain('v1.2.3');
      expect(libraryRelease.stdout).toContain('without artifacts');

      const os = process.platform === 'darwin' ? 'darwin' : 'linux';
      const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
      const host = `${os}-${arch}`;
      const cliConfig = config
        .replace(
          'mode: "library"',
          `mode: "binary"\n mainPackage: "src/cli.ts"\n targets: ["${host}"]`,
        )
        .replace('includeArtifacts: false', 'includeArtifacts: true');
      await writeFile(configPath, cliConfig);
      const build = await run(['build', '--output-dir', 'output with spaces']);
      expect(build.code, build.stderr).toBe(0);
      const output = join(dir, 'output with spaces');
      const assets = await readdir(output);
      expect(assets).toContain(`typescript-fixture-${host}.tar.gz`);
      expect(assets).toContain(`typescript-fixture-${host}`);
      const checksums = await readFile(join(output, 'checksums.txt'), 'utf8');
      for (const line of checksums.trim().split('\n')) {
        const [hash, name] = line.split('  ');
        expect(
          createHash('sha256')
            .update(await readFile(join(output, name)))
            .digest('hex'),
        ).toBe(hash);
      }
      const binary = join(output, `typescript-fixture-${host}`);
      expect((await stat(binary)).mode & 0o111).not.toBe(0);
      const executable = Bun.spawn([binary], {
        stdout: 'pipe',
        stderr: 'pipe',
      });
      expect(await executable.exited).toBe(0);
      expect((await new Response(executable.stdout).text()).trim()).toBe('5');
      await mkdir(join(dir, 'build'), { recursive: true });
      await writeFile(join(dir, 'build/stale.tar.gz'), 'stale');
      const release = await runCLI(['release', '--draft'], dir, repo, {
        GH_FLAREBYTE_FAKE_RELEASE: '1',
      });
      expect(release.code, release.stderr).toBe(0);
      expect(release.stdout).toContain('Release v1.2.3');
      const crossConfig = cliConfig.replace(`"${host}"`, '"windows-amd64"');
      await writeFile(configPath, crossConfig);
      expect((await run(['build'])).code).toBe(2);
      await writeFile(
        configPath,
        cliConfig.replace(
          'mainPackage: "src/cli.ts"',
          'mainPackage: "src/missing.ts"',
        ),
      );
      expect((await run(['build'])).code).toBe(6);

      await writeFile(configPath, config);
      const testPath = join(dir, 'test/math.test.ts');
      const tests = await readFile(testPath, 'utf8');
      await writeFile(
        testPath,
        `${tests}\ntest('deliberate failure', () => expect(1).toBe(2));\n`,
      );
      const failed = await run([
        'test',
        '--style',
        'per_test',
        '--failed-only',
      ]);
      expect(failed.code).toBe(1);
      expect(failed.stdout).toContain(
        '✗ test/math.test.ts::deliberate failure',
      );
      expect(failed.stdout).not.toContain('✓');
      expect((await run(['build'])).code).toBe(6);
      await writeFile(testPath, tests);

      const sourcePath = join(dir, 'src/math.ts');
      const source = await readFile(sourcePath, 'utf8');
      await writeFile(
        sourcePath,
        source.replace('return a + b;', "return 'type error';"),
      );
      const lint = await run(['lint']);
      expect(lint.code).toBe(1);
      expect(lint.stderr).toContain('not assignable');
      expect(
        (await run(['build', '--output-dir', 'blocked-output'])).code,
      ).toBe(6);
      expect(
        await Bun.file(join(dir, 'blocked-output/checksums.txt')).exists(),
      ).toBe(false);
      await writeFile(
        sourcePath,
        `${source}\nexport function badlyFormatted(){return 1}\n`,
      );
      expect((await run(['lint'])).code).toBe(1);
      expect((await run(['format'])).code).toBe(0);
      expect((await run(['lint'])).code).toBe(0);
      await writeFile(sourcePath, source);

      const manifestPath = join(dir, 'package.json');
      const manifest = await readFile(manifestPath, 'utf8');
      await writeFile(manifestPath, manifest.replace('5.9.3', '5.8.3'));
      expect((await run(['test'])).code).toBe(1);
      expect(await readFile(lockPath, 'utf8')).toBe(lock);
      await writeFile(manifestPath, manifest);
      await unlink(lockPath);
      const missingLock = await run(['test']);
      expect(missingLock.code).toBe(1);
      expect(missingLock.stderr).toContain('bun.lock');
      expect(await Bun.file(lockPath).exists()).toBe(false);
      await writeFile(lockPath, lock);
      await mkdir(join(dir, 'empty-bin'));
      const missingTool = await runCLI(['test'], dir, repo, {
        PATH: join(dir, 'empty-bin'),
      });
      expect(missingTool.code).toBe(1);
      expect(missingTool.stderr).toContain('Bun');
    }, 180_000);
  },
);
