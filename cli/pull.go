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

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull latest container images",
	Long: `Pull the latest container images for all services and agents defined in
angee.yaml. Does NOT restart — run 'angee restart' after pulling to apply.

Usage:
  angee pull              Pull latest images
  angee pull && angee restart`,
	RunE: runPull,
}

func runPull(cmd *cobra.Command, args []string) error {
	path := resolveRoot()

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

	fmt.Printf("\n\033[1mangee pull\033[0m\n\n")

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

	// Pull latest images
	printInfo("Pulling latest images...")
	if err := runDockerCompose(composePath, path, projectName, "pull"); err != nil {
		fmt.Printf("  \033[33m!\033[0m  Some images failed to pull (this is normal for locally-built images)\n")
	} else {
		printSuccess("Pulled latest images")
	}

	fmt.Println()
	printInfo("Run 'angee restart' to apply updated images")
	fmt.Println()
	return nil
}
