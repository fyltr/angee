package cli

import "github.com/spf13/cobra"

// fixturesCmd forwards to the framework's `angee fixtures` subcommand
// (typically `angee fixtures load`).
var fixturesCmd = &cobra.Command{
	Use:                "fixtures [framework-args...]",
	Short:              "Manage framework fixtures (project-mode → manage.py angee fixtures)",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return dispatchToRuntime("fixtures", args)
	},
}
