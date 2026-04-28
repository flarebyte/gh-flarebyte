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
		"build.language",
		"topics",
		"labels",
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
		build: {
			language: "go"
		}
		buildPlan: {
			outputDir: "build"
			checksumFile: "build/checksums.txt"
			targets: [
				"linux/amd64",
				"darwin/arm64",
			]
		}
		release: {
			versionSource: "main.project.yaml"
			tagPrefix: "v"
			notesMode: "generate-notes"
			artifactDir: "build"
			includeChecksums: true
		}
		topics: [
			"gh-extension",
		"github-cli",
		"git",
		"flarebyte",
	]
	labels: [
		{
			name: "bug"
			color: "B60205"
			description: "Something is broken"
		},
		{
			name: "enhancement"
			color: "0E8A16"
			description: "New feature"
		},
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
