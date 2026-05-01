{
  configVersion: "1"
  action: "input-pipeline"

  discovery: {
    root: "."
  }

  errors: {
    mode: "keep-going"
    embedErrors: true
  }

  filter: {
    inline: """
      return thoth.starts_with(locator, "e2e/")
        and (
          thoth.ends_with(locator, ".test.ts")
          or thoth.ends_with(locator, ".suite.ts")
        )
      """
  }

  map: {
    inline: """
      return {
        language = "typescript",
      }
      """
  }

  shell: {
    enabled: true
    decodeJsonStdout: true
    program: "/bin/sh"
    workingDir: "."
    argsTemplate: [
      "-c",
      "npx maat-ostraca analyse " +
      "--in '{locator}' " +
      "--rules 'import_files_list,testcase_titles_list' " +
      "--language typescript " +
      "--json",
    ]
  }

  postMap: {
    inline: """
      local rules = shell and shell.json and shell.json.rules or {}
      local language = shell and shell.json and shell.json.language or "typescript"
      return {
        meta = {
          language = language,
          import_files_list = rules.import_files_list or {},
          testcase_titles_list = rules.testcase_titles_list or {},
        },
      }
      """
  }

  persistMeta: {
    enabled: true
    outDir: "./thoth-meta/ts-e2e"
  }

  output: {
    out: "./temp/pipeline-ts-e2e-maat.json"
    pretty: true
    lines: false
  }

  ui: {
    progress: true
    progressIntervalMs: 250
  }
}
