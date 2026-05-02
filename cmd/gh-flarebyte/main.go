// purpose: Provide the gh-flarebyte process entrypoint that initializes runtime build metadata and delegates command execution.
// responsibilities: Bootstrap runtime defaults; invoke cli.Run with stdio; exit with the returned status code.
// architecture notes: GoVersion is hydrated at runtime only when missing so ldflags-built binaries keep their embedded value while local dev runs still report accurate runtime info.
package main

import (
	"os"
	"runtime"

	"github.com/flarebyte/gh-flarebyte/internal/buildinfo"
	"github.com/flarebyte/gh-flarebyte/internal/cli"
)

func main() {
	// Keep Go version accurate by default in local builds.
	if buildinfo.GoVersion == "unknown" || buildinfo.GoVersion == "" {
		buildinfo.GoVersion = runtime.Version()
	}
	result := cli.Run(os.Args[1:], os.Stdout, os.Stderr)
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}
