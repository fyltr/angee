package cli

import (
	"github.com/fyltr/angee/api"
	"github.com/spf13/cobra"
)

var (
	devOnly   []string
	devExcept []string
	devFollow bool
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Run the current stack through a local operator",
	Long: `Run the current stack's declared dev services, jobs, and workflows.

The CLI submits a dev reconciliation request to the operator. It does not
dispatch to framework-specific tools or inspect project runtime files.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		req := api.ReconcileRequest{
			Root:   resolveRoot(),
			Mode:   "dev",
			Only:   devOnly,
			Except: devExcept,
			Follow: devFollow,
		}
		return postProvision("/reconcile", req)
	},
}

func init() {
	devCmd.Flags().StringSliceVar(&devOnly, "only", nil, "Run only these declared services/jobs")
	devCmd.Flags().StringSliceVar(&devExcept, "except", nil, "Run all declared services/jobs except these")
	devCmd.Flags().BoolVarP(&devFollow, "follow", "f", false, "Follow logs after reconciliation")
}
