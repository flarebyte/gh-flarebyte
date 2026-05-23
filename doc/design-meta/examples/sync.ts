export type SyncMode = "push";

export type RepositoryFeatures = {
  issues: boolean;
  wiki: boolean;
  projects: boolean;
  discussions: boolean;
  autoMerge: boolean;
  mergeCommit: boolean;
  rebaseMerge: boolean;
  squashMerge: boolean;
  squashMergeCommitMessage: "default" | "pr-title" | "pr-title-commits" | "pr-title-description";
  deleteBranchOnMerge: boolean;
  allowForking: boolean;
  allowUpdateBranch: boolean;
  advancedSecurity: boolean;
  secretScanning: boolean;
  secretScanningPushProtection: boolean;
};

export type LabelConfig = {
  name: string;
  color: string;
  description: string;
};

export type BuildConfig =
  | {
      language: "go" | "dart";
      mode: "binary";
      outputDir: string;
      checksumFile: string;
      targets: BuildTarget[];
      artifactTargetSuffix?: boolean;
    }
  | {
      language: "go" | "dart";
      mode: "library";
      packages: string[];
      runTests?: boolean;
    };

export type BuildTarget = `${"linux" | "darwin" | "windows"}-${"amd64" | "arm64"}`;

export type GoCGOConfig = {
  enabled: boolean;
  cc?: string;
  cxx?: string;
};

export type GoConfig = {
  cacheDir?: string;
  modCacheDir?: string;
  toolchain?: string;
  cgo?: GoCGOConfig;
};

export type ReleaseConfigBase = {
  versionSource: string;
  tagPrefix: string;
  includeArtifacts: boolean;
  artifactDir?: string;
  includeChecksums?: boolean;
};

export type ReleaseConfig =
  | (ReleaseConfigBase & {
      notesMode: "generate-notes" | "notes-from-tag";
    })
  | (ReleaseConfigBase & {
      notesMode: "notes-file";
      releaseNotesFilePath: string;
    });

export type ProjectConfig = {
  org: string;
  repo: string;
};

export type RepositoryConfig = {
  description: string;
  defaultBranch: string;
  homepage: string;
  visibility: "public" | "private" | "internal";
  template: boolean;
  topics: string[];
  labels: LabelConfig[];
  features: RepositoryFeatures;
};

export type GhFlarebyteConfig = {
  project: ProjectConfig;
  sync: SyncPlan;
  repository: RepositoryConfig;
  go?: GoConfig;
  build: BuildConfig;
  release: ReleaseConfig;
};

export type SyncPlan = {
  mode: SyncMode;
};

export type DriftItem = {
  field: string;
  local: string | boolean | string[];
  remote: string | boolean | string[];
};
