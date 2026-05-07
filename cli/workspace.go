package cli

import (
	"github.com/fyltr/angee/api"
	"github.com/spf13/cobra"
)

type workspaceInitOptions struct {
	Template       string
	Branch         string
	Overrides      []string
	Secrets        []string
	Ports          []string
	CreateBranches bool
	Start          bool
	Yes            bool
}

type workspaceUpdateOptions struct {
	Ref      string
	Override []string
	Secrets  []string
	Ports    []string
	Sync     bool
	Restart  bool
	Yes      bool
}

var (
	workspaceInitOpts   workspaceInitOptions
	workspaceUpdateOpts workspaceUpdateOptions
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
}

var workspaceInitCmd = &cobra.Command{
	Use:   "init <workspace>",
	Short: "Provision a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		overrides, err := parseKeyValueFlags(workspaceInitOpts.Overrides, "--override")
		if err != nil {
			return err
		}
		secrets, err := parseKeyValueFlags(workspaceInitOpts.Secrets, "--secret")
		if err != nil {
			return err
		}
		ports, err := parsePortFlags(workspaceInitOpts.Ports)
		if err != nil {
			return err
		}
		req := api.WorkspaceInitRequest{
			Name:           args[0],
			Root:           resolveRoot(),
			Template:       workspaceInitOpts.Template,
			Branch:         workspaceInitOpts.Branch,
			Overrides:      overrides,
			Secrets:        secrets,
			Ports:          ports,
			CreateBranches: workspaceInitOpts.CreateBranches,
			Start:          workspaceInitOpts.Start,
			Yes:            workspaceInitOpts.Yes,
		}
		return postProvision("/workspaces/init", req)
	},
}

var workspaceUpdateCmd = &cobra.Command{
	Use:   "update <workspace>",
	Short: "Update a workspace from its template and sources",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		overrides, err := parseKeyValueFlags(workspaceUpdateOpts.Override, "--override")
		if err != nil {
			return err
		}
		secrets, err := parseKeyValueFlags(workspaceUpdateOpts.Secrets, "--secret")
		if err != nil {
			return err
		}
		ports, err := parsePortFlags(workspaceUpdateOpts.Ports)
		if err != nil {
			return err
		}
		req := api.WorkspaceUpdateRequest{
			Name:      args[0],
			Root:      resolveRoot(),
			Ref:       workspaceUpdateOpts.Ref,
			Overrides: overrides,
			Secrets:   secrets,
			Ports:     ports,
			Sync:      workspaceUpdateOpts.Sync,
			Restart:   workspaceUpdateOpts.Restart,
			Yes:       workspaceUpdateOpts.Yes,
		}
		return postProvision("/workspaces/"+args[0]+"/update", req)
	},
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspaces",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return postProvision("/workspaces/list", api.WorkspaceListRequest{Root: resolveRoot()})
	},
}

var workspaceDevCmd = &cobra.Command{
	Use:   "dev <workspace>",
	Short: "Run a workspace's dev services",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return postProvision("/workspaces/"+args[0]+"/dev", api.WorkspaceDevRequest{Root: resolveRoot(), Name: args[0]})
	},
}

func init() {
	workspaceInitCmd.Flags().StringVar(&workspaceInitOpts.Template, "template", "", "Workspace template ref")
	workspaceInitCmd.Flags().StringVar(&workspaceInitOpts.Branch, "branch", "", "Branch or ref for workspace sources")
	workspaceInitCmd.Flags().StringArrayVar(&workspaceInitOpts.Overrides, "override", nil, "Override a source ref: --override source=ref")
	workspaceInitCmd.Flags().StringArrayVar(&workspaceInitOpts.Secrets, "secret", nil, "Supply a secret: --secret name=value")
	workspaceInitCmd.Flags().StringArrayVar(&workspaceInitOpts.Ports, "port", nil, "Override a port lease: --port name=8120")
	workspaceInitCmd.Flags().BoolVar(&workspaceInitOpts.CreateBranches, "create-branches", false, "Create missing branches")
	workspaceInitCmd.Flags().BoolVar(&workspaceInitOpts.Start, "start", false, "Start workspace services after provisioning")
	workspaceInitCmd.Flags().BoolVarP(&workspaceInitOpts.Yes, "yes", "y", false, "Accept defaults and run non-interactively")

	workspaceUpdateCmd.Flags().StringVar(&workspaceUpdateOpts.Ref, "ref", "", "Template or source ref to update to")
	workspaceUpdateCmd.Flags().StringArrayVar(&workspaceUpdateOpts.Override, "override", nil, "Override a source ref: --override source=ref")
	workspaceUpdateCmd.Flags().StringArrayVar(&workspaceUpdateOpts.Secrets, "secret", nil, "Supply a secret: --secret name=value")
	workspaceUpdateCmd.Flags().StringArrayVar(&workspaceUpdateOpts.Ports, "port", nil, "Override a port lease: --port name=8120")
	workspaceUpdateCmd.Flags().BoolVar(&workspaceUpdateOpts.Sync, "sync", false, "Sync materialized sources")
	workspaceUpdateCmd.Flags().BoolVar(&workspaceUpdateOpts.Restart, "restart", false, "Restart workspace services after update")
	workspaceUpdateCmd.Flags().BoolVarP(&workspaceUpdateOpts.Yes, "yes", "y", false, "Accept defaults and run non-interactively")

	workspaceCmd.AddCommand(workspaceInitCmd, workspaceUpdateCmd, workspaceListCmd, workspaceDevCmd)
}
