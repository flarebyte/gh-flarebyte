// purpose: Wire concise dependency assessment and guarded installation commands.
// responsibilities: Expose only presentation flags; retain existing Cobra result handling.
// architecture notes: Runtime dependency policy comes from the local repository configuration.
package cli

import (
	"io"

	"github.com/spf13/cobra"
)

func newDepCommand(stdout, stderr io.Writer) *cobra.Command {
	parent := &cobra.Command{Use: "dep", Short: "Assess dependencies before adopting them"}
	for _, action := range []string{"check", "install"} {
		action := action
		var jsonOutput, explain bool
		cmd := &cobra.Command{
			Use:   action + " <ecosystem> <package>",
			Short: map[string]string{"check": "Assess a package or exact version in the local repository", "install": "Install a recommended version using the supported ecosystem workflow"}[action],
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return resultToError(runDependency(cmd.Context(), action, args[0], args[1], jsonOutput, explain, stdout, stderr))
			},
		}
		cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print compact JSON")
		cmd.Flags().BoolVar(&explain, "explain", false, "Include evidence and effective policy")
		parent.AddCommand(cmd)
	}
	return parent
}
