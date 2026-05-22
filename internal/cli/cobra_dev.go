package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type exitCodeError struct {
	code int
	err  error
}

func (e exitCodeError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return ""
}

func runWithCobra(args []string, stdout, stderr io.Writer) Result {
	if shouldPrintRootHelp(args) {
		printHelp(stdout)
		return Result{ExitCode: ExitOK}
	}
	root := newCobraRoot(stdout, stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var exitErr exitCodeError
		if errors.As(err, &exitErr) {
			return Result{ExitCode: exitErr.code, Err: exitErr.err}
		}
		_, _ = fmt.Fprintf(stderr, "unknown arguments: %v\n", args)
		_, _ = fmt.Fprintln(stderr, "run `gh flarebyte --help` for usage")
		return Result{ExitCode: ExitUsage, Err: errors.New("invalid invocation")}
	}
	return Result{ExitCode: ExitOK}
}

func shouldPrintRootHelp(args []string) bool {
	if len(args) == 0 {
		return true
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return true
	}
	return false
}

func newCobraRoot(stdout, stderr io.Writer) *cobra.Command {
	var versionFlag bool
	var jsonFlag bool

	root := &cobra.Command{
		Use:           "gh flarebyte",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonFlag && !versionFlag {
				_, _ = fmt.Fprintln(stderr, "invalid invocation: --json must be used with --version")
				return exitCodeError{code: ExitUsage, err: errors.New("invalid invocation")}
			}
			if versionFlag {
				v := currentVersionInfo()
				if jsonFlag {
					enc := json.NewEncoder(stdout)
					enc.SetEscapeHTML(false)
					if err := enc.Encode(v); err != nil {
						return exitCodeError{code: ExitFailure, err: err}
					}
					return nil
				}
				_, _ = fmt.Fprintf(
					stdout,
					"gh-flarebyte %s commitId=%s date=%s os=%s arch=%s goVersion=%s\n",
					v.Version, v.CommitID, v.Date, v.OS, v.Arch, v.GoVersion,
				)
				return nil
			}
			printHelp(stdout)
			return nil
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.Flags().BoolVar(&versionFlag, "version", false, "Print version information")
	root.Flags().BoolVar(&jsonFlag, "json", false, "Print JSON output where supported")
	root.AddCommand(
		newBuildCobraCommand(stdout, stderr),
		newTestCobraCommand(stdout, stderr),
		newDevSubCommand("format", handleFormat, stdout, stderr),
		newDevSubCommand("lint", handleLint, stdout, stderr),
		newCovCobraCommand(stdout, stderr),
		newReleaseCobraCommand(stdout, stderr),
		newRepoCommand(stdout, stderr),
		newReposCommand(stdout, stderr),
		newConfigCommand(stdout, stderr),
	)
	return root
}

func newDevSubCommand(name string, handler func([]string, io.Writer, io.Writer) Result, stdout, stderr io.Writer) *cobra.Command {
	return newSubCommand(name, handler, stdout, stderr)
}

func newTestCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	var style string
	var color string
	cmd := &cobra.Command{
		Use:           "test",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return exitCodeError{code: ExitUsage, err: fmt.Errorf("invalid invocation: unknown argument %q", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if style != "" && style != "summary" && style != "per_test" {
				_, _ = fmt.Fprintf(stderr, "invalid invocation: --style %q is not supported; expected summary or per_test\n", style)
				return exitCodeError{code: ExitUsage, err: fmt.Errorf("invalid invocation: --style %q is not supported; expected summary or per_test", style)}
			}
			if color != "" && color != "auto" && color != "true" && color != "false" {
				_, _ = fmt.Fprintf(stderr, "invalid invocation: --color %q is not supported; expected auto, true, or false\n", color)
				return exitCodeError{code: ExitUsage, err: fmt.Errorf("invalid invocation: --color %q is not supported; expected auto, true, or false", color)}
			}
			res := runTest(style, color, stdout, stderr)
			return resultToError(res)
		},
	}
	cmd.Flags().StringVar(&style, "style", "", "Override output style: summary or per_test")
	cmd.Flags().StringVar(&color, "color", "", "Override color mode: auto, true, or false")
	return cmd
}

func newCovCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	var min float64
	cmd := &cobra.Command{
		Use:           "cov",
		Short:         "Compute test coverage. --min sets a failure threshold percentage (0-100).",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return exitCodeError{code: ExitUsage, err: fmt.Errorf("invalid invocation: unknown argument %q", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var minPtr *float64
			if cmd.Flags().Changed("min") {
				if min < 0 || min > 100 {
					_, _ = fmt.Fprintln(stderr, "invalid invocation: --min must be a number between 0 and 100")
					return exitCodeError{code: ExitUsage, err: fmt.Errorf("invalid invocation: --min must be a number between 0 and 100")}
				}
				minPtr = &min
			}
			res := runCov(minPtr, stdout, stderr)
			return resultToError(res)
		},
	}
	cmd.Flags().Float64Var(&min, "min", 0, "Coverage threshold percentage (0-100)")
	return cmd
}

func newRepoCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "repo",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		newRepoInitCobraCommand(stdout, stderr),
		newRepoUpdateCobraCommand(stdout, stderr),
		newRepoAuditCobraCommand(stdout, stderr),
	)
	return cmd
}

func newReposCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "repos",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newReposMineCobraCommand(stdout, stderr))
	return cmd
}

func newConfigCommand(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "config",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newConfigValidateCobraCommand(stdout, stderr))
	return cmd
}

func newBuildCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	var target, outputDir string
	cmd := &cobra.Command{
		Use:           "build",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return exitCodeError{code: ExitUsage, err: fmt.Errorf("invalid invocation: unknown argument %q", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return resultToError(runBuild(target, outputDir, stdout, stderr))
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Build only one target (os-arch)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Override build output directory")
	return cmd
}

func newReleaseCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	var draft bool
	var notesFile string
	cmd := &cobra.Command{
		Use:           "release",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return exitCodeError{code: ExitUsage, err: fmt.Errorf("invalid invocation: unknown argument %q", args[0])}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return resultToError(runRelease(draft, notesFile, stdout, stderr))
		},
	}
	cmd.Flags().BoolVar(&draft, "draft", false, "Create release as draft")
	cmd.Flags().StringVar(&notesFile, "notes-file", "", "Override notes file path")
	return cmd
}

func newRepoInitCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	var repo string
	var overwrite bool
	cmd := &cobra.Command{Use: "init", SilenceUsage: true, SilenceErrors: true, Args: noExtraArgs}
	cmd.Flags().StringVar(&repo, "repo", "", "Repository owner/name")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing config file")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return resultToError(runRepoInit(repo, overwrite, stdout, stderr))
	}
	return cmd
}

func newRepoUpdateCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	var repo string
	var confirmDeletions bool
	var acceptVisibility bool
	cmd := &cobra.Command{Use: "update", SilenceUsage: true, SilenceErrors: true, Args: noExtraArgs}
	cmd.Flags().StringVar(&repo, "repo", "", "Repository owner/name")
	cmd.Flags().BoolVar(&confirmDeletions, "confirm-deletions", false, "Allow deletions")
	cmd.Flags().BoolVar(&acceptVisibility, "accept-visibility-change-consequences", false, "Allow visibility change")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return resultToError(runRepoUpdate(repo, confirmDeletions, acceptVisibility, stdout, stderr))
	}
	return cmd
}

func newRepoAuditCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	var repo string
	var asJSON bool
	cmd := &cobra.Command{Use: "audit", SilenceUsage: true, SilenceErrors: true, Args: noExtraArgs}
	cmd.Flags().StringVar(&repo, "repo", "", "Repository owner/name")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return resultToError(runRepoAudit(repo, asJSON, stdout, stderr))
	}
	return cmd
}

func newReposMineCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	var org string
	var asJSON bool
	cmd := &cobra.Command{Use: "mine", SilenceUsage: true, SilenceErrors: true, Args: noExtraArgs}
	cmd.Flags().StringVar(&org, "org", "", "Organization")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return resultToError(runReposMine(org, asJSON, stdout, stderr))
	}
	return cmd
}

func newConfigValidateCobraCommand(stdout, stderr io.Writer) *cobra.Command {
	var configPath string
	cmd := &cobra.Command{Use: "validate", SilenceUsage: true, SilenceErrors: true, Args: noExtraArgs}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to config file")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return resultToError(runConfigValidate(configPath, stdout, stderr))
	}
	return cmd
}

func noExtraArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return exitCodeError{code: ExitUsage, err: fmt.Errorf("invalid invocation: unknown argument %q", args[0])}
	}
	return nil
}

func newSubCommand(name string, handler func([]string, io.Writer, io.Writer) Result, stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			res := handler(args, stdout, stderr)
			if res.ExitCode != ExitOK || res.Err != nil {
				return exitCodeError{code: res.ExitCode, err: res.Err}
			}
			return nil
		},
	}
}

func resultToError(res Result) error {
	if res.ExitCode != ExitOK || res.Err != nil {
		return exitCodeError{code: res.ExitCode, err: res.Err}
	}
	return nil
}
