export type ContributedRepo = {
  owner: string;
  name: string;
  visibility: "public" | "private" | "internal";
  defaultBranch: string;
};

export type ReposMineReport = {
  org: string;
  contributor: string;
  count: number;
  repos: ContributedRepo[];
};

export const reposMineReportExample: ReposMineReport = {
  org: "flarebyte",
  contributor: "olivier",
  count: 2,
  repos: [
    {
      owner: "flarebyte",
      name: "gh-flarebyte",
      visibility: "public",
      defaultBranch: "main",
    },
    {
      owner: "flarebyte",
      name: "baldrick-seer",
      visibility: "public",
      defaultBranch: "main",
    },
  ],
};
