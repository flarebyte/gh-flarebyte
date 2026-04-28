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
  deleteBranchOnMerge: boolean;
  allowForking: boolean;
};

export type RepositoryConfig = {
  org: string;
  repo: string;
  description: string;
  defaultBranch: string;
  topics: string[];
  features: RepositoryFeatures;
};

export type SyncPlan = {
  mode: SyncMode;
  managedFields: string[];
  dryRun: boolean;
};

export type DriftItem = {
  field: string;
  local: string | boolean | string[];
  remote: string | boolean | string[];
};
