package flyb

source: "gh-flarebyte"
name:   "gh-flarebyte"
modules: ["core"]

reports: [{
	title:       "gh-flarebyte Design Notes"
	filepath:    "../design/gh-flarebyte.md"
	description: "Repository sync model for the flarebyte GitHub CLI extension."
	sections: [{
		title:       "01 Overview"
		description: "What the extension manages and why the repo-local CUE file exists."
		sections: [{
			title:       "01 Intent"
			description: "Why the checked-in config file exists."
			notes: [
				"project.summary",
			]
		}, {
			title:       "02 Scope"
			description: "The operational boundary for the extension."
			notes: [
				"project.scope",
			]
		}]
	}, {
		title:       "02 Configuration"
		description: "Canonical repo configuration and the field map used for sync."
		sections: [{
			title:       "01 Config Example"
			description: "The repo-local `.gh-flarebyte.cue` file."
			notes: [
				"repo.config.example",
			]
		}, {
			title:       "02 Field Map"
			description: "How config fields map onto GitHub sync targets and extension-local automation settings."
			notes: [
				"repo.config.fields",
			]
		}, {
			title:       "03 Topics"
			description: "Repository topics are a flat sync target in the cue config."
			notes: [
				"repo.topics.config",
			]
		}, {
			title:       "04 Labels"
			description: "Repository labels have their own structured sync shape in the cue config."
			notes: [
				"repo.labels.config",
			]
		}, {
			title:       "05 Build"
			description: "Build language selection for gh flarebyte build."
			notes: [
				"repo.build.config",
			]
		}, {
			title:       "06 Release"
			description: "Release publication settings for gh flarebyte release."
			notes: [
				"repo.release.config",
			]
		}, {
			title:       "07 Sync Types"
			description: "TypeScript shapes for the sync contract."
			notes: [
				"sync.types",
			]
		}, {
			title:       "08 Config Coverage"
			description: "Additional gh repo edit settings that are now modeled in the cue sync config."
			notes: [
				"repo.config.coverage",
			]
		}]
	}, {
		title:       "03 Commands"
		description: "User-facing extension commands and the config-driven sync path."
		sections: [{
			title:       "01 Command Matrix"
			description: "User-facing extension actions and their purpose."
			notes: [
				"command.matrix",
			]
		}, {
			title:       "02 Command Flows"
			description: "TypeScript examples that show the intended command sequences."
			notes: [
				"command.flows",
			]
		}, {
			title:       "03 Build"
			description: "How build orchestration is driven from config."
			notes: [
				"command.build",
			]
		}, {
			title:       "04 Release"
			description: "How release publication is driven from config."
			notes: [
				"command.release",
			]
		}, {
			title:       "05 Init"
			description: "What repo bootstrap does."
			notes: [
				"command.init",
			]
		}, {
			title:       "06 Update"
			description: "What reconciliation from cue config means."
			notes: [
				"command.update",
			]
		}, {
			title:       "07 Audit"
			description: "What read-only drift checking means."
			notes: [
				"command.audit",
			]
		}, {
			title:       "08 Repos Mine"
			description: "What repository discovery returns."
			notes: [
				"command.repos.mine",
			]
		}, {
			title:       "09 GitHub Flags"
			description: "The existing `gh repo edit` knobs that `gh flarebyte repo update` applies from config."
			notes: [
				"gh.repo.edit.flags",
			]
		}]
	}, {
		title:       "04 Existing GitHub Behaviour"
		description: "GitHub behaviors and command shapes that are examples of the external system, not new extension commands."
		sections: [{
			title:       "01 Labels"
			description: "Label lifecycle and bulk label conventions that the extension synchronizes from cue config."
			notes: [
				"label.matrix",
			]
		}, {
			title:       "02 Releases"
			description: "Release creation and publication flows that remain existing gh behavior."
			notes: [
				"release.matrix",
			]
		}, {
			title:       "03 Search"
			description: "Search and content discovery shapes that are examples of current GitHub usage."
			notes: [
				"search.query",
			]
		}]
	}, {
		title:       "05 Discovery"
		description: "Repo discovery for the 'repos mine' workflow, shown as an existing GitHub capability."
		sections: [{
			title:       "01 Repos Mine"
			description: "How the extension discovers repositories the user contributes to by leaning on existing GitHub data."
			notes: [
				"repo.discovery",
			]
		}]
	}, {
		title:       "06 Open Questions"
		description: "Pending decisions that should stay visible in the spec."
		sections: [{
			title:       "01 Decisions Pending"
			description: "What still needs agreement before the sync contract hardens."
			notes: [
				"project.open-questions",
			]
		}]
	}]
}]

notes: [
	{
		name:     "project.summary"
		title:    "Project Summary"
		markdown: "Flarebyte's `gh` extension manages GitHub repository state from a checked-in `.gh-flarebyte.cue` file so repo metadata, topics, labels, and repo settings can be synchronized deterministically. The same file also carries top-level `build` and `release` blocks that drive local extension commands. `gh flarebyte repo update` applies the config-driven `gh repo edit` changes."
		labels:   ["overview", "summary", "sync"]
	},
	{
		name:     "project.scope"
		title:    "Project Scope"
		markdown: "The extension is centered on repo bootstrap, reconciliation, audit, repository discovery, build, and release. Topics are flat strings, labels are structured objects, repo-edit mutations are applied by `gh flarebyte repo update` rather than by manually repeating `gh repo edit` flags, and build and release automation remain extension-local configuration rather than GitHub repository state."
		labels:   ["scope", "sync", "config"]
	},
	{
		name:     "repo.config.example"
		title:    "Spec Config Example"
		filepath: "examples/.gh-flarebyte.cue"
		labels:   ["cue", "configuration", "spec"]
	},
	{
		name:     "repo.config.fields"
		title:    "Config Sync Field Map"
		filepath: "examples/config-fields.csv"
		arguments: ["format-csv=table"]
		labels:   ["csv", "configuration", "sync"]
	},
	{
		name:     "repo.build.config"
		title:    "Build Config"
		markdown: "Build orchestration is driven by a top-level `build` block in the Cue config. `gh flarebyte build` reads the configured language and uses a Go implementation initially, with Dart allowed later as a supported language. The command should write language-specific binaries under the configured output directory, emit the configured checksum manifest, and keep the output layout stable across languages. Targets are expressed as `os-arch` strings such as `linux-amd64`."
		labels:   ["configuration", "build", "spec"]
	},
	{
		name:     "repo.release.config"
		title:    "Release Config"
		markdown: "Release publication is driven by a top-level `release` block in the Cue config. `gh flarebyte release` should use the configured release tag prefix, source version, artifact layout, release notes policy, and optional `releaseNotesFilePath` to publish a GitHub release from the build outputs."
		labels:   ["configuration", "release", "spec"]
	},
	{
		name:     "repo.topics.config"
		title:    "Topic Sync"
		markdown: "Topics are kept as a flat string list in the cue config and synchronized as the complete desired repository topics set. If applying the config would delete remote topics, `gh flarebyte repo update` should fail unless the user explicitly confirms deletions."
		labels:   ["configuration", "topics", "spec"]
	},
	{
		name:     "repo.labels.config"
		title:    "Label Sync"
		markdown: "Labels use a structured list of objects in the cue config so name, color, and description can be reconciled separately from repository topics. The config represents the complete desired label set; labels missing from config should be treated as deletions and require explicit confirmation before update."
		labels:   ["configuration", "labels", "spec"]
	},
	{
		name:     "repo.config.coverage"
		title:    "Additional Config Coverage"
		markdown: "The Cue sync config now models homepage, visibility, template, advanced security, secret scanning, push protection, allow-update-branch, and squash-merge commit-message settings."
		labels:   ["configuration", "coverage", "existing-gh"]
	},
	{
		name:     "sync.types"
		title:    "Sync Types"
		filepath: "examples/sync.ts"
		labels:   ["typescript", "sync", "spec"]
	},
	{
		name:     "command.matrix"
		title:    "Extension Command Matrix"
		filepath: "examples/command-matrix.csv"
		arguments: ["format-csv=table"]
		labels:   ["csv", "commands", "spec"]
	},
	{
		name:     "command.flows"
		title:    "Extension Command Flows"
		filepath: "examples/command-flows.ts"
		labels:   ["typescript", "commands", "spec"]
	},
	{
		name:     "command.build"
		title:    "Build Command"
		markdown: "Build the project from the top-level `build` block. Start with Go only, but keep the config shape open for Dart so the command can grow without changing its contract. The first implementation should produce `<outputDir>/<name>-<target>` artifacts and a configured checksum file, with target names expressed as `os-arch` strings such as `linux-amd64` and driven from config rather than shell scripts."
		labels:   ["commands", "build", "spec"]
	},
	{
		name:     "command.release"
		title:    "Release Command"
		markdown: "Run `gh flarebyte build` first, then publish a GitHub release from the resulting build outputs. Use the top-level `release` block to choose the tag, artifacts, and release note behavior, including `releaseNotesFilePath` when `notesMode` is `notes-file`, and implement the command in Go rather than the current Bun helper."
		labels:   ["commands", "release", "spec"]
	},
	{
		name:     "command.init"
		title:    "Init Command"
		markdown: "Bootstrap a repository by seeding `.gh-flarebyte.cue` with repository, build, and release defaults and then applying the initial syncable repo settings."
		labels:   ["commands", "init", "spec"]
	},
	{
		name:     "command.update"
		title:    "Update Command"
		markdown: "Reconcile the live GitHub repository from `.gh-flarebyte.cue`, including repo settings, topics, and label definitions. Topics and labels are exact-set sync targets, so remote items missing from config should be treated as deletions. The command must fail unless the user explicitly confirms deletions, for example with `--confirm-deletions`."
		labels:   ["commands", "update", "spec"]
	},
	{
		name:     "command.audit"
		title:    "Audit Command"
		markdown: "Compare the checked-in Cue config with GitHub and report drift without changing remote state."
		labels:   ["commands", "audit", "spec"]
	},
	{
		name:     "command.repos.mine"
		title:    "Repos Mine Command"
		markdown: "List repositories the current user contributes to within an organization so the extension can discover target repos before sync."
		labels:   ["commands", "discovery", "spec"]
	},
	{
		name:     "label.matrix"
		title:    "Existing Label Behaviour"
		filepath: "examples/label-matrix.csv"
		arguments: ["format-csv=table"]
		labels:   ["csv", "labels", "existing-gh"]
	},
	{
		name:     "release.matrix"
		title:    "Existing Release Behaviour"
		filepath: "examples/release-matrix.csv"
		arguments: ["format-csv=table"]
		labels:   ["csv", "releases", "existing-gh"]
	},
	{
		name:     "search.query"
		title:    "Existing Search Shapes"
		filepath: "examples/search-query.ts"
		labels:   ["typescript", "search", "existing-gh"]
	},
	{
		name:     "gh.repo.edit.flags"
		title:    "Existing gh Repo Edit Flags"
		filepath: "examples/gh-repo-edit.csv"
		arguments: ["format-csv=table"]
		labels:   ["csv", "github", "existing-gh"]
	},
	{
		name:     "repo.discovery"
		title:    "Existing Discovery Shape"
		filepath: "examples/repo-discovery.ts"
		labels:   ["typescript", "discovery", "existing-gh"]
	},
	{
		name:     "project.open-questions"
		title:    "Open Questions"
		markdown: "Clarify the non-interactive CI story for deletion confirmation, the first Dart build contract once it lands, and whether release notes should eventually support templates beyond generated notes and a single notes file path."
		labels:   ["questions", "sync", "policy"]
	},
]

relationships: [
	{
		from:   "repo.config.example"
		to:     "repo.config.fields"
		label:  "described_by"
		labels: ["configuration", "documentation"]
	},
	{
		from:   "repo.config.example"
		to:     "sync.types"
		label:  "shaped_by"
		labels: ["sync", "types"]
	},
	{
		from:   "command.matrix"
		to:     "command.flows"
		label:  "illustrated_by"
		labels: ["commands", "workflow"]
	},
	{
		from:   "command.matrix"
		to:     "gh.repo.edit.flags"
		label:  "backs_onto"
		labels: ["commands", "github"]
	},
	{
		from:   "command.matrix"
		to:     "label.matrix"
		label:  "extends_to"
		labels: ["commands", "labels"]
	},
	{
		from:   "command.matrix"
		to:     "release.matrix"
		label:  "extends_to"
		labels: ["commands", "releases"]
	},
	{
		from:   "command.matrix"
		to:     "search.query"
		label:  "extends_to"
		labels: ["commands", "search"]
	},
	{
		from:   "repo.discovery"
		to:     "command.matrix"
		label:  "supports"
		labels: ["discovery", "commands"]
	},
]

argumentRegistry: {
	version: "1"
	arguments: [
		{
			name:          "format-csv"
			valueType:     "enum"
			scopes: ["note"]
			allowedValues: ["table"]
			defaultValue:  "table"
		},
	]
}
