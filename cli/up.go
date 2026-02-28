package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fyltr/angee-go/internal/compiler"
	"github.com/fyltr/angee-go/internal/config"
	"github.com/fyltr/angee-go/internal/root"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the Angee platform",
	Long: `Start the Angee platform using the docker-compose.yaml compiled during
init. Once the operator is healthy, the CLI triggers a deploy so the
operator takes ownership of the compose file from that point on.

Example:
  angee up`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
	path := resolveRoot()

	// Verify required files exist
	composePath := filepath.Join(path, "docker-compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.yaml not found at %s — run 'angee init' first", path)
	}

	// Load angee.yaml for the project name
	cfg, err := config.Load(filepath.Join(path, "angee.yaml"))
	if err != nil {
		return fmt.Errorf("loading angee.yaml: %w", err)
	}
	projectName := cfg.Name
	if projectName == "" {
		projectName = "angee"
	}

	// Ensure agent directories + stub .env files exist before docker compose
	// tries to mount them (env_file references must exist at compose-up time).
	r, err := root.Open(path)
	if err != nil {
		return fmt.Errorf("opening ANGEE_ROOT: %w", err)
	}
	for agentName, agent := range cfg.Agents {
		if err := r.EnsureAgentDir(agentName); err != nil {
			return fmt.Errorf("creating agent dir %s: %w", agentName, err)
		}
		if err := compiler.RenderAgentFiles(path, r.AgentDir(agentName), agent, cfg.MCPServers); err != nil {
			return fmt.Errorf("agent files for %s: %w", agentName, err)
		}
	}

	fmt.Printf("\n\033[1mangee up\033[0m\n\n")

	// If operator is already running, just trigger deploy
	if isOperatorRunning() {
		printInfo("Operator already running — triggering deploy")
		if err := triggerDeploy(); err != nil {
			printError(fmt.Sprintf("deploy failed: %s (run 'angee logs' to debug)", err))
		} else {
			printSuccess("Services deployed")
		}
		printPlatformReady()
		return nil
	}

	// Start the stack using the compose file compiled during init.
	// docker compose up -d returns non-zero if ANY container fails,
	// even when most services started fine. Warn instead of aborting.
	printInfo("Starting stack...")
	if err := runDockerCompose(composePath, path, projectName, "up", "-d", "--remove-orphans"); err != nil {
		fmt.Printf("  \033[33m!\033[0m  Some containers failed to start (run 'angee logs' to investigate)\n")
	}

	// Wait for operator
	printInfo("Waiting for operator...")
	if err := waitForOperator(30 * time.Second); err != nil {
		return fmt.Errorf("operator did not start: %w", err)
	}
	printSuccess("Operator started (port 9000)")

	// Hand off to the operator: trigger deploy so it re-compiles and
	// reconciles via its RuntimeBackend (--env-file, --pull, agent dirs, etc.)
	printInfo("Deploying angee.yaml via operator...")
	if err := triggerDeploy(); err != nil {
		printError(fmt.Sprintf("deploy failed: %s (run 'angee logs' to debug)", err))
	} else {
		printSuccess("Services deployed")
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

func isOperatorRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := newRequest("GET", resolveOperator()+"/health", nil)
	if err != nil {
		return false
	}
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func waitForOperator(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isOperatorRunning() {
			return nil
		}
		time.Sleep(1 * time.Second)
		fmt.Print(".")
	}
	fmt.Println()
	return fmt.Errorf("timeout waiting for operator after %s", timeout)
}

func triggerDeploy() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := newRequest("POST", resolveOperator()+"/deploy", nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("operator returned %d", resp.StatusCode)
	}
	return nil
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
