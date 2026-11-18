// purpose: Delegate a recommended exact npm version to the user's normal package manager.
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

func runDelegatedNPM(ctx context.Context, dir, name, version string, stdin io.Reader, stdout, stderr io.Writer) Result {
	parsed, exact, err := dependencies.Parse("npm", name+"@"+version)
	if err != nil || parsed != name || exact == nil || *exact != version {
		_, _ = fmt.Fprintln(stderr, "invalid exact npm coordinate")
		return Result{ExitCode: ExitUsage}
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if ctx.Err() != nil {
		return Result{ExitCode: 130, Err: ctx.Err()}
	}
	// Validated values are individual arguments; npm owns all resolution and mutation.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.CommandContext(ctx, "npm", "install", "--save-exact", "--", name+"@"+version)
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
	_, _ = fmt.Fprintln(stderr, "cannot start or finish native npm:", err)
	return Result{ExitCode: ExitDependencyFailure, Err: err}
}
