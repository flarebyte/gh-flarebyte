package flyb

source: "minimal"
name:   "minimal"
modules: ["core"]

reports: [{
	title:       "Minimal Architecture Report"
	filepath:    "out/minimal.md"
	description: "Small starter report."
	sections: [{
		title:       "Overview"
		description: "Minimal section set."
		sections: [{
			title:       "Core Nodes"
			description: "Plain note rendering."
			notes: ["app.api", "app.db"]
		}]
	}]
}]

notes: [
	{
		name:     "app.api"
		title:    "API Service"
		markdown: "Handles client requests."
		labels: ["service", "api"]
	},
	{
		name:     "app.db"
		title:    "Primary Database"
		markdown: "Stores persistent state."
		labels: ["storage", "database"]
	},
]

relationships: [{
	from:   "app.api"
	to:     "app.db"
	label:  "depends_on"
	labels: ["depends_on"]
}]

argumentRegistry: {
	version:   "1"
	arguments: []
}
