export type SyncMode = "push" | "pull" | "bidirectional";

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

export type RepositoryConfig = {
  org: string;
  repo: string;
  description: string;
  defaultBranch: string;
  homepage: string;
  visibility: "public" | "private" | "internal";
  template: boolean;
  topics: string[];
  features: RepositoryFeatures;
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
