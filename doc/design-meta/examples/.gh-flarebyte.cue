package ghflarebyte

project: {
	org:  "flarebyte"
	repo: "gh-flarebyte"
}

sync: {
	mode: "bidirectional"
	managedFields: [
		"description",
		"defaultBranch",
		"topics",
		"features.issues",
		"features.wiki",
		"features.projects",
		"features.discussions",
		"features.autoMerge",
		"features.mergeCommit",
		"features.rebaseMerge",
		"features.squashMerge",
		"features.deleteBranchOnMerge",
	]
}

repository: {
	description: "CLI for landing your git commands right"
	defaultBranch: "main"
	topics: [
		"gh-extension",
		"github-cli",
		"git",
		"flarebyte",
	]
	features: {
		issues:               true
		wiki:                 false
		projects:             false
		discussions:          false
		autoMerge:            true
		mergeCommit:          false
		rebaseMerge:          false
		squashMerge:          true
		deleteBranchOnMerge:  true
		allowForking:         false
	}
}
