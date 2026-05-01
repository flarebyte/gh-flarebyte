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
