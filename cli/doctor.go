package cli

import "github.com/spf13/cobra"

// doctorCmd forwards to the framework's `angee doctor` subcommand.
var doctorCmd = &cobra.Command{
	Use:                "doctor [framework-args...]",
	Short:              "Validate framework metadata (project-mode → manage.py angee doctor)",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return dispatchToRuntime("doctor", args)
	},
}
