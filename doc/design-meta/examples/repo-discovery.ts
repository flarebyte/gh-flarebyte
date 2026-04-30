export type RepoDiscovery = {
  org: string;
  cursor: string | null;
  pageSize: number;
  scope: "viewer.repositoriesContributedTo";
};

export const repoDiscovery: RepoDiscovery = {
  org: "my-org",
  cursor: null,
  pageSize: 100,
  scope: "viewer.repositoriesContributedTo",
};
