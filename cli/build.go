package cli

import "github.com/spf13/cobra"

// buildCmd is a thin Cobra forwarder. Project-mode (parent-walk finds
// `.angee/project.yaml`) → exec the framework's `angee build` subcommand
// via the runtime adapter. DisableFlagParsing so framework-side flags
// (`--check`, `--watch`, `--output ...`) pass through verbatim.
var buildCmd = &cobra.Command{
	Use:                "build [framework-args...]",
	Short:              "Compose installed packages into runtime/ (project-mode → manage.py angee build)",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return dispatchToRuntime("build", args)
	},
}
