package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fyltr/angee/internal/compiler"
	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/root"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update template, agents, and skills",
	Long: `Update the platform components.

Subcommands:
  angee update template  Re-fetch and re-render the template
  angee update agents    Pull latest images and restart all services`,
}

var updateTemplateCmd = &cobra.Command{
	Use:   "template",
	Short: "Re-fetch and re-render the template",
	Long: `Re-fetch the template used during 'angee init', re-render angee.yaml.tmpl
with current parameters, and show a diff against the current angee.yaml.

The template source URL is stored in operator.yaml during init.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("angee update template is not yet implemented")
		fmt.Println("This will re-fetch the template from the source stored in operator.yaml,")
		fmt.Println("re-render angee.yaml.tmpl, and show a diff for review.")
		return nil
	},
}

var updateAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Pull latest images and restart",
	Long: `Pull the latest container images for all services and agents defined in
angee.yaml, then restart the stack so updated images take effect.

Equivalent to: docker compose pull && angee up`,
	RunE: runUpdateAgents,
}

var updateSkillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Update skill definitions",
	Long:  `Update skill definitions from a skill registry. (Placeholder for future implementation.)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("angee update skills is not yet implemented")
		fmt.Println("This will update skill definitions from a skill registry.")
		return nil
	},
}

func runUpdateAgents(cmd *cobra.Command, args []string) error {
	path := resolveRoot()

	// Load angee.yaml
	cfg, err := config.Load(filepath.Join(path, "angee.yaml"))
	if err != nil {
		return fmt.Errorf("loading angee.yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	projectName := cfg.Name
	if projectName == "" {
		projectName = "angee"
	}

	r, err := root.Open(path)
	if err != nil {
		return fmt.Errorf("opening ANGEE_ROOT: %w", err)
	}

	fmt.Printf("\n\033[1mangee update agents\033[0m\n\n")

	// Ensure agent directories + render config file templates
	for agentName, agent := range cfg.Agents {
		if err := r.EnsureAgentDir(agentName); err != nil {
			return fmt.Errorf("creating agent dir %s: %w", agentName, err)
		}
		if err := compiler.RenderAgentFiles(path, r.AgentDir(agentName), agent, cfg.MCPServers); err != nil {
			return fmt.Errorf("agent files for %s: %w", agentName, err)
		}
	}

	// Compile angee.yaml → docker-compose.yaml
	opCfg, err := r.LoadOperatorConfig()
	if err != nil {
		return fmt.Errorf("loading operator config: %w", err)
	}
	comp := compiler.New(path, opCfg.Docker.Network)
	if opCfg.APIKey != "" {
		comp.APIKey = opCfg.APIKey
	} else if key := os.Getenv("ANGEE_API_KEY"); key != "" {
		comp.APIKey = key
	}
	cf, err := comp.Compile(cfg)
	if err != nil {
		return fmt.Errorf("compiling: %w", err)
	}
	composePath := filepath.Join(path, "docker-compose.yaml")
	if err := compiler.Write(cf, composePath); err != nil {
		return fmt.Errorf("writing docker-compose.yaml: %w", err)
	}
	printSuccess("Compiled docker-compose.yaml")

	// Pull latest images
	printInfo("Pulling latest images...")
	if err := runDockerCompose(composePath, path, projectName, "pull"); err != nil {
		fmt.Printf("  \033[33m!\033[0m  Some images failed to pull (continuing anyway)\n")
	} else {
		printSuccess("Pulled latest images")
	}

	// Restart everything with new images
	printInfo("Restarting stack...")
	if err := runDockerCompose(composePath, path, projectName, "up", "-d", "--remove-orphans"); err != nil {
		fmt.Printf("  \033[33m!\033[0m  Some containers failed to start (run 'angee logs' to investigate)\n")
	} else {
		printSuccess("Stack restarted with updated images")
	}

	fmt.Println()
	return nil
}

func init() {
	updateCmd.AddCommand(updateTemplateCmd, updateAgentsCmd, updateSkillsCmd)
}
