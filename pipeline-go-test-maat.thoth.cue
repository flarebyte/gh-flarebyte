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
      return thoth.ends_with(locator, "_test.go")
      """
  }

  map: {
    inline: """
      return {
        language = "go",
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
      "--rules 'import_files_list,function_map' " +
      "--language go " +
      "--json",
    ]
  }

  postMap: {
    inline: """
      local rules = shell and shell.json and shell.json.rules or {}
      return {
        meta = {
          language = mapped and mapped.language or "go",
          import_files_list = rules.import_files_list or {},
          function_list = thoth.sort_keys(rules.function_map or {}),
        },
      }
      """
  }

  persistMeta: {
    enabled: true
    outDir: "./thoth-meta/go-test"
  }

  output: {
    out: "./temp/pipeline-go-test-maat.json"
    pretty: true
    lines: false
  }

  ui: {
    progress: true
    progressIntervalMs: 250
  }
}
