import { describe, expect, test } from "bun:test";
import { cwd } from "node:process";
import { makeTempDir, runCLI } from "./helpers";

describe("gh-flarebyte smoke", () => {
  test("--help returns usage text", async () => {
    const repoRoot = cwd();
    await makeTempDir(repoRoot);
    const result = await runCLI(["--help"], repoRoot);
    expect(result.code).toBe(0);
    expect(result.stderr).toBe("");
    expect(result.stdout).toContain("Usage:");
    expect(result.stdout).toContain("gh flarebyte --version [--json]");
  });

  test("--version --json returns documented shape", async () => {
    const repoRoot = cwd();
    const result = await runCLI(["--version", "--json"], repoRoot);
    expect(result.code).toBe(0);
    expect(result.stderr).toBe("");
    const payload = JSON.parse(result.stdout) as Record<string, unknown>;
    for (const key of [
      "version",
      "commitId",
      "date",
      "os",
      "arch",
      "goVersion",
    ]) {
      expect(payload[key]).toBeDefined();
    }
  });
});
