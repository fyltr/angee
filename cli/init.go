package cli

import "github.com/spf13/cobra"

var (
	topInitOpts      stackInitOptions
	topInitAgentOpts agentInitOptions
	topInitDev       bool
)

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

var initAgentCmd = &cobra.Command{
	Use:   "agent <agent>",
	Short: "Provision an agent-backed workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentInit(args[0], topInitAgentOpts)
	},
}

func init() {
	addStackInitFlags(initCmd, &topInitOpts)
	initCmd.Flags().BoolVar(&topInitDev, "dev", false, "Use the dev stack template")
	addAgentInitFlags(initAgentCmd, &topInitAgentOpts)
	initCmd.AddCommand(initAgentCmd)
}
