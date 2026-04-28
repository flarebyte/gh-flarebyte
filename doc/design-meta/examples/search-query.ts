export type SearchQuery = {
  org: string;
  topic?: string;
  language?: string;
  archived?: boolean;
};

export type CodeSearchRequest = {
  org: string;
  filename: string;
  includeContent: boolean;
};

export type RepoContentRequest = {
  repo: string;
  path: string;
  ref?: string;
};
