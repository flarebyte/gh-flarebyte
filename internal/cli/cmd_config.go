// purpose: Expose config validation as a CLI command that fails fast on malformed or incomplete cue config.
// responsibilities: Parse config command flags; resolve config path; load/validate config; print validation status.
// architecture notes: Validation delegates entirely to internal/config to keep command behavior aligned with the same rules used by build/repo/release flows.
package cli

import (
	"fmt"
	"io"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func runConfigValidate(configPath string, stdout, stderr io.Writer) Result {
	absPath, err := config.ResolvePath(configPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	if _, err := config.Load(configPath); err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
	_, _ = fmt.Fprintf(stdout, "config is valid: %s\n", absPath)
	return Result{ExitCode: ExitOK}
}
