package cli

import "github.com/spf13/cobra"

// migrateCmd forwards to the framework's `angee migrate` subcommand.
// DisableFlagParsing so framework-side flags pass through verbatim.
var migrateCmd = &cobra.Command{
	Use:                "migrate [framework-args...]",
	Short:              "Run the framework's migrations (project-mode → manage.py angee migrate)",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return dispatchToRuntime("migrate", args)
	},
}
