package cli

import "github.com/spf13/cobra"

var topInitOpts stackInitOptions

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize the default stack",
	Long: `Initialize the default stack.

This is shorthand for 'angee stack init dev'. The CLI submits the request to
the operator provisioning path; it does not render templates directly.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) == 1 {
			path = args[0]
		}
		return runStackInit("dev", path, topInitOpts)
	},
}

func init() {
	addStackInitFlags(initCmd, &topInitOpts)
}
