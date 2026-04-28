package ghflarebyte

project: {
	org:  "flarebyte"
	repo: "gh-flarebyte"
}

sync: {
	mode: "bidirectional"
	visibilityChangeConsequenceAccepted: true
	managedFields: [
		"description",
		"defaultBranch",
		"homepage",
		"visibility",
		"template",
		"topics",
		"features.issues",
		"features.wiki",
		"features.projects",
		"features.discussions",
		"features.autoMerge",
		"features.mergeCommit",
		"features.rebaseMerge",
		"features.squashMerge",
		"features.squashMergeCommitMessage",
		"features.deleteBranchOnMerge",
		"features.allowForking",
		"features.allowUpdateBranch",
		"features.advancedSecurity",
		"features.secretScanning",
		"features.secretScanningPushProtection",
	]
}

repository: {
	description: "CLI for landing your git commands right"
	defaultBranch: "main"
	homepage: "https://github.com/flarebyte/gh-flarebyte"
	visibility: "public"
	template: false
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
		squashMergeCommitMessage: "pr-title"
		deleteBranchOnMerge:  true
		allowForking:         false
		allowUpdateBranch:    false
		advancedSecurity:     true
		secretScanning:       true
		secretScanningPushProtection: true
	}
}
