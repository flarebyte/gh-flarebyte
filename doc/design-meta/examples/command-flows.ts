export type CommandFlow = {
  name: string;
  command: string;
  repo: string;
  purpose: string;
  syncEffect: string;
};

export const commandFlows: CommandFlow[] = [
  {
    name: "build",
    command: "gh flarebyte build --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Build local artifacts from the top-level build block.",
    syncEffect: "Write versioned binaries and checksums under the configured output directory.",
  },
  {
    name: "release",
    command: "gh flarebyte release --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Build first, then publish a GitHub release from the configured release block.",
    syncEffect: "Run the build flow and upload release assets to GitHub.",
  },
  {
    name: "init",
    command: "gh flarebyte repo init --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Bootstrap repo-local config and seed the GitHub repo settings plus local build and release defaults.",
    syncEffect: "Create or update `.gh-flarebyte.cue` and apply initial GitHub defaults.",
  },
  {
    name: "update",
    command:
      "gh flarebyte repo update --repo my-org/my-repo --confirm-deletions --accept-visibility-change-consequences",
    repo: "my-org/my-repo",
    purpose: "Reconcile GitHub repository state from the local config.",
    syncEffect:
      "Push local desired state to GitHub and fail if deletions or visibility changes are detected without the required safety flags.",
  },
  {
    name: "audit",
    command: "gh flarebyte repo audit --repo my-org/my-repo",
    repo: "my-org/my-repo",
    purpose: "Report drift between the checked-in config and GitHub.",
    syncEffect: "Read-only comparison.",
  },
  {
    name: "repos-mine",
    command: "gh flarebyte repos mine --org my-org",
    repo: "my-org",
    purpose: "Discover repositories the current user contributes to within an organization.",
    syncEffect: "Read-only discovery against GitHub data.",
  },
];
