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
	]
	labels: [
		{
			name:        "bug"
			color:       "B60205"
			description: "Something is broken"
		},
	]
}

build: {
	language:     "go"
	outputDir:    "build"
	checksumFile: "build/checksums.txt"
	targets: [
		"linux-amd64",
	]
}

release: {
	versionSource:    "main.project.yaml"
	tagPrefix:        "v"
	notesMode:        "notes-file"
	artifactDir:      "build"
	includeChecksums: true
}
