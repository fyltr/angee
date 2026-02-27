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
	Long: `Open an interactive terminal session inside an agent container.

This runs docker exec -it into the agent's container, giving you direct
terminal access. The agent can be running OpenCode, Claude Code, or any
terminal-based coding agent.

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

// attachToAgent runs docker exec -it into the agent's container.
func attachToAgent(agentName string) error {
	containerService := resolveAgentService(agentName)
	projectName := resolveProjectName()

	// Build the container name: <project>-<service>-1
	containerName := fmt.Sprintf("%s-%s-1", projectName, containerService)

	// Check if the container is running
	checkCmd := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", containerName)
	out, err := checkCmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("agent %q is not running (container: %s) — start with 'angee up'", agentName, containerName)
	}

	fmt.Printf("\n\033[1mAttaching to %s agent\033[0m (%s)\n", agentName, containerName)
	fmt.Printf("\033[90m(you are now inside the agent container — exit to detach)\033[0m\n\n")

	// docker exec -it <container> /bin/sh (use sh as universal fallback)
	cmd := exec.Command("docker", "exec", "-it", containerName, "/bin/sh")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Exit code from the shell is normal — don't treat as error
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

	// Execute the message as a command inside the container
	execCmd := exec.Command("docker", "exec", containerName, "sh", "-c", message)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	return execCmd.Run()
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
