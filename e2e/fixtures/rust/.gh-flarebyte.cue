package ghflarebyte

project: { org: "flarebyte", repo: "rust-fixture" }
sync: { mode: "push" }
repository: {
  description: "Rust Cargo fixture"
  defaultBranch: "main"
  homepage: "https://github.com/flarebyte/gh-flarebyte"
  visibility: "public"
  template: false
  topics: ["rust", "cargo"]
  labels: [{ name: "bug", color: "B60205", description: "Bug" }]
}
build: {
  language: "rust"
  mode: "library"
  packages: ["./..."]
  mainPackage: "app"
  runTests: true
  outputDir: "build"
  checksumFile: "build/checksums.txt"
}
devOutput: { color: "false", style: "summary", showPassed: true }
coverage: { min: 50, enforceMin: true }
release: {
  versionSource: "math/Cargo.toml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  artifactDir: "build"
  includeArtifacts: false
  includeChecksums: true
}
