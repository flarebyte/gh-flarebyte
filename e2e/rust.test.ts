import { describe, expect, test } from 'bun:test';
import { cp, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { makeTempDir, runCLI } from './helpers';

describe.skipIf(process.env.GH_FLAREBYTE_RUST_E2E !== '1')(
  'gh-flarebyte Rust with Cargo',
  () => {
    test('workspace developer commands, all build modes, and simulated releases', async () => {
      const repoRoot = process.cwd();
      const tempDir = await makeTempDir(repoRoot);
      await cp(join(repoRoot, 'e2e/fixtures/rust'), tempDir, {
        recursive: true,
      });
      // Cargo verifies that package sources are committed; use an isolated fixture repository.
      for (const args of [
        ['init', '--quiet'],
        ['add', '.'],
        [
          '-c',
          'user.name=Fixture',
          '-c',
          'user.email=fixture@example.invalid',
          'commit',
          '--quiet',
          '-m',
          'Fixture',
        ],
      ]) {
        const proc = Bun.spawn(['git', ...args], {
          cwd: tempDir,
          stdout: 'pipe',
          stderr: 'pipe',
        });
        expect(await proc.exited, await new Response(proc.stderr).text()).toBe(
          0,
        );
      }
      const run = (args: string[]) => runCLI(args, tempDir, repoRoot);
      const configPath = join(tempDir, '.gh-flarebyte.cue');
      const config = await readFile(configPath, 'utf8');
      const lockPath = join(tempDir, 'Cargo.lock');
      const lock = await readFile(lockPath, 'utf8');

      for (const command of ['format', 'lint', 'test', 'build']) {
        const result = await run([command]);
        expect(result.code, result.stderr).toBe(0);
      }
      const tests = await run(['test', '--style', 'per_test']);
      expect(tests.code, tests.stderr).toBe(0);
      expect(tests.stdout).toContain('✓ tests::adds_numbers');
      expect(tests.stdout).toContain('↷ tests::ignored_test');
      expect(tests.stdout).toContain('✓ adds_negative_numbers');
      expect(tests.stdout).toContain('math/src/lib.rs');
      expect((await run(['test', '--failed-only'])).stdout).toBe('');
      const low = await run(['cov', '--min', '100']);
      expect(low.code).toBe(1);
      expect(low.stderr).toContain('coverage below minimum');
      await writeFile(
        configPath,
        config.replace('style: "summary"', 'style: "per_test"'),
      );
      const covPass = await run(['cov', '--min', '0']);
      expect(covPass.code, covPass.stderr).toBe(0);
      expect(covPass.stdout).toContain('COV PASS');
      expect(covPass.stdout).toContain('math/src/lib.rs');
      await writeFile(configPath, config);

      const libraryRelease = await runCLI(
        ['release', '--draft'],
        tempDir,
        repoRoot,
        { GH_FLAREBYTE_FAKE_RELEASE: '1' },
      );
      expect(libraryRelease.code, libraryRelease.stderr).toBe(0);
      expect(libraryRelease.stdout).toContain('v1.2.3');
      expect(libraryRelease.stdout).toContain('without artifacts');

      const packageConfig = config
        .replace('mode: "library"', 'mode: "package"')
        .replace('includeArtifacts: false', 'includeArtifacts: true');
      await writeFile(configPath, packageConfig);
      const packages = await run([
        'build',
        '--output-dir',
        'output with spaces',
      ]);
      expect(packages.code, packages.stderr).toBe(0);
      expect(
        (await readdir(join(tempDir, 'output with spaces'))).sort(),
      ).toEqual([
        'checksums.txt',
        'flarebyte-rust-app-1.2.3.crate',
        'flarebyte-rust-math-1.2.3.crate',
      ]);
      const packageRelease = await runCLI(
        ['release', '--draft'],
        tempDir,
        repoRoot,
        { GH_FLAREBYTE_FAKE_RELEASE: '1' },
      );
      expect(packageRelease.code, packageRelease.stderr).toBe(0);
      expect((await run(['build', '--target', 'linux-amd64'])).code).toBe(2);

      const hostOS = { darwin: 'darwin', linux: 'linux', win32: 'windows' }[
        process.platform
      ];
      const hostArch = { arm64: 'arm64', x64: 'amd64' }[process.arch];
      expect(hostOS).toBeDefined();
      expect(hostArch).toBeDefined();
      const target = `${hostOS}-${hostArch}`;
      await writeFile(
        configPath,
        packageConfig.replace(
          'mode: "package"',
          `mode: "binary"\n  targets: ["${target}"]`,
        ),
      );
      const binary = await run(['build']);
      expect(binary.code, binary.stderr).toBe(0);
      const binaryAssets = await readdir(join(tempDir, 'build'));
      const extension = hostOS === 'windows' ? '.exe' : '';
      expect(binaryAssets).toContain(`rust-fixture-${target}${extension}`);
      expect(binaryAssets).toContain(
        `rust-fixture-${target}${hostOS === 'windows' ? '.zip' : '.tar.gz'}`,
      );
      const executable = Bun.spawn(
        [join(tempDir, 'build', `rust-fixture-${target}${extension}`)],
        { stdout: 'pipe', stderr: 'pipe' },
      );
      expect(await executable.exited).toBe(0);
      expect((await new Response(executable.stdout).text()).trim()).toBe('5');
      const binaryRelease = await runCLI(
        ['release', '--draft'],
        tempDir,
        repoRoot,
        { GH_FLAREBYTE_FAKE_RELEASE: '1' },
      );
      expect(binaryRelease.code, binaryRelease.stderr).toBe(0);

      await writeFile(configPath, config);
      const sourcePath = join(tempDir, 'math/src/lib.rs');
      const source = await readFile(sourcePath, 'utf8');
      await writeFile(
        sourcePath,
        `${source}\n#[test]\nfn deliberate_failure() { assert_eq!(1, 2); }\n`,
      );
      const failure = await run([
        'test',
        '--style',
        'per_test',
        '--failed-only',
      ]);
      expect(failure.code).toBe(1);
      expect(failure.stdout).toContain('✗ deliberate_failure');
      expect(failure.stdout).not.toContain('✓');
      expect(failure.stderr).toContain('assertion');
      expect((await run(['build'])).code).toBe(6);
      await writeFile(sourcePath, `${source}\nfn unused_function() {}\n`);
      const lint = await run(['lint']);
      expect(lint.code).toBe(1);
      expect(lint.stderr).toContain('unused_function');
      await writeFile(sourcePath, 'this is not Rust');
      expect((await run(['test'])).code).toBe(1);
      await writeFile(sourcePath, source);

      await writeFile(
        join(tempDir, 'math/Cargo.toml'),
        (await readFile(join(tempDir, 'math/Cargo.toml'), 'utf8')).replace(
          'version.workspace = true',
          'version = "1.2.4"',
        ),
      );
      const stale = await run(['test']);
      expect(stale.code).toBe(1);
      expect(stale.stderr).toContain('lock');
      expect(await readFile(lockPath, 'utf8')).toBe(lock);
      await mkdir(join(tempDir, 'empty-bin'));
      const missing = await runCLI(['test'], tempDir, repoRoot, {
        PATH: join(tempDir, 'empty-bin'),
      });
      expect(missing.code).toBe(1);
      expect(missing.stderr).toContain('cargo');
    }, 180_000);
  },
);
