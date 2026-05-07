package cli

import (
	"github.com/fyltr/angee/api"
	"github.com/spf13/cobra"
)

type stackInitOptions struct {
	Template string
	Set      []string
	Secrets  []string
	Ports    []string
	Force    bool
	Yes      bool
}

type stackUpdateOptions struct {
	Set     []string
	Secrets []string
	Ports   []string
	Yes     bool
}

var (
	stackInitOpts   stackInitOptions
	stackUpdateOpts stackUpdateOptions
)

var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Manage stacks",
}

var stackInitCmd = &cobra.Command{
	Use:   "init <name> [path]",
	Short: "Initialize a stack from a template",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) == 2 {
			path = args[1]
		}
		return runStackInit(args[0], path, stackInitOpts)
	},
}

var stackUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the current stack from its active template",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		set, err := parseKeyValueFlags(stackUpdateOpts.Set, "--set")
		if err != nil {
			return err
		}
		secrets, err := parseKeyValueFlags(stackUpdateOpts.Secrets, "--secret")
		if err != nil {
			return err
		}
		ports, err := parsePortFlags(stackUpdateOpts.Ports)
		if err != nil {
			return err
		}
		req := api.StackUpdateRequest{
			Root:    resolveRoot(),
			Set:     set,
			Secrets: secrets,
			Ports:   ports,
			Yes:     stackUpdateOpts.Yes,
		}
		return postProvision("/stacks/update", req)
	},
}

func init() {
	addStackInitFlags(stackInitCmd, &stackInitOpts)
	stackUpdateCmd.Flags().StringArrayVar(&stackUpdateOpts.Set, "set", nil, "Set a template value: --set name=value")
	stackUpdateCmd.Flags().StringArrayVar(&stackUpdateOpts.Secrets, "secret", nil, "Supply a secret: --secret name=value")
	stackUpdateCmd.Flags().StringArrayVar(&stackUpdateOpts.Ports, "port", nil, "Override a port lease: --port name=8120")
	stackUpdateCmd.Flags().BoolVarP(&stackUpdateOpts.Yes, "yes", "y", false, "Accept defaults and run non-interactively")
	stackCmd.AddCommand(stackInitCmd, stackUpdateCmd)
}

func addStackInitFlags(cmd *cobra.Command, opts *stackInitOptions) {
	cmd.Flags().StringVarP(&opts.Template, "template", "t", "", "Template ref or path")
	cmd.Flags().StringArrayVar(&opts.Set, "set", nil, "Set a template value: --set name=value")
	cmd.Flags().StringArrayVar(&opts.Secrets, "secret", nil, "Supply a secret: --secret name=value")
	cmd.Flags().StringArrayVar(&opts.Ports, "port", nil, "Override a port lease: --port name=8120")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Overwrite an existing stack root")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Accept defaults and run non-interactively")
}

func runStackInit(name, path string, opts stackInitOptions) error {
	set, err := parseKeyValueFlags(opts.Set, "--set")
	if err != nil {
		return err
	}
	secrets, err := parseKeyValueFlags(opts.Secrets, "--secret")
	if err != nil {
		return err
	}
	ports, err := parsePortFlags(opts.Ports)
	if err != nil {
		return err
	}
	req := api.StackInitRequest{
		Name:     name,
		Path:     path,
		Root:     resolveInitRoot(path),
		Template: opts.Template,
		Set:      set,
		Secrets:  secrets,
		Ports:    ports,
		Force:    opts.Force,
		Yes:      opts.Yes,
	}
	return postProvision("/stacks/init", req)
}
