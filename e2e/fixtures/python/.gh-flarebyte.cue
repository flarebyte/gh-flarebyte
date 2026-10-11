package ghflarebyte

project: { org: "flarebyte", repo: "python-fixture" }
sync: { mode: "push" }
repository: {
  description: "Python 3 and uv fixture"
  defaultBranch: "main"
  homepage: "https://github.com/flarebyte/gh-flarebyte"
  visibility: "public"
  template: false
  topics: ["python", "uv"]
  labels: [{ name: "bug", color: "B60205", description: "Bug" }]
}
build: {
  language: "python"
  mode: "package"
  runTests: true
  outputDir: "build"
  checksumFile: "build/checksums.txt"
}
devOutput: { color: "false", style: "summary", showPassed: true }
coverage: { min: 80, enforceMin: true }
release: {
  versionSource: "pyproject.toml"
  tagPrefix: "v"
  notesMode: "generate-notes"
  artifactDir: "build"
  includeArtifacts: true
  includeChecksums: true
}
