export type AuditDiff = {
  field: string;
  local: string | boolean | string[];
  remote: string | boolean | string[];
};

export type AuditReport = {
  repo: string;
  driftCount: number;
  hasDrift: boolean;
  diffs: AuditDiff[];
};

export const auditReportExample: AuditReport = {
  repo: "flarebyte/gh-flarebyte",
  driftCount: 2,
  hasDrift: true,
  diffs: [
    {
      field: "repository.homepage",
      local: "https://github.com/flarebyte/gh-flarebyte",
      remote: "",
    },
    {
      field: "repository.topics",
      local: ["gh-extension", "github-cli", "git", "flarebyte"],
      remote: ["gh-extension", "git"],
    },
  ],
};
