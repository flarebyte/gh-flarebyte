package ghflarebyte

project: {
	org:  "flarebyte"
	repo: "gh-flarebyte"
}

sync: {
	mode: "push"
}

repository: {
	description:   "CLI for landing your git commands right"
	defaultBranch: "main"
	homepage:      "https://github.com/flarebyte/gh-flarebyte"
	visibility:    "public"
	template:      false
	topics: [
		"gh-extension",
		"github-cli",
		"git",
		"flarebyte",
	]
	labels: [
		{
			name:        "bug"
			color:       "d73a4a"
			description: "Something isn't working"
		},
		{
			name:        "documentation"
			color:       "0075ca"
			description: "Improvements or additions to documentation"
		},
		{
			name:        "duplicate"
			color:       "cfd3d7"
			description: "This issue or pull request already exists"
		},
		{
			name:        "enhancement"
			color:       "a2eeef"
			description: "New feature or request"
		},
		{
			name:        "good first issue"
			color:       "7057ff"
			description: "Good for newcomers"
		},
		{
			name:        "help wanted"
			color:       "008672"
			description: "Extra attention is needed"
		},
		{
			name:        "invalid"
			color:       "e4e669"
			description: "This doesn't seem right"
		},
		{
			name:        "question"
			color:       "d876e3"
			description: "Further information is requested"
		},
		{
			name:        "wontfix"
			color:       "ffffff"
			description: "This will not be worked on"
		},
		{
			name:        "wip"
			color:       "fbca04"
			description: "Work in progress"
		},
	]
	features: {
		issues:                       true
		wiki:                         false
		projects:                     false
		discussions:                  false
		autoMerge:                    true
		mergeCommit:                  false
		rebaseMerge:                  false
		squashMerge:                  true
		squashMergeCommitMessage:     "pr-title"
		deleteBranchOnMerge:          true
		allowForking:                 false
		allowUpdateBranch:            false
		advancedSecurity:             true
		secretScanning:               true
		secretScanningPushProtection: true
	}
}

build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	artifactTargetSuffix: false
	targets: [
		"linux-amd64",
		"darwin-arm64",
	]
}

release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "generate-notes"
	artifactDir:      "build"
	includeChecksums: true
}
