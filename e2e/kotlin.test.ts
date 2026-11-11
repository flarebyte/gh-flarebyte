import { describe, expect, test } from 'bun:test';
import { createHash } from 'node:crypto';
import {
  cp,
  mkdir,
  readdir,
  readFile,
  rm,
  unlink,
  writeFile,
} from 'node:fs/promises';
import { join } from 'node:path';
import { makeTempDir, runCLI } from './helpers';

describe.skipIf(process.env.GH_FLAREBYTE_KOTLIN_E2E !== '1')(
  'gh-flarebyte Kotlin/JVM Gradle library and CLI',
  () => {
    test('shared commands, strict reports, JVM distribution and release policy', async () => {
      const repo = process.cwd();
      const dir = await makeTempDir(repo);
      try {
        await cp(join(repo, 'e2e/fixtures/kotlin'), dir, {
          recursive: true,
          filter: (path) =>
            !['.gradle', 'build', '.kotlin'].some((name) =>
              path.split('/').includes(name),
            ),
        });
        const run = (args: string[]) => runCLI(args, dir, repo);
        const configPath = join(dir, '.gh-flarebyte.cue');
        const config = await readFile(configPath, 'utf8');
        const lock = await readFile(join(dir, 'gradle.lockfile'), 'utf8');
        for (const command of ['config', 'format', 'lint', 'test', 'build']) {
          const result = await run(
            command === 'config' ? ['config', 'validate'] : [command],
          );
          expect(result.code, result.stderr).toBe(0);
        }
        expect(await Bun.file(join(dir, 'dist/checksums.txt')).exists()).toBe(
          false,
        );
        const perTest = await run(['test', '--style', 'per_test']);
        expect(perTest.code, perTest.stderr).toBe(0);
        expect(perTest.stdout).toContain('✓ fixture.MathTest::adds()');
        expect(perTest.stdout).toContain('↷ fixture.MathTest::skipped()');
        expect((await run(['test', '--failed-only'])).stdout).toBe('');
        const cov = await run(['cov', '--min', '0']);
        expect(cov.code, cov.stderr).toBe(0);
        const low = await run(['cov', '--min', '100']);
        expect(low.code, low.stderr).toBe(1);
        expect(low.stderr).toContain('coverage below minimum');
        expect((await run(['build', '--target', 'linux-amd64'])).code).toBe(2);
        const libraryRelease = await runCLI(['release', '--draft'], dir, repo, {
          GH_FLAREBYTE_FAKE_RELEASE: '1',
        });
        expect(libraryRelease.code, libraryRelease.stderr).toBe(0);
        expect(libraryRelease.stdout).toContain('without artifacts');

        const gradlePath = join(dir, 'build.gradle.kts');
        const libraryGradle = await readFile(gradlePath, 'utf8');
        await writeFile(
          gradlePath,
          libraryGradle.replace('plugins {', 'plugins {\n    application') +
            '\napplication { mainClass.set("fixture.MainKt") }\n',
        );
        const cliConfig = config
          .replace('mode: "library"', 'mode: "binary"')
          .replace('includeArtifacts: false', 'includeArtifacts: true');
        await writeFile(configPath, cliConfig);
        const build = await run([
          'build',
          '--output-dir',
          'output with spaces',
        ]);
        expect(build.code, build.stderr).toBe(0);
        const output = join(dir, 'output with spaces');
        expect((await readdir(output)).sort()).toEqual([
          'checksums.txt',
          'kotlin-fixture-1.2.3.zip',
        ]);
        const archive = join(output, 'kotlin-fixture-1.2.3.zip');
        const sum = createHash('sha256')
          .update(await readFile(archive))
          .digest('hex');
        expect(await readFile(join(output, 'checksums.txt'), 'utf8')).toBe(
          `${sum}  kotlin-fixture-1.2.3.zip\n`,
        );
        const extracted = join(dir, 'unpacked');
        await mkdir(extracted);
        const unzip = Bun.spawn(['unzip', '-q', archive, '-d', extracted], {
          stdout: 'pipe',
          stderr: 'pipe',
        });
        expect(await unzip.exited).toBe(0);
        const [root] = await readdir(extracted);
        const cli = Bun.spawn([join(extracted, root, 'bin/kotlin-fixture')], {
          env: process.env,
          stdout: 'pipe',
          stderr: 'pipe',
        });
        expect(await cli.exited).toBe(0);
        expect(await new Response(cli.stdout).text()).toBe('sum=5\n');
        await mkdir(join(dir, 'dist'), { recursive: true });
        await writeFile(join(dir, 'dist/stale.zip'), 'stale');
        const release = await runCLI(['release', '--draft'], dir, repo, {
          GH_FLAREBYTE_FAKE_RELEASE: '1',
        });
        expect(release.code, release.stderr).toBe(0);
        expect(release.stdout).toContain('v1.2.3');
        expect(await readFile(join(dir, 'gradle.lockfile'), 'utf8')).toBe(lock);

        const source = join(dir, 'src/test/kotlin/fixture/MathTest.kt');
        const original = await readFile(source, 'utf8');
        await writeFile(
          source,
          original.replace('assertEquals(5,', 'assertEquals(99,'),
        );
        const failed = await run([
          'test',
          '--style',
          'per_test',
          '--failed-only',
        ]);
        expect(failed.code).toBe(1);
        expect(failed.stdout).toContain('✗ fixture.MathTest::adds()');
        await writeFile(source, original);
        await unlink(join(dir, 'gradle.lockfile'));
        const missing = await run(['test']);
        expect(missing.code).toBe(1);
        expect(missing.stderr).toContain('gradle.lockfile');
      } finally {
        await rm(dir, { recursive: true, force: true });
      }
    }, 1_500_000);
  },
);
