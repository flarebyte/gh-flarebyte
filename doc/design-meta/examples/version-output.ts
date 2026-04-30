export type VersionInfo = {
  version: string;
  commitId: string;
  date: string;
  os: string;
  arch: string;
  goVersion: string;
};

export const versionExample: VersionInfo = {
  version: "v1.2.3",
  commitId: "a1b2c3d4",
  date: "2026-04-30T09:15:00Z",
  os: "darwin",
  arch: "arm64",
  goVersion: "go1.25.0",
};
