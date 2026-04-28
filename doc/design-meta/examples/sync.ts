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

export type BuildConfig = {
  language: "go" | "dart";
  outputDir: string;
  checksumFile: string;
  targets: BuildTarget[];
};

export type BuildTarget = `${"linux" | "darwin"}-${"amd64" | "arm64"}`;

export type ReleaseConfig = {
  versionSource: string;
  tagPrefix: string;
  notesMode: "generate-notes" | "notes-file" | "notes-from-tag";
  releaseNotesFilePath?: string;
  artifactDir: string;
  includeChecksums: boolean;
};

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
  build: BuildConfig;
  release: ReleaseConfig;
};

export type SyncPlan = {
  mode: SyncMode;
  managedFields: string[];
  dryRun: boolean;
  visibilityChangeConsequenceAccepted: boolean;
};

export type DriftItem = {
  field: string;
  local: string | boolean | string[];
  remote: string | boolean | string[];
};
