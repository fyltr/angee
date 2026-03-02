package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// agentContainerNames maps CLI shorthand → docker compose service name.
var agentContainerNames = map[string]string{
	"admin":     "agent-angee-admin",
	"developer": "agent-angee-developer",
	"dev":       "agent-angee-developer",
}

var chatCmd = &cobra.Command{
	Use:   "chat [agent]",
	Short: "Attach to an agent's terminal (default: admin)",
	Long: `Attach to an agent's interactive terminal.

The agent (Claude Code, OpenCode, etc.) is already running inside the
container. This command connects your terminal to the running session
via docker attach. Use Ctrl-p Ctrl-q to detach without stopping the agent.

Examples:
  angee chat              # attach to admin agent
  angee chat developer    # attach to developer agent
  angee chat my-agent     # attach to any named agent`,
	RunE: func(cmd *cobra.Command, args []string) error {
		agent := "admin"
		if len(args) > 0 {
			agent = args[0]
		}
		return attachToAgent(agent)
	},
}

// adminCmd is a shortcut for `angee chat admin`
var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Attach to the admin agent's terminal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return attachToAgent("admin")
	},
}

// developCmd is a shortcut for `angee chat developer`
var developCmd = &cobra.Command{
	Use:     "develop",
	Aliases: []string{"developer", "dev"},
	Short:   "Attach to the developer agent's terminal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return attachToAgent("developer")
	},
}

// askCmd sends a single message to an agent (non-interactive).
var askCmd = &cobra.Command{
	Use:   "ask [message]",
	Short: "Send a one-shot message to the admin agent",
	Long: `Send a single message to an agent by exec-ing a command in the container.

Examples:
  angee ask "show me the project status"
  angee ask --agent developer "run the tests"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runAsk,
}

var askAgent string

func init() {
	askCmd.Flags().StringVar(&askAgent, "agent", "admin", "Agent to send the message to")
}

// attachToAgent connects to the agent's interactive terminal.
//
// The strategy depends on how the agent runs:
//   - Interactive PID 1 (e.g. claude): docker attach to the running process
//   - Server PID 1 (e.g. opencode serve): docker exec a client into the container
func attachToAgent(agentName string) error {
	containerService := resolveAgentService(agentName)
	projectName := resolveProjectName()
	containerName := fmt.Sprintf("%s-%s-1", projectName, containerService)

	// Check if the container is running
	checkCmd := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", containerName)
	out, err := checkCmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("agent %q is not running (container: %s) — start with 'angee up'", agentName, containerName)
	}

	fmt.Printf("\n\033[1mAttaching to %s agent\033[0m (%s)\n", agentName, containerName)

	// Detect the agent runtime from the container's entrypoint/cmd.
	runtime := detectAgentRuntime(containerName)

	var cmd *exec.Cmd
	switch runtime {
	case "opencode":
		// OpenCode runs as a server; exec a client to attach.
		cmd = exec.Command("docker", "exec", "-it", containerName,
			"opencode", "attach", "http://localhost:4096")
	default:
		// Interactive agent (claude, etc.): attach directly to PID 1.
		fmt.Println("  Use Ctrl-p Ctrl-q to detach.")
		cmd = exec.Command("docker", "attach", containerName)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("attaching to agent: %w", err)
	}

	fmt.Println("\n  Detached from agent.")
	return nil
}

func runAsk(cmd *cobra.Command, args []string) error {
	message := strings.Join(args, " ")
	containerService := resolveAgentService(askAgent)
	projectName := resolveProjectName()
	containerName := fmt.Sprintf("%s-%s-1", projectName, containerService)

	// Check if the container is running
	checkCmd := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", containerName)
	out, err := checkCmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("agent %q is not running — start with 'angee up'", askAgent)
	}

	// Detect runtime to pick the right one-shot command.
	runtime := detectAgentRuntime(containerName)

	var execCmd *exec.Cmd
	switch runtime {
	case "opencode":
		execCmd = exec.Command("docker", "exec", containerName, "opencode", "run", message)
	default:
		execCmd = exec.Command("docker", "exec", "-w", "/workspace", containerName, "claude", "-p", message)
	}

	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	return execCmd.Run()
}

// detectAgentRuntime inspects the container's entrypoint and command to
// determine which agent runtime is running. Returns "opencode", "claude",
// or "" (unknown/fallback to docker attach).
func detectAgentRuntime(containerName string) string {
	out, err := exec.Command("docker", "inspect", "--format",
		"{{.Config.Entrypoint}} {{.Config.Cmd}}", containerName).Output()
	if err != nil {
		return ""
	}
	s := strings.ToLower(strings.TrimSpace(string(out)))
	if strings.Contains(s, "opencode") {
		return "opencode"
	}
	if strings.Contains(s, "claude") {
		return "claude"
	}
	return ""
}

// resolveAgentService maps an agent name to its docker compose service name.
func resolveAgentService(name string) string {
	if svc, ok := agentContainerNames[name]; ok {
		return svc
	}
	// For custom agents, use the standard prefix
	return "agent-" + name
}

// resolveProjectName gets the docker compose project name from angee.yaml.
func resolveProjectName() string {
	path := resolveRoot()
	cfg, err := loadProjectName(path)
	if err != nil || cfg == "" {
		return "angee"
	}
	return cfg
}

// loadProjectName reads just the name field from angee.yaml.
func loadProjectName(rootPath string) (string, error) {
	cfgPath := rootPath + "/angee.yaml"
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", err
	}
	// Quick parse — just find the name field
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "name:")), nil
		}
	}
	return "", nil
}
