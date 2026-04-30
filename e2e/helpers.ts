import { randomUUID } from "node:crypto";
import { mkdir } from "node:fs/promises";
import { join } from "node:path";

export type RunResult = {
  code: number;
  stdout: string;
  stderr: string;
};

const BLOCKED_PATTERNS = [
  "repo update",
  "release create",
  "release upload",
  "api --method POST",
  "api --method PATCH",
  "api --method PUT",
  "api --method DELETE",
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
  const dir = join(baseDir, "tmp", `e2e-${randomUUID()}`);
  await mkdir(dir, { recursive: true });
  return dir;
}

export async function runCLI(
  args: string[],
  cwd: string,
): Promise<RunResult> {
  const cmdString = `gh flarebyte ${args.join(" ")}`.trim();
  assertReadOnlyCLICommand(cmdString);

  const env = {
    ...process.env,
    GOCACHE: join(cwd, ".gocache"),
  };
  const proc = Bun.spawn(["go", "run", "./cmd/gh-flarebyte", ...args], {
    cwd,
    env,
    stdout: "pipe",
    stderr: "pipe",
  });
  const code = await proc.exited;
  const stdout = await new Response(proc.stdout).text();
  const stderr = await new Response(proc.stderr).text();
  return { code, stdout, stderr };
}
