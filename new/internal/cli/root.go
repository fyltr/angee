package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fyltr/angee/api"
	"github.com/fyltr/angee/internal/operator"
	"github.com/fyltr/angee/internal/service"
	"github.com/spf13/cobra"
)

var Version = "dev"

func Execute() error {
	return NewRoot(os.Stdout, os.Stderr).Execute()
}

func NewRoot(stdout, stderr io.Writer) *cobra.Command {
	var root string
	var operatorURL string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:           "angee",
		Short:         "Stack manager for angee.ai",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.PersistentFlags().StringVar(&root, "root", ".", "ANGEE_ROOT containing angee.yaml")
	cmd.PersistentFlags().StringVar(&operatorURL, "operator", os.Getenv("ANGEE_OPERATOR_URL"), "operator URL for HTTP mode")
	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "write JSON output")

	cmd.AddCommand(versionCommand(stdout))
	cmd.AddCommand(initCommand(stdout, &root, &operatorURL))
	cmd.AddCommand(stackCommand(stdout, &root, &operatorURL))
	cmd.AddCommand(statusCommand(stdout, &root, &operatorURL, &jsonOutput))
	cmd.AddCommand(runtimeCommands(stdout, &root, &operatorURL)...)
	cmd.AddCommand(serviceCommand(stdout, &root, &operatorURL, &jsonOutput))
	cmd.AddCommand(jobCommand(stdout, &root, &operatorURL, &jsonOutput))
	cmd.AddCommand(sourceCommand(stdout, &root, &operatorURL, &jsonOutput))
	cmd.AddCommand(workspaceCommand(stdout, &root, &operatorURL, &jsonOutput))
	cmd.AddCommand(internalCommand(stdout, &root, &operatorURL, &jsonOutput))
	cmd.AddCommand(operatorCommand(stdout, stderr))
	return cmd
}

func initCommand(stdout io.Writer, root, operatorURL *string) *cobra.Command {
	var dev bool
	var inputs []string
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a stack",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			template := "dev"
			if !dev {
				return fmt.Errorf("init requires --dev or use stack init <template>")
			}
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			parsedInputs, err := parseKeyValues(inputs)
			if err != nil {
				return err
			}
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.StackInit(cmd.Context(), template, path, parsedInputs); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "stack initialized")
			return err
		},
	}
	cmd.Flags().BoolVar(&dev, "dev", false, "use the dev stack template")
	cmd.Flags().StringArrayVar(&inputs, "input", nil, "template input K=V")
	return cmd
}

func stackCommand(stdout io.Writer, root, operatorURL *string) *cobra.Command {
	cmd := &cobra.Command{Use: "stack", Short: "Manage stack configuration"}
	var initInputs []string
	initCmd := &cobra.Command{
		Use:   "init <template> [path]",
		Short: "Initialize a stack from a template",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 2 {
				path = args[1]
			}
			inputs, err := parseKeyValues(initInputs)
			if err != nil {
				return err
			}
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.StackInit(cmd.Context(), args[0], path, inputs); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "stack initialized")
			return err
		},
	}
	initCmd.Flags().StringArrayVar(&initInputs, "input", nil, "template input K=V")
	cmd.AddCommand(initCmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update generated runtime files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.StackUpdate(cmd.Context()); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "stack updated")
			return err
		},
	})
	var purge bool
	destroyCmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy stack runtime resources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.StackDestroy(cmd.Context(), purge); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "stack destroyed")
			return err
		},
	}
	destroyCmd.Flags().BoolVar(&purge, "purge", false, "remove runtime state directories")
	cmd.AddCommand(destroyCmd)
	return cmd
}

func versionCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Angee version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(stdout, Version)
			return err
		},
	}
}

func runtimeCommands(stdout io.Writer, root, operatorURL *string) []*cobra.Command {
	var build bool
	upCmd := &cobra.Command{
		Use:   "up [service...]",
		Short: "Start container services",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.StackUp(cmd.Context(), args, build); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "container services started")
			return err
		},
	}
	upCmd.Flags().BoolVar(&build, "build", false, "build images before starting")

	buildCmd := &cobra.Command{
		Use:   "build [service...]",
		Short: "Build container service images",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.StackBuild(cmd.Context(), args); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "container images built")
			return err
		},
	}

	downCmd := &cobra.Command{
		Use:   "down",
		Short: "Stop runtime backends",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.StackDown(cmd.Context()); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "stack stopped")
			return err
		},
	}

	startCmd := serviceActionCommand(stdout, root, operatorURL, "start")
	stopCmd := serviceActionCommand(stdout, root, operatorURL, "stop")
	restartCmd := serviceActionCommand(stdout, root, operatorURL, "restart")

	var follow bool
	logsCmd := &cobra.Command{
		Use:   "logs [service...]",
		Short: "Show service logs",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			lines, err := platform.StackLogs(cmd.Context(), args, follow)
			if err != nil {
				return err
			}
			for line := range lines {
				if _, err := fmt.Fprint(stdout, line); err != nil {
					return err
				}
			}
			return nil
		},
	}
	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow logs")

	var devBuild bool
	devCmd := &cobra.Command{
		Use:   "dev",
		Short: "Run the local development stack",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.StackDev(cmd.Context(), devBuild); err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, "dev stack started")
			return err
		},
	}
	devCmd.Flags().BoolVar(&devBuild, "build", false, "build container images before starting")

	return []*cobra.Command{buildCmd, upCmd, devCmd, downCmd, startCmd, stopCmd, restartCmd, logsCmd}
}

func serviceActionCommand(stdout io.Writer, root, operatorURL *string, action string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <service>...",
		Short: action + " services",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			switch action {
			case "start":
				err = platform.ServiceStart(cmd.Context(), args)
			case "stop":
				err = platform.ServiceStop(cmd.Context(), args)
			case "restart":
				err = platform.ServiceRestart(cmd.Context(), args)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "services %s\n", actionPast(action))
			return err
		},
	}
}

func actionPast(action string) string {
	switch action {
	case "start":
		return "started"
	case "stop":
		return "stopped"
	case "restart":
		return "restarted"
	default:
		return action
	}
}

func serviceCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Manage services"}
	cmd.AddCommand(serviceInitCommand(stdout, root, operatorURL))
	cmd.AddCommand(serviceUpdateCommand(stdout, root, operatorURL))
	cmd.AddCommand(serviceDestroyCommand(stdout, root, operatorURL))
	cmd.AddCommand(serviceListCommand(stdout, root, operatorURL, jsonOutput))
	cmd.AddCommand(serviceActionCommand(stdout, root, operatorURL, "start"))
	cmd.AddCommand(serviceActionCommand(stdout, root, operatorURL, "stop"))
	cmd.AddCommand(serviceActionCommand(stdout, root, operatorURL, "restart"))
	cmd.AddCommand(serviceLogsCommand(stdout, root, operatorURL))
	return cmd
}

func serviceLogsCommand(stdout io.Writer, root, operatorURL *string) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Show service logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			logs, err := platform.StackLogs(cmd.Context(), args, follow)
			if err != nil {
				return err
			}
			for line := range logs {
				if _, err := fmt.Fprint(stdout, line); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow logs")
	return cmd
}

func jobCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{Use: "job", Short: "Manage jobs"}
	cmd.AddCommand(jobListCommand(stdout, root, operatorURL, jsonOutput))
	cmd.AddCommand(jobRunCommand(stdout, root, operatorURL))
	cmd.AddCommand(&cobra.Command{
		Use:   "logs <name>",
		Short: "Show job logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("job logs are returned by job run")
		},
	})
	return cmd
}

func jobListCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List jobs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			jobs, err := platform.JobList(cmd.Context())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, jobs)
			}
			for _, job := range jobs {
				if _, err := fmt.Fprintf(stdout, "%s\t%s\n", job.Name, job.Runtime); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func jobRunCommand(stdout io.Writer, root, operatorURL *string) *cobra.Command {
	var inputValues []string
	cmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Run a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputs, err := parseKeyValues(inputValues)
			if err != nil {
				return err
			}
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			out, err := platform.JobRun(cmd.Context(), args[0], inputs)
			if len(out) > 0 {
				if _, writeErr := stdout.Write(out); writeErr != nil {
					return writeErr
				}
			}
			return err
		},
	}
	cmd.Flags().StringArrayVar(&inputValues, "input", nil, "job input K=V")
	return cmd
}

func serviceInitCommand(stdout io.Writer, root, operatorURL *string) *cobra.Command {
	var req api.ServiceInitRequest
	var env []string
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Add a service to angee.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Name = args[0]
			parsedEnv, err := parseKeyValues(env)
			if err != nil {
				return err
			}
			req.Env = parsedEnv
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.ServiceInit(cmd.Context(), req); err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "service %s added\n", req.Name)
			return err
		},
	}
	bindServiceFlags(cmd, &req, &env)
	cmd.Flags().BoolVar(&req.Start, "start", false, "start service after adding it")
	return cmd
}

func serviceUpdateCommand(stdout io.Writer, root, operatorURL *string) *cobra.Command {
	var req api.ServiceInitRequest
	var env []string
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a service in angee.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Name = args[0]
			if len(env) > 0 {
				parsedEnv, err := parseKeyValues(env)
				if err != nil {
					return err
				}
				req.Env = parsedEnv
			}
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.ServiceUpdate(cmd.Context(), req); err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "service %s updated\n", req.Name)
			return err
		},
	}
	bindServiceFlags(cmd, &req, &env)
	return cmd
}

func serviceDestroyCommand(stdout io.Writer, root, operatorURL *string) *cobra.Command {
	var stop bool
	cmd := &cobra.Command{
		Use:   "destroy <name>",
		Short: "Remove a service from angee.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.ServiceDestroy(cmd.Context(), args[0], stop); err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "service %s removed\n", args[0])
			return err
		},
	}
	cmd.Flags().BoolVar(&stop, "stop", true, "stop the service before removing it")
	return cmd
}

func serviceListCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List services",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			services, err := platform.ServiceList(cmd.Context())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, services)
			}
			for _, service := range services {
				if _, err := fmt.Fprintf(stdout, "%s\t%s\t%s\n", service.Name, service.Runtime, service.Status); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func bindServiceFlags(cmd *cobra.Command, req *api.ServiceInitRequest, env *[]string) {
	cmd.Flags().StringVar(&req.Runtime, "runtime", "", "service runtime: container or local")
	cmd.Flags().StringVar(&req.Image, "image", "", "container image")
	cmd.Flags().StringArrayVar(&req.Command, "command", nil, "command argument, repeat for each arg")
	cmd.Flags().StringArrayVar(&req.Mounts, "mount", nil, "mount URI")
	cmd.Flags().StringArrayVar(env, "env", nil, "environment variable K=V")
	cmd.Flags().StringArrayVar(&req.Ports, "port", nil, "port mapping")
	cmd.Flags().StringVar(&req.Workdir, "workdir", "", "working directory URI or path")
}

func parseKeyValues(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("expected K=V, got %q", value)
		}
		out[key] = val
	}
	return out, nil
}

func localPlatform(root, operatorURL *string) (*service.Platform, error) {
	if operatorURL != nil && *operatorURL != "" {
		return nil, fmt.Errorf("HTTP operator mode is not wired yet")
	}
	return service.New(*root)
}

func sourceCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{Use: "source", Short: "Manage sources"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			sources, err := platform.SourceList(cmd.Context())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, sources)
			}
			for _, source := range sources {
				exists := "missing"
				if source.Exists {
					exists = "ready"
				}
				if _, err := fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", source.Name, source.Kind, exists, source.Path); err != nil {
					return err
				}
			}
			return nil
		},
	})
	cmd.AddCommand(sourceOneCommand(stdout, root, operatorURL, jsonOutput, "fetch"))
	cmd.AddCommand(sourceOneCommand(stdout, root, operatorURL, jsonOutput, "status"))
	cmd.AddCommand(sourceOneCommand(stdout, root, operatorURL, jsonOutput, "pull"))
	cmd.AddCommand(sourcePushCommand(stdout, root, operatorURL, jsonOutput))
	return cmd
}

func sourceOneCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool, action string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <name>",
		Short: action + " a source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			var state api.SourceState
			switch action {
			case "fetch":
				state, err = platform.SourceFetch(cmd.Context(), args[0])
			case "status":
				state, err = platform.SourceStatus(cmd.Context(), args[0])
			case "pull":
				state, err = platform.SourcePull(cmd.Context(), args[0])
			}
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, state)
			}
			exists := "missing"
			if state.Exists {
				exists = "ready"
			}
			_, err = fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", state.Name, state.Kind, exists, state.Path)
			return err
		},
	}
}

func sourcePushCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	var ref string
	cmd := &cobra.Command{
		Use:   "push <name>",
		Short: "push a source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			state, err := platform.SourcePush(cmd.Context(), args[0], ref)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, state)
			}
			_, err = fmt.Fprintf(stdout, "%s\t%s\tready\t%s\n", state.Name, state.Kind, state.Path)
			return err
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "ref to push")
	return cmd
}

func workspaceCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{Use: "workspace", Short: "Manage workspaces"}
	cmd.AddCommand(workspaceCreateCommand(stdout, root, operatorURL, jsonOutput))
	cmd.AddCommand(workspaceUpdateCommand(stdout, root, operatorURL, jsonOutput))
	cmd.AddCommand(workspaceListCommand(stdout, root, operatorURL, jsonOutput))
	cmd.AddCommand(workspaceGetCommand(stdout, root, operatorURL, jsonOutput))
	cmd.AddCommand(workspaceDestroyCommand(stdout, root, operatorURL))
	cmd.AddCommand(workspaceLogsCommand(stdout, root, operatorURL))
	cmd.AddCommand(workspaceGitCommand(stdout, root, operatorURL, jsonOutput))
	cmd.AddCommand(workspacePushCommand(stdout, root, operatorURL, jsonOutput))
	cmd.AddCommand(workspaceLifecycleCommand(stdout, root, operatorURL, "start"))
	cmd.AddCommand(workspaceLifecycleCommand(stdout, root, operatorURL, "stop"))
	cmd.AddCommand(workspaceLifecycleCommand(stdout, root, operatorURL, "restart"))
	return cmd
}

func workspaceUpdateCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	var ttl string
	var inputValues []string
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update workspace metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputs, err := parseKeyValues(inputValues)
			if err != nil {
				return err
			}
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			ref, err := platform.WorkspaceUpdate(cmd.Context(), args[0], inputs, ttl)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, ref)
			}
			_, err = fmt.Fprintf(stdout, "workspace %s updated\n", ref.Name)
			return err
		},
	}
	cmd.Flags().StringVar(&ttl, "ttl", "", "workspace TTL")
	cmd.Flags().StringArrayVar(&inputValues, "input", nil, "workspace input K=V")
	return cmd
}

func workspaceLogsCommand(stdout io.Writer, root, operatorURL *string) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Show workspace logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			logs, err := platform.WorkspaceLogs(cmd.Context(), args[0], follow)
			if err != nil {
				return err
			}
			for line := range logs {
				if _, err := fmt.Fprint(stdout, line); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow logs")
	return cmd
}

func workspaceGitCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "git <name>",
		Short: "Show workspace git status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			states, err := platform.WorkspaceGitStatus(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, states)
			}
			for _, state := range states {
				dirty := "clean"
				if state.Dirty {
					dirty = "dirty"
				}
				if _, err := fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", state.Slot, state.Ref, dirty, state.Path); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func workspacePushCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	var ref string
	cmd := &cobra.Command{
		Use:   "push <name>",
		Short: "Push workspace git sources",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			states, err := platform.WorkspacePush(cmd.Context(), args[0], ref)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, states)
			}
			for _, state := range states {
				if _, err := fmt.Fprintf(stdout, "%s\t%s\t%s\n", state.Slot, state.Ref, state.Path); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "ref to push")
	return cmd
}

func workspaceCreateCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	var req api.WorkspaceCreateRequest
	var inputs []string
	cmd := &cobra.Command{
		Use:   "create <template>",
		Short: "Create a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Template = args[0]
			parsedInputs, err := parseKeyValues(inputs)
			if err != nil {
				return err
			}
			req.Inputs = parsedInputs
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			ref, err := platform.WorkspaceCreate(cmd.Context(), req)
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, ref)
			}
			_, err = fmt.Fprintf(stdout, "workspace %s created at %s\n", ref.Name, ref.Path)
			return err
		},
	}
	cmd.Flags().StringArrayVar(&inputs, "input", nil, "template input K=V")
	cmd.Flags().StringVar(&req.Name, "name", "", "workspace name")
	cmd.Flags().StringVar(&req.TTL, "ttl", "", "workspace TTL")
	cmd.Flags().BoolVar(&req.Start, "start", false, "start workspace after creating it")
	return cmd
}

func workspaceListCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List workspaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			refs, err := platform.WorkspaceList(cmd.Context())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, refs)
			}
			for _, ref := range refs {
				if _, err := fmt.Fprintf(stdout, "%s\t%s\t%s\n", ref.Name, ref.Template, ref.Path); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func workspaceGetCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			ref, err := platform.WorkspaceGet(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, ref)
			}
			_, err = fmt.Fprintf(stdout, "%s\t%s\t%s\n", ref.Name, ref.Template, ref.Path)
			return err
		},
	}
}

func workspaceDestroyCommand(stdout io.Writer, root, operatorURL *string) *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "destroy <name>",
		Short: "Destroy a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			if err := platform.WorkspaceDestroy(cmd.Context(), args[0], purge); err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "workspace %s destroyed\n", args[0])
			return err
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "remove workspace files")
	return cmd
}

func workspaceLifecycleCommand(stdout io.Writer, root, operatorURL *string, action string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <name>",
		Short: action + " workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform, err := localPlatform(root, operatorURL)
			if err != nil {
				return err
			}
			switch action {
			case "start":
				err = platform.WorkspaceStart(cmd.Context(), args[0])
			case "stop":
				err = platform.WorkspaceStop(cmd.Context(), args[0])
			case "restart":
				if err = platform.WorkspaceStop(cmd.Context(), args[0]); err == nil {
					err = platform.WorkspaceStart(cmd.Context(), args[0])
				}
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "workspace %s %s\n", args[0], actionPast(action))
			return err
		},
	}
}

func statusCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show declared stack state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if *operatorURL != "" {
				return fmt.Errorf("HTTP operator mode is not wired yet")
			}
			platform, err := service.New(*root)
			if err != nil {
				return err
			}
			status, err := platform.StackStatus(cmd.Context())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, status)
			}
			_, err = fmt.Fprintf(stdout, "%s\nroot: %s\nservices: %d\njobs: %d\nworkspaces: %d\n", status.Name, status.Root, len(status.Services), len(status.Jobs), len(status.Workspaces))
			return err
		},
	}
}

func internalCommand(stdout io.Writer, root, operatorURL *string, jsonOutput *bool) *cobra.Command {
	internalCmd := &cobra.Command{
		Use:    "internal",
		Short:  "Internal development commands",
		Hidden: true,
	}
	stackCmd := &cobra.Command{Use: "stack", Short: "Internal stack commands"}
	stackCmd.AddCommand(&cobra.Command{
		Use:   "compile",
		Short: "Compile runtime backend files without starting processes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if *operatorURL != "" {
				return fmt.Errorf("HTTP operator mode is not wired yet")
			}
			platform, err := service.New(*root)
			if err != nil {
				return err
			}
			compiled, err := platform.StackCompile(cmd.Context())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, compiled)
			}
			text, err := compiled.Text()
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(stdout, text)
			return err
		},
	})
	stackCmd.AddCommand(&cobra.Command{
		Use:   "prepare",
		Short: "Compile and write runtime backend files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if *operatorURL != "" {
				return fmt.Errorf("HTTP operator mode is not wired yet")
			}
			platform, err := service.New(*root)
			if err != nil {
				return err
			}
			compiled, err := platform.StackPrepare(cmd.Context())
			if err != nil {
				return err
			}
			if *jsonOutput {
				return writeJSON(stdout, compiled)
			}
			_, err = fmt.Fprintln(stdout, "runtime files prepared")
			return err
		},
	})
	internalCmd.AddCommand(stackCmd)
	return internalCmd
}

func operatorCommand(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "operator",
		Short: "Run the Angee operator",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return operator.Execute(cmd.Context(), args, stdout, stderr)
		},
	}
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func WithContext(cmd *cobra.Command, ctx context.Context) *cobra.Command {
	cmd.SetContext(ctx)
	return cmd
}
