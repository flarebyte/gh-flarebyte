package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func handleConfigValidate(args []string, stdout, stderr io.Writer) Result {
	configPath, err := parseConfigPath(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return Result{ExitCode: ExitUsage, Err: err}
	}
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

func parseConfigPath(args []string) (string, error) {
	configPath := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" {
			if i+1 >= len(args) {
				return "", errors.New("invalid invocation: --config requires a path")
			}
			configPath = args[i+1]
			i++
			continue
		}
		return "", fmt.Errorf("invalid invocation: unknown argument %q", arg)
	}
	return configPath, nil
}
