package ghflarebyte

project: { org: "flarebyte", repo: "kotlin-fixture" }
sync: { mode: "push" }
repository: {
  description: "Kotlin library and CLI fixture"
  defaultBranch: "main"
  homepage: "https://github.com/flarebyte/gh-flarebyte"
  visibility: "public"
  template: false
  topics: ["kotlin", "jvm"]
  labels: [{ name: "bug", color: "B60205", description: "Bug" }]
}
build: {
  language: "kotlin"
  mode: "library"
  packages: ["./..."]
  runTests: true
  outputDir: "dist"
  checksumFile: "dist/checksums.txt"
}
devOutput: { color: "false", style: "summary", showPassed: true }
coverage: { min: 0, enforceMin: true }
release: {
  versionSource: "main.project.yaml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  artifactDir: "dist"
  includeArtifacts: false
  includeChecksums: true
}
