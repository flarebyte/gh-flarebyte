// purpose: Delegate a recommended exact package version to the user's normal package manager.
// responsibilities: Preserve native streams/status, stdin and cancellation without staging or resolving dependencies.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

var delegateNPM = runDelegatedNPM
var delegateGo = runDelegatedGo
var delegatePyPI = runDelegatedPyPI
var delegateCargo = runDelegatedCargo

func runDelegatedNPM(ctx context.Context, dir, name, version string, stdin io.Reader, stdout, stderr io.Writer) Result {
	return runDelegatedPackage(ctx, dir, "npm", name, version, stdin, stdout, stderr)
}

func runDelegatedGo(ctx context.Context, dir, name, version string, stdin io.Reader, stdout, stderr io.Writer) Result {
	return runDelegatedPackage(ctx, dir, "go", name, version, stdin, stdout, stderr)
}

func runDelegatedPyPI(ctx context.Context, dir, name, version string, stdin io.Reader, stdout, stderr io.Writer) Result {
	return runDelegatedPackage(ctx, dir, "pypi", name, version, stdin, stdout, stderr)
}

func runDelegatedCargo(ctx context.Context, dir, name, version string, stdin io.Reader, stdout, stderr io.Writer) Result {
	return runDelegatedPackage(ctx, dir, "cargo", name, version, stdin, stdout, stderr)
}

func runDelegatedPackage(ctx context.Context, dir, system, name, version string, stdin io.Reader, stdout, stderr io.Writer) Result {
	var executable string
	var args []string
	switch system {
	case "npm":
		executable = "npm"
		args = []string{"install", "--save-exact", "--", name + "@" + version}
	case "go":
		executable = "go"
		args = []string{"get", name + "@" + version}
	case "pypi":
		executable = "uv"
		args = []string{"add", name + "==" + version}
	case "cargo":
		executable = "cargo"
		args = []string{"add", name + "@=" + version}
	default:
		return Result{ExitCode: ExitDependencyUnsupported}
	}
	parsed, exact, err := dependencies.Parse(system, name+"@"+version)
	if err != nil || parsed != name || exact == nil || *exact != version {
		_, _ = fmt.Fprintf(stderr, "invalid exact %s coordinate\n", system)
		return Result{ExitCode: ExitUsage}
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if ctx.Err() != nil {
		return Result{ExitCode: 130, Err: ctx.Err()}
	}
	// Executables are selected above; validated coordinates are separate arguments.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.Stdout, cmd.Stderr = stdout, stdout
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		return Result{ExitCode: 130, Err: ctx.Err()}
	}
	if err == nil {
		return Result{ExitCode: ExitOK}
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() > 0 {
		return Result{ExitCode: exit.ExitCode(), Err: err}
	}
	_, _ = fmt.Fprintf(stderr, "cannot start or finish native %s: %v\n", executable, err)
	return Result{ExitCode: ExitDependencyFailure, Err: err}
}
