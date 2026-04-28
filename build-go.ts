#!/usr/bin/env bun

// Bun/TypeScript rewrite of build-go.mjs (zx)
// Builds Go binaries for the supported release targets and writes checksums

import crypto from 'node:crypto';
import { promises as fs } from 'node:fs';
import path from 'node:path';
import {
  readVersionFromProjectYAML,
  runChecked,
} from './script/shared/go-release';

async function ensureDir(p: string): Promise<void> {
  await fs.mkdir(p, { recursive: true });
}

function ensureGoEnv(baseEnv: Record<string, string>, cwd: string) {
  if (!baseEnv.GOCACHE) {
    baseEnv.GOCACHE = path.join(cwd, '.gocache');
  }
  if (!baseEnv.GOMODCACHE) {
    baseEnv.GOMODCACHE = path.join(cwd, '.gomodcache');
  }
}

async function runCapture(
  cmd: string[],
  opts: { cwd?: string; env?: Record<string, string | undefined> } = {},
): Promise<string> {
  const proc = Bun.spawn(cmd, {
    cwd: opts.cwd,
    env: opts.env,
    stdout: 'pipe',
    stderr: 'pipe',
  });
  const exitCode = await proc.exited;
  if (exitCode !== 0) {
    return '';
  }
  return (await new Response(proc.stdout).text()).trim();
}

async function sha256File(filePath: string): Promise<string> {
  const hash = crypto.createHash('sha256');
  const file = Bun.file(filePath);
  const stream = file.stream();
  const reader = stream.getReader();
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    if (value) hash.update(value);
  }
  return hash.digest('hex');
}

async function main() {
  const version = (await readVersionFromProjectYAML()).trim();
  if (!version)
    throw new Error('version not found in main.project.yaml (tags.version)');

  const currentDirectory = process.cwd();
  const binaryName = 'seer';
  const folderName = path.basename(currentDirectory);
  const modulePath =
    (await runCapture(['go', 'list', '-m'], { cwd: currentDirectory })) ||
    `github.com/flarebyte/${folderName}`;
  const commitFromGit = await runCapture(
    ['git', 'rev-parse', '--short=12', 'HEAD'],
    { cwd: currentDirectory },
  );
  const commit = process.env.COMMIT || commitFromGit || 'unknown';
  const currentDate =
    process.env.DATE ?? new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
  const ldflags = [
    `-X ${modulePath}/internal/buildinfo.Version=${version}`,
    `-X ${modulePath}/internal/buildinfo.Commit=${commit}`,
    `-X ${modulePath}/internal/buildinfo.Date=${currentDate}`,
  ].join(' ');

  const platforms = [
    { label: 'Linux (amd64)', os: 'linux', arch: 'amd64' },
    { label: 'macOS (Apple Silicon)', os: 'darwin', arch: 'arm64' },
  ] as const;

  await ensureDir('build');
  await ensureDir(path.join(currentDirectory, '.gocache'));
  await ensureDir(path.join(currentDirectory, '.gomodcache'));

  const builtFiles: string[] = [];

  for (const p of platforms) {
    console.log(p.label);
    const env: Record<string, string> = { ...process.env } as Record<
      string,
      string
    >;
    ensureGoEnv(env, currentDirectory);
    env.GOOS = p.os;
    env.GOARCH = p.arch;
    if (p.os === 'darwin') {
      const macArch = p.arch === 'amd64' ? 'x86_64' : 'arm64';
      env.CGO_ENABLED = '1';
      env.CC = 'clang';
      env.CGO_CFLAGS = `-arch ${macArch}`;
      env.CGO_LDFLAGS = `-arch ${macArch}`;
      env.MACOSX_DEPLOYMENT_TARGET = env.MACOSX_DEPLOYMENT_TARGET || '11.0';
    }

    const extension = p.os === 'windows' ? '.exe' : '';
    const out = path.join('build', `${binaryName}-${p.os}-${p.arch}${extension}`);
    await runChecked(
      ['go', 'build', '-o', out, '-ldflags', ldflags, './cmd/seer'],
      {
        env,
      },
    );
    builtFiles.push(out);
  }

  // checksums (sha256), format: "<hex>  <path>" like shasum
  const lines: string[] = [];
  for (const f of builtFiles) {
    const digest = await sha256File(f);
    lines.push(`${digest}  ${f}`);
  }
  await fs.writeFile('build/checksums.txt', `${lines.join('\n')}\n`, 'utf8');
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
