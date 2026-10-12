package ghflarebyte

project: { org: "flarebyte", repo: "swift-fixture" }
sync: { mode: "push" }
repository: {
  description: "SwiftPM fixture"
  defaultBranch: "main"
  homepage: "https://github.com/flarebyte/gh-flarebyte"
  visibility: "public"
  template: false
  topics: ["swift", "swiftpm"]
  labels: [{ name: "bug", color: "B60205", description: "Bug" }]
}
build: {
  language: "swift"
  mode: "library"
  packages: ["./..."]
  runTests: true
  outputDir: "build"
  checksumFile: "build/checksums.txt"
}
devOutput: { color: "false", style: "summary", showPassed: true }
coverage: { min: 0, enforceMin: true }
release: {
  versionSource: "main.project.yaml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  artifactDir: "build"
  includeArtifacts: false
  includeChecksums: true
}
