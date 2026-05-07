package cli

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"text/tabwriter"

	"github.com/fyltr/angee/api"
	"github.com/spf13/cobra"
)

var validAgentName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type agentInitOptions struct {
	Template          string
	WorkspaceTemplate string
	Branch            string
	Overrides         []string
	Secrets           []string
	Ports             []string
	Dev               bool
	CreateBranches    bool
	Start             bool
	Yes               bool
}

type agentUpdateOptions struct {
	Template          string
	WorkspaceTemplate string
	Ref               string
	Overrides         []string
	Secrets           []string
	Ports             []string
	Restart           bool
	Yes               bool
}

var (
	agentInitOpts   agentInitOptions
	agentUpdateOpts agentUpdateOptions
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agent-backed workspaces",
}

var agentInitCmd = &cobra.Command{
	Use:   "init <agent>",
	Short: "Provision an agent-backed workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentInit(args[0], agentInitOpts)
	},
}

var agentUpdateCmd = &cobra.Command{
	Use:   "update <agent>",
	Short: "Update an agent from its template",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAgentName(args[0]); err != nil {
			return err
		}
		overrides, err := parseKeyValueFlags(agentUpdateOpts.Overrides, "--override")
		if err != nil {
			return err
		}
		secrets, err := parseKeyValueFlags(agentUpdateOpts.Secrets, "--secret")
		if err != nil {
			return err
		}
		ports, err := parsePortFlags(agentUpdateOpts.Ports)
		if err != nil {
			return err
		}
		req := api.AgentUpdateRequest{
			Name:              args[0],
			Root:              resolveRoot(),
			Template:          agentUpdateOpts.Template,
			WorkspaceTemplate: agentUpdateOpts.WorkspaceTemplate,
			Ref:               agentUpdateOpts.Ref,
			Overrides:         overrides,
			Secrets:           secrets,
			Ports:             ports,
			Restart:           agentUpdateOpts.Restart,
			Yes:               agentUpdateOpts.Yes,
		}
		return postProvision("/agents/"+args[0]+"/update", req)
	},
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agents",
	Args:  cobra.NoArgs,
	RunE:  runAgentList,
}

var agentStartCmd = &cobra.Command{
	Use:   "start <agent>",
	Short: "Start an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAgentName(args[0]); err != nil {
			return err
		}
		return postProvision("/agents/"+args[0]+"/start", nil)
	},
}

var agentStopCmd = &cobra.Command{
	Use:   "stop <agent>",
	Short: "Stop an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAgentName(args[0]); err != nil {
			return err
		}
		return postProvision("/agents/"+args[0]+"/stop", nil)
	},
}

var agentRestartCmd = &cobra.Command{
	Use:   "restart <agent>",
	Short: "Restart an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAgentName(args[0]); err != nil {
			return err
		}
		return postProvision("/agents/"+args[0]+"/restart", api.AgentActionRequest{Name: args[0], Root: resolveRoot()})
	},
}

var agentDestroyCmd = &cobra.Command{
	Use:   "destroy <agent>",
	Short: "Destroy an agent workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAgentName(args[0]); err != nil {
			return err
		}
		return postProvision("/agents/"+args[0]+"/destroy", api.AgentActionRequest{Name: args[0], Root: resolveRoot()})
	},
}

var agentLogsCmd = &cobra.Command{
	Use:   "logs <agent>",
	Short: "Show agent logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAgentName(args[0]); err != nil {
			return err
		}
		return streamAgentLogs(args[0])
	},
}

func init() {
	addAgentInitFlags(agentInitCmd, &agentInitOpts)

	agentUpdateCmd.Flags().StringVar(&agentUpdateOpts.Template, "template", "", "Agent template ref")
	agentUpdateCmd.Flags().StringVar(&agentUpdateOpts.WorkspaceTemplate, "workspace-template", "", "Workspace template ref")
	agentUpdateCmd.Flags().StringVar(&agentUpdateOpts.Ref, "ref", "", "Template or source ref to update to")
	agentUpdateCmd.Flags().StringArrayVar(&agentUpdateOpts.Overrides, "override", nil, "Override a source ref: --override source=ref")
	agentUpdateCmd.Flags().StringArrayVar(&agentUpdateOpts.Secrets, "secret", nil, "Supply a secret: --secret name=value")
	agentUpdateCmd.Flags().StringArrayVar(&agentUpdateOpts.Ports, "port", nil, "Override a port lease: --port name=8120")
	agentUpdateCmd.Flags().BoolVar(&agentUpdateOpts.Restart, "restart", false, "Restart services after update")
	agentUpdateCmd.Flags().BoolVarP(&agentUpdateOpts.Yes, "yes", "y", false, "Accept defaults and run non-interactively")

	agentLogsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	agentLogsCmd.Flags().IntVarP(&logsLines, "lines", "n", 100, "Number of lines to show")

	agentCmd.AddCommand(
		agentInitCmd,
		agentListCmd,
		agentStartCmd,
		agentStopCmd,
		agentRestartCmd,
		agentLogsCmd,
		agentUpdateCmd,
		agentDestroyCmd,
	)
}

func validateAgentName(name string) error {
	if !validAgentName.MatchString(name) {
		return fmt.Errorf("invalid agent name %q: must match %s", name, validAgentName.String())
	}
	return nil
}

func runAgentInit(name string, opts agentInitOptions) error {
	if err := validateAgentName(name); err != nil {
		return err
	}
	if opts.Dev && opts.Template == "" {
		opts.Template = "agents/angee-dev"
	}
	overrides, err := parseKeyValueFlags(opts.Overrides, "--override")
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
	req := api.AgentInitRequest{
		Name:              name,
		Root:              resolveRoot(),
		Template:          opts.Template,
		WorkspaceTemplate: opts.WorkspaceTemplate,
		Branch:            opts.Branch,
		Overrides:         overrides,
		Secrets:           secrets,
		Ports:             ports,
		CreateBranches:    opts.CreateBranches,
		Start:             opts.Start,
		Yes:               opts.Yes,
	}
	return postProvision("/agents/init", req)
}

func addAgentInitFlags(cmd *cobra.Command, opts *agentInitOptions) {
	cmd.Flags().StringVar(&opts.Template, "template", "", "Agent template ref")
	cmd.Flags().StringVar(&opts.WorkspaceTemplate, "workspace-template", "", "Workspace template ref")
	cmd.Flags().StringVar(&opts.Branch, "branch", "", "Branch or ref for workspace sources")
	cmd.Flags().StringArrayVar(&opts.Overrides, "override", nil, "Override a source ref: --override source=ref")
	cmd.Flags().StringArrayVar(&opts.Secrets, "secret", nil, "Supply a secret: --secret name=value")
	cmd.Flags().StringArrayVar(&opts.Ports, "port", nil, "Override a port lease: --port name=8120")
	cmd.Flags().BoolVar(&opts.Dev, "dev", false, "Use the default dev agent template")
	cmd.Flags().BoolVar(&opts.CreateBranches, "create-branches", false, "Create missing branches")
	cmd.Flags().BoolVar(&opts.Start, "start", false, "Start agent services after provisioning")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Accept defaults and run non-interactively")
}

func runAgentList(cmd *cobra.Command, args []string) error {
	var agents []api.AgentInfo
	if _, err := apiGet("/agents", &agents); err != nil {
		return err
	}
	if outputJSON {
		return nil
	}
	if len(agents) == 0 {
		fmt.Println("No agents found.")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tHEALTH\tLIFECYCLE\tROLE")
	for _, agent := range agents {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", agent.Name, agent.Status, agent.Health, agent.Lifecycle, agent.Role)
	}
	return w.Flush()
}

func streamAgentLogs(agent string) error {
	params := url.Values{}
	params.Set("lines", fmt.Sprintf("%d", logsLines))
	if logsFollow {
		params.Set("follow", "true")
	}
	return streamAPIGet(fmt.Sprintf("/agents/%s/logs", agent), params, os.Stdout)
}
