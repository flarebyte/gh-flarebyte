import { describe, expect, test } from 'bun:test';
import { cp, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { makeTempDir, runCLI } from './helpers';

describe.skipIf(process.env.GH_FLAREBYTE_SWIFT_E2E !== '1')(
  'gh-flarebyte SwiftPM',
  () => {
    test('resolved source-control dependencies fail without changing a stale lockfile', async () => {
      const repoRoot = process.cwd();
      const dir = await makeTempDir(repoRoot);
      const dependency = await makeTempDir(repoRoot);
      await cp(join(repoRoot, 'e2e/fixtures/swift'), dir, { recursive: true });
      await mkdir(join(dependency, 'Sources/Dependency'), { recursive: true });
      await writeFile(
        join(dependency, 'Package.swift'),
        `// swift-tools-version: 6.0
import PackageDescription
let package = Package(name: "Dependency", products: [.library(name: "Dependency", targets: ["Dependency"])], targets: [.target(name: "Dependency")])
`,
      );
      await writeFile(
        join(dependency, 'Sources/Dependency/Dependency.swift'),
        'public let value = 1\n',
      );
      const command = async (argv: string[], cwd: string) => {
        const proc = Bun.spawn(argv, { cwd, stdout: 'pipe', stderr: 'pipe' });
        const output = Promise.all([
          new Response(proc.stdout).text(),
          new Response(proc.stderr).text(),
        ]);
        const code = await proc.exited;
        const [stdout, stderr] = await output;
        expect(code, `${stdout}\n${stderr}`).toBe(0);
      };
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
        ['tag', '1.0.0'],
        ['tag', '1.1.0'],
      ])
        await command(['git', ...args], dependency);
      const manifestPath = join(dir, 'Package.swift');
      const manifest = (await readFile(manifestPath, 'utf8'))
        .replace(
          'targets: [\n    .target',
          `dependencies: [.package(url: "file://${dependency}", exact: "1.0.0")],\n  targets: [\n    .target`,
        )
        .replace(
          'dependencies: ["FixtureMath"]',
          `dependencies: ["FixtureMath", .product(name: "Dependency", package: "${dependency.split('/').at(-1)}")]`,
        );
      await writeFile(manifestPath, manifest);
      await command(['swift', 'package', 'resolve'], dir);
      const lockPath = join(dir, 'Package.resolved');
      const lock = await readFile(lockPath, 'utf8');
      const passing = await runCLI(['test'], dir, repoRoot);
      expect(passing.code, passing.stderr).toBe(0);
      expect(await readFile(lockPath, 'utf8')).toBe(lock);
      await writeFile(
        manifestPath,
        manifest.replace('exact: "1.0.0"', 'exact: "1.1.0"'),
      );
      const stale = await runCLI(['test'], dir, repoRoot);
      expect(stale.code).toBe(1);
      expect(stale.stderr).toContain('resolved');
      expect(await readFile(lockPath, 'utf8')).toBe(lock);
    }, 120_000);

    test('developer commands, both test frameworks, builds, failures, and simulated releases', async () => {
      const repoRoot = process.cwd();
      const dir = await makeTempDir(repoRoot);
      await cp(join(repoRoot, 'e2e/fixtures/swift'), dir, { recursive: true });
      const run = (args: string[]) => runCLI(args, dir, repoRoot);
      const configPath = join(dir, '.gh-flarebyte.cue');
      const config = await readFile(configPath, 'utf8');
      for (const command of ['format', 'lint', 'test', 'build']) {
        const result = await run([command]);
        expect(result.code, result.stderr).toBe(0);
      }
      const report = await run(['test', '--style', 'per_test']);
      expect(report.code, report.stderr).toBe(0);
      expect(report.stdout).toContain('testAddition');
      expect(report.stdout).toContain('modernAddition');
      expect(report.stdout).toContain('↷');
      expect(report.stdout).toContain('modernSkipped');
      expect(report.stdout).toContain('testSkipped');
      expect(report.stdout).toContain('tests=4');
      expect((await run(['test', '--failed-only'])).stdout).toBe('');
      const cov = await run(['cov', '--min', '100']);
      expect(cov.code).toBe(1);
      expect(cov.stderr).toContain('coverage below minimum');
      await writeFile(
        configPath,
        config.replace('style: "summary"', 'style: "per_test"'),
      );
      const passingCov = await run(['cov', '--min', '0']);
      expect(passingCov.code, passingCov.stderr).toBe(0);
      expect(passingCov.stdout).toContain('Sources/FixtureMath/Math.swift');
      expect(passingCov.stdout).not.toContain('Tests/');
      await writeFile(configPath, config);
      const libraryRelease = await runCLI(
        ['release', '--draft'],
        dir,
        repoRoot,
        { GH_FLAREBYTE_FAKE_RELEASE: '1' },
      );
      expect(libraryRelease.code, libraryRelease.stderr).toBe(0);
      expect(libraryRelease.stdout).toContain('v1.2.3');
      expect(libraryRelease.stdout).toContain('without artifacts');
      const os = process.platform === 'darwin' ? 'darwin' : 'linux';
      const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
      const target = `${os}-${arch}`;
      const binaryConfig = config
        .replace('mode: "library"', `mode: "binary"\n targets: ["${target}"]`)
        .replace('includeArtifacts: false', 'includeArtifacts: true');
      await writeFile(configPath, binaryConfig);
      const build = await run(['build', '--output-dir', 'output with spaces']);
      expect(build.code, build.stderr).toBe(0);
      expect(await readdir(join(dir, 'output with spaces'))).toContain(
        'checksums.txt',
      );
      const executable = Bun.spawn(
        [join(dir, 'output with spaces', `swift-fixture-${target}`)],
        { stdout: 'pipe', stderr: 'pipe' },
      );
      expect(await executable.exited).toBe(0);
      expect((await new Response(executable.stdout).text()).trim()).toBe('5');
      const cross = await run(['build', '--target', 'windows-amd64']);
      expect(cross.code).toBe(2);
      await mkdir(join(dir, 'build'), { recursive: true });
      await writeFile(join(dir, 'build', 'stale.tar.gz'), 'stale');
      const release = await runCLI(['release', '--draft'], dir, repoRoot, {
        GH_FLAREBYTE_FAKE_RELEASE: '1',
      });
      expect(release.code, release.stderr).toBe(0);
      expect(release.stdout).not.toContain('stale.tar.gz');
      await writeFile(configPath, config);
      const testsPath = join(dir, 'Tests/FixtureMathTests/MathTests.swift');
      const tests = await readFile(testsPath, 'utf8');
      await writeFile(
        testsPath,
        `${tests}\n@Test func deliberateFailure() { #expect(1 == 2) }\n`,
      );
      const failing = await run([
        'test',
        '--style',
        'per_test',
        '--failed-only',
      ]);
      expect(failing.code).toBe(1);
      expect(failing.stdout).toContain('✗');
      expect(failing.stdout).toContain('deliberateFailure');
      expect(failing.stdout).not.toContain('✓');
      expect(failing.stderr).toContain('Expectation failed');
      expect((await run(['build'])).code).toBe(6);
      await writeFile(
        testsPath,
        tests.replace(
          'XCTAssertEqual(FixtureMath.add(2, 3), 5)',
          'XCTAssertEqual(FixtureMath.add(2, 3), 6)',
        ),
      );
      const xcFailure = await run([
        'test',
        '--style',
        'per_test',
        '--failed-only',
      ]);
      expect(xcFailure.code).toBe(1);
      expect(xcFailure.stdout).toContain('testAddition');
      expect(xcFailure.stdout).toContain('✗');
      await writeFile(testsPath, tests);
      const sourcePath = join(dir, 'Sources/FixtureMath/Math.swift');
      const source = await readFile(sourcePath, 'utf8');
      await writeFile(
        sourcePath,
        `${source}\npublic func badlyFormatted()->Int{return 1}\n`,
      );
      expect((await run(['lint'])).code).toBe(1);
      expect((await run(['format'])).code).toBe(0);
      expect((await run(['lint'])).code).toBe(0);
      await writeFile(
        sourcePath,
        `${source}\n#warning("fixture compiler warning")\n`,
      );
      expect((await run(['format'])).code).toBe(0);
      const compilerWarning = await run(['lint']);
      expect(compilerWarning.code).toBe(1);
      expect(compilerWarning.stderr).toContain('fixture compiler warning');
      await writeFile(sourcePath, 'this is not Swift');
      expect((await run(['test'])).code).toBe(1);
      await writeFile(sourcePath, source);
      const manifestPath = join(dir, 'Package.swift');
      const manifest = await readFile(manifestPath, 'utf8');
      await writeFile(
        manifestPath,
        manifest.replace(
          'targets: [\n    .target',
          'dependencies: [.package(url: "https://example.invalid/dependency.git", from: "1.0.0")],\n  targets: [\n    .target',
        ),
      );
      const missingLock = await run(['test']);
      expect(missingLock.code).toBe(1);
      expect(missingLock.stderr).toContain('Package.resolved');
      await writeFile(manifestPath, manifest);
      await mkdir(join(dir, 'empty-bin'));
      const missing = await runCLI(['test'], dir, repoRoot, {
        PATH: join(dir, 'empty-bin'),
      });
      expect(missing.code).toBe(1);
      expect(missing.stderr).toContain('swift');
    }, 240_000);
  },
);
