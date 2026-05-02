package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fyltr/angee/internal/config"
	"github.com/spf13/cobra"
)

// agentNameRE constrains agent names to docker-safe characters. Without this
// guard, a user-supplied name flows into a docker container name string.
// Docker rejects containers with `/` etc., but constraining at the CLI gives
// a clearer error.
var agentNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

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

// developCmd is a shortcut for `angee chat developer`.
//
// Note: `dev` is intentionally NOT an alias here — it's claimed by the
// project-mode orchestrator (cli/dev.go). `develop` and `developer` work.
var developCmd = &cobra.Command{
	Use:     "develop",
	Aliases: []string{"developer"},
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

// attachToAgent runs docker exec -it to connect to the agent's opencode server.
func attachToAgent(agentName string) error {
	if !agentNameRE.MatchString(agentName) {
		return fmt.Errorf("invalid agent name %q: must match %s", agentName, agentNameRE.String())
	}
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

	fmt.Printf("\n\033[1mAttaching to %s agent\033[0m (%s)\n\n", agentName, containerName)

	// Attach to the opencode server running inside the container
	cmd := exec.Command("docker", "exec", "-it", containerName,
		"opencode", "attach", "http://localhost:4096")
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
	if !agentNameRE.MatchString(askAgent) {
		return fmt.Errorf("invalid agent name %q: must match %s", askAgent, agentNameRE.String())
	}
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

	// Run a one-shot message via opencode run
	execCmd := exec.Command("docker", "exec", containerName,
		"opencode", "run", message)
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

// loadProjectName parses angee.yaml properly and returns the top-level name.
// The previous hand-rolled parser matched any indented `name:` key (e.g. inside
// a service definition), giving wrong project names.
func loadProjectName(rootPath string) (string, error) {
	cfg, err := config.Load(filepath.Join(rootPath, "angee.yaml"))
	if err != nil {
		return "", err
	}
	return cfg.Name, nil
}
