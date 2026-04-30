package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"

	"github.com/flarebyte/gh-flarebyte/internal/buildinfo"
	"github.com/flarebyte/gh-flarebyte/internal/config"
)

// Exit codes aligned with the current design contract.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

type VersionInfo struct {
	Version   string `json:"version"`
	CommitID  string `json:"commitId"`
	Date      string `json:"date"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"goVersion"`
}

type Result struct {
	ExitCode int
	Err      error
}

func Run(args []string, stdout, stderr io.Writer) Result {
	if len(args) == 0 || contains(args, "--help") {
		printHelp(stdout)
		return Result{ExitCode: ExitOK}
	}

	if len(args) >= 2 && args[0] == "config" && args[1] == "validate" {
		configPath, err := parseConfigPath(args[2:])
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		absPath, err := config.ResolvePath(configPath)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		if _, err := config.Load(configPath); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return Result{ExitCode: ExitUsage, Err: err}
		}
		fmt.Fprintf(stdout, "config is valid: %s\n", absPath)
		return Result{ExitCode: ExitOK}
	}

	hasVersion := contains(args, "--version")
	hasJSON := contains(args, "--json")

	if hasJSON && !hasVersion {
		fmt.Fprintln(stderr, "invalid invocation: --json must be used with --version")
		return Result{ExitCode: ExitUsage, Err: errors.New("invalid invocation")}
	}

	if hasVersion {
		v := currentVersionInfo()
		if hasJSON {
			enc := json.NewEncoder(stdout)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(v); err != nil {
				return Result{ExitCode: ExitFailure, Err: err}
			}
			return Result{ExitCode: ExitOK}
		}
		fmt.Fprintf(
			stdout,
			"gh-flarebyte %s commitId=%s date=%s os=%s arch=%s goVersion=%s\n",
			v.Version,
			v.CommitID,
			v.Date,
			v.OS,
			v.Arch,
			v.GoVersion,
		)
		return Result{ExitCode: ExitOK}
	}

	fmt.Fprintf(stderr, "unknown arguments: %v\n", args)
	fmt.Fprintln(stderr, "run `gh flarebyte --help` for usage")
	return Result{ExitCode: ExitUsage, Err: errors.New("invalid invocation")}
}

func currentVersionInfo() VersionInfo {
	gv := buildinfo.GoVersion
	if gv == "unknown" || gv == "" {
		gv = runtime.Version()
	}
	return VersionInfo{
		Version:   buildinfo.Version,
		CommitID:  buildinfo.CommitID,
		Date:      buildinfo.Date,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: gv,
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "gh flarebyte - manage GitHub repository state from .gh-flarebyte.cue")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gh flarebyte --help")
	fmt.Fprintln(w, "  gh flarebyte --version [--json]")
	fmt.Fprintln(w, "  gh flarebyte config validate [--config path]")
}

func contains(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
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
