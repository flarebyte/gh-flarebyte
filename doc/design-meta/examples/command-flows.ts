export type CommandFlow = {
  name: string;
  command: string;
  repo: string;
  purpose: string;
  syncEffect: string;
};

export const commandFlows: CommandFlow[] = [
  {
    name: "init",
    command: "gh flarebyte repo init --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Bootstrap repo-local config and seed the GitHub repo settings.",
    syncEffect: "Create or update `.gh-flarebyte.cue` and apply initial defaults.",
  },
  {
    name: "update",
    command: "gh flarebyte repo update --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Reconcile GitHub repository state from the local config.",
    syncEffect: "Push local desired state to GitHub.",
  },
  {
    name: "audit",
    command: "gh flarebyte repo audit --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Report drift between the checked-in config and GitHub.",
    syncEffect: "Read-only comparison.",
  },
];
