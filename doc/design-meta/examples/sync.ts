export type SyncMode = "push";

// Only mergeCommit, rebaseMerge, squashMerge, and deleteBranchOnMerge are enforced.
export type RepositoryFeatures = {
  issues?: boolean;
  wiki?: boolean;
  projects?: boolean;
  discussions?: boolean;
  autoMerge?: boolean;
  mergeCommit?: boolean;
  rebaseMerge?: boolean;
  squashMerge?: boolean;
  squashMergeCommitMessage?: "default" | "pr-title" | "pr-title-commits" | "pr-title-description";
  deleteBranchOnMerge?: boolean;
  allowForking?: boolean;
  allowUpdateBranch?: boolean;
  advancedSecurity?: boolean;
  secretScanning?: boolean;
  secretScanningPushProtection?: boolean;
};

export type LabelConfig = {
  name: string;
  color: string;
  description: string;
};

// Shape examples; language-specific selector and target restrictions are checked at runtime.
export type BuildConfig =
  | {
      language: "go" | "rust" | "swift" | "typescript";
      mode?: "binary"; // The default mode.
      mainPackage?: string; // Required for TypeScript; other languages have selection defaults.
      outputDir: string;
      checksumFile: string;
      targets: BuildTarget[];
      artifactTargetSuffix?: boolean;
      runTests?: boolean;
    }
  | {
      language: "go" | "dart" | "rust" | "swift" | "typescript" | "kotlin";
      mode: "library";
      packages?: string[]; // Defaults to ["./..."]; TS/Kotlin accept only that root selection.
      runTests?: boolean;
    }
  | {
      language: "python" | "rust";
      mode: "package";
      packages?: string[];
      outputDir: string;
      checksumFile: string;
      runTests?: boolean;
    }
  | {
      language: "kotlin";
      mode?: "binary";
      outputDir: string;
      checksumFile: string;
      runTests?: boolean;
      // Gradle selects the entry point. Platform targets and target suffixes are rejected.
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
  includeArtifacts?: boolean; // Defaults to true; libraries require false.
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
  features?: RepositoryFeatures;
};

export type ToolingConfig = {
  runtime: "bun" | "python3";
  root?: string; // Defaults to e2e or script; repository-relative.
  dependencies?: {
    manager?: "bun" | "npm" | "uv";
    root?: string; // Defaults to the repository root (.).
  };
};

export type GhFlarebyteConfig = {
  project: ProjectConfig;
  sync: SyncPlan;
  repository: RepositoryConfig;
  go?: GoConfig;
  e2e?: ToolingConfig;
  script?: ToolingConfig;
  devOutput?: {
    color?: "auto" | "true" | "false";
    style?: "summary" | "per_test";
    showPassed?: boolean;
  };
  coverage?: { min?: number; enforceMin?: boolean };
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
