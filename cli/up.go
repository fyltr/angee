package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/fyltr/angee-go/internal/compiler"
	"github.com/fyltr/angee-go/internal/config"
	"github.com/fyltr/angee-go/internal/root"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the Angee platform",
	Long: `Compile angee.yaml → docker-compose.yaml, render agent config files,
and run docker compose up. Changed services are automatically recreated.

Example:
  angee up`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("\n\033[1mangee up\033[0m\n\n")

	// Ensure agent directories + render config file templates
	for agentName, agent := range cfg.Agents {
		if err := r.EnsureAgentDir(agentName); err != nil {
			return fmt.Errorf("creating agent dir %s: %w", agentName, err)
		}
		if err := compiler.RenderAgentFiles(path, r.AgentDir(agentName), agent, cfg.MCPServers); err != nil {
			return fmt.Errorf("agent files for %s: %w", agentName, err)
		}
	}
	printSuccess("Rendered agent config files")

	// Compile angee.yaml → docker-compose.yaml
	opCfg, err := r.LoadOperatorConfig()
	if err != nil {
		return fmt.Errorf("loading operator config: %w", err)
	}
	comp := compiler.New(path, opCfg.Docker.Network)
	cf, err := comp.Compile(cfg)
	if err != nil {
		return fmt.Errorf("compiling: %w", err)
	}
	composePath := filepath.Join(path, "docker-compose.yaml")
	if err := compiler.Write(cf, composePath); err != nil {
		return fmt.Errorf("writing docker-compose.yaml: %w", err)
	}
	printSuccess("Compiled docker-compose.yaml")

	// docker compose up -d — detects changes and recreates affected services.
	printInfo("Starting stack...")
	if err := runDockerCompose(composePath, path, projectName, "up", "-d", "--remove-orphans"); err != nil {
		fmt.Printf("  \033[33m!\033[0m  Some containers failed to start (run 'angee logs' to investigate)\n")
	} else {
		printSuccess("Stack started")
	}

	printPlatformReady()
	return nil
}

func printPlatformReady() {
	fmt.Printf("\n\033[1mPlatform ready:\033[0m\n\n")
	printInfo("UI        →  \033[4mhttp://localhost:3333\033[0m")
	printInfo("API       →  \033[4mhttp://localhost:8000/api\033[0m")
	printInfo("Operator  →  \033[4mhttp://localhost:9000\033[0m")
	fmt.Println()
	printInfo("angee ls          View agents and services")
	printInfo("angee admin       Chat with admin agent")
	printInfo("angee develop     Chat with developer agent")
	fmt.Println()
}

func runDockerCompose(composePath, projectDir, projectName string, args ...string) error {
	cmdArgs := []string{
		"compose",
		"--project-name", projectName,
		"--file", composePath,
		"--project-directory", projectDir,
	}
	// Pass .env file if it exists
	envFile := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envFile); err == nil {
		cmdArgs = append(cmdArgs, "--env-file", envFile)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
