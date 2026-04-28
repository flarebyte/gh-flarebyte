export type GithubProjectConfig = {
    topics: string[];
    profile: string;
}

type ContributionStrategy = 'reasonable' | 'priviedged';
type todo = {
    allowSquashMerging: boolean;
    automaticallyDeleteHeadBranches: boolean;
    //Branch protection rules
    directCommitToMain: boolean;
    dependencyGraph: boolean;
    dependabot: boolean;
    codeScanning: boolean;
    codeQuality: boolean;
    security: boolean;
}