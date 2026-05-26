import { randomUUID } from 'node:crypto';
import { access, mkdir } from 'node:fs/promises';
import { join } from 'node:path';

export type RunResult = {
  code: number;
  stdout: string;
  stderr: string;
};

let buildOnce: Promise<void> | null = null;

const BLOCKED_PATTERNS = [
  'repo update',
  'release create',
  'release upload',
  'api --method POST',
  'api --method PATCH',
  'api --method PUT',
  'api --method DELETE',
];

export function assertReadOnlyCLICommand(command: string): void {
  for (const pattern of BLOCKED_PATTERNS) {
    if (command.includes(pattern)) {
      throw new Error(
        `Refusing to run mutating GitHub command in E2E: ${command}`,
      );
    }
  }
}

export async function makeTempDir(baseDir: string): Promise<string> {
  const dir = join(baseDir, 'tmp', `e2e-${randomUUID()}`);
  await mkdir(dir, { recursive: true });
  return dir;
}

export async function runCLI(
  args: string[],
  cwd: string,
  repoRoot = cwd,
  extraEnv: Record<string, string> = {},
): Promise<RunResult> {
  const cmdString = `gh flarebyte ${args.join(' ')}`.trim();
  assertReadOnlyCLICommand(cmdString);

  await ensureBuiltBinary(repoRoot);
  const cmd = [join(repoRoot, '.e2e-bin', 'gh-flarebyte'), ...args];

  const env = {
    ...process.env,
    GOCACHE: join(cwd, '.gocache'),
    GH_FLAREBYTE_FAKE_READONLY: '1',
    ...extraEnv,
  };
  const proc = Bun.spawn(cmd, {
    cwd,
    env,
    stdout: 'pipe',
    stderr: 'pipe',
  });
  const code = await proc.exited;
  const stdout = await new Response(proc.stdout).text();
  const stderr = await new Response(proc.stderr).text();
  return { code, stdout, stderr };
}

async function ensureBuiltBinary(repoRoot: string): Promise<void> {
  if (buildOnce) {
    return buildOnce;
  }
  buildOnce = (async () => {
    const outDir = join(repoRoot, '.e2e-bin');
    const binaryPath = join(outDir, 'gh-flarebyte');
    try {
      await access(binaryPath);
      return;
    } catch {
      // Build once when no prebuilt binary exists.
    }
    await mkdir(outDir, { recursive: true });
    const proc = Bun.spawn(
      ['go', 'build', '-o', binaryPath, './cmd/gh-flarebyte'],
      {
        cwd: repoRoot,
        env: {
          ...process.env,
          GOCACHE: join(repoRoot, '.gocache'),
          GOMODCACHE: join(repoRoot, '.gomodcache'),
        },
        stdout: 'pipe',
        stderr: 'pipe',
      },
    );
    const code = await proc.exited;
    if (code !== 0) {
      const stderr = await new Response(proc.stderr).text();
      throw new Error(`failed to build e2e binary: ${stderr}`);
    }
  })();
  return buildOnce;
}
