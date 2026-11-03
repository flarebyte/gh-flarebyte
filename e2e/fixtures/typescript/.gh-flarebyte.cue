package ghflarebyte

project: { org: "flarebyte", repo: "typescript-fixture" }
sync: { mode: "push" }
repository: {
  description: "TypeScript library and CLI fixture"
  defaultBranch: "main"
  homepage: "https://github.com/flarebyte/gh-flarebyte"
  visibility: "public"
  template: false
  topics: ["typescript", "bun"]
  labels: [{ name: "bug", color: "B60205", description: "Bug" }]
}
build: {
  language: "typescript"
  mode: "library"
  packages: ["./..."]
  runTests: true
  outputDir: "build"
  checksumFile: "build/checksums.txt"
}
devOutput: { color: "false", style: "summary", showPassed: true }
coverage: { min: 0, enforceMin: true }
release: {
  versionSource: "package.json"
  tagPrefix: "v"
  notesMode: "generate-notes"
  artifactDir: "build"
  includeArtifacts: false
  includeChecksums: true
}
