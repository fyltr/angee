package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
	fmt.Printf("\n\033[1mangee up\033[0m\n\n")

	composePath, projectName, err := localCompile(path)
	if err != nil {
		return err
	}
	printSuccess("Compiled docker-compose.yaml")

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
