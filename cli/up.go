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
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the Angee platform",
	Long: `Start the Angee platform by compiling angee.yaml into a single docker-compose
stack and running it. All services (operator, traefik, app, DB, etc.) are
defined in angee.yaml — there is no separate system stack.

Example:
  angee up`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
	path := resolveRoot()

	// Load angee.yaml
	angeeYAMLPath := filepath.Join(path, "angee.yaml")
	cfg, err := config.Load(angeeYAMLPath)
	if err != nil {
		return fmt.Errorf("loading angee.yaml: %w (run 'angee init' first)", err)
	}

	fmt.Printf("\n\033[1mangee up\033[0m\n\n")

	// Compile angee.yaml → docker-compose.yaml
	c := compiler.New(path, "angee-net")
	cf, err := c.Compile(cfg)
	if err != nil {
		return fmt.Errorf("compiling angee.yaml: %w", err)
	}

	composePath := filepath.Join(path, "docker-compose.yaml")
	if err := compiler.Write(cf, composePath); err != nil {
		return fmt.Errorf("writing docker-compose.yaml: %w", err)
	}
	printSuccess("Compiled angee.yaml → docker-compose.yaml")

	// Use project name from angee.yaml
	projectName := cfg.Name
	if projectName == "" {
		projectName = "angee"
	}

	// Check if operator is already running
	if isOperatorRunning() {
		printInfo("Operator already running")
	} else {
		printInfo("Starting stack...")
		if err := runDockerCompose(composePath, path, projectName, "up", "-d", "--remove-orphans"); err != nil {
			return fmt.Errorf("starting stack: %w", err)
		}

		// Wait for operator
		printInfo("Waiting for operator...")
		if err := waitForOperator(30 * time.Second); err != nil {
			return fmt.Errorf("operator did not start: %w", err)
		}
		printSuccess("Operator started (port 9000)")
	}

	// Trigger initial deploy via operator
	printInfo("Deploying angee.yaml...")
	if err := triggerDeploy(); err != nil {
		printError(fmt.Sprintf("deploy failed: %s (run 'angee logs' to debug)", err))
	} else {
		printSuccess("Services deployed")
	}

	fmt.Printf("\n\033[1mPlatform ready:\033[0m\n\n")
	printInfo("UI        →  \033[4mhttp://localhost:3333\033[0m")
	printInfo("API       →  \033[4mhttp://localhost:8000/api\033[0m")
	printInfo("Operator  →  \033[4mhttp://localhost:9000\033[0m")
	fmt.Println()
	printInfo("angee ls          View agents and services")
	printInfo("angee admin       Chat with admin agent")
	printInfo("angee develop     Chat with developer agent")
	fmt.Println()

	return nil
}

func isOperatorRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", resolveOperator()+"/health", nil)
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
	req, err := http.NewRequestWithContext(ctx, "POST", resolveOperator()+"/deploy", nil)
	if err != nil {
		return err
	}
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
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
