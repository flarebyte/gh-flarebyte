// purpose: Print exact Swift package guidance after a successful named-package check.
// responsibilities: Leave Package.swift edits and dependency resolution to the normal Swift workflow.
package cli

import (
	"fmt"
	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
	"io"
)

func printSwiftDependency(name, version string, stdout, stderr io.Writer) Result {
	canonical, exact, err := dependencies.Parse("swift", name+"@"+version)
	if err != nil || canonical != name || exact == nil || *exact != version {
		_, _ = fmt.Fprintln(stderr, "invalid exact Swift coordinate")
		return Result{ExitCode: ExitUsage}
	}
	_, err = fmt.Fprintf(stdout, "PRINT ONLY — nothing installed.\nAdd to the dependencies section of Package.swift:\n.package(url: %q, exact: %q)\nUse your normal Swift Package Manager workflow to resolve dependencies and build.\n", name, version)
	if err != nil {
		return Result{ExitCode: ExitDependencyFailure, Err: err}
	}
	return Result{ExitCode: ExitOK}
}
