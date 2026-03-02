package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart all services and agents",
	Long: `Stop and restart all services and agents. Recompiles angee.yaml,
stops the stack, then starts it fresh. Containers using updated images
(from 'angee pull') will be recreated automatically.

Usage:
  angee restart
  angee pull && angee restart`,
	RunE: runRestart,
}

func runRestart(cmd *cobra.Command, args []string) error {
	path := resolveRoot()
	fmt.Printf("\n\033[1mangee restart\033[0m\n\n")

	composePath, projectName, err := localCompile(path)
	if err != nil {
		return err
	}
	printSuccess("Compiled docker-compose.yaml")

	printInfo("Stopping stack...")
	if err := runDockerCompose(composePath, path, projectName, "down"); err != nil {
		fmt.Printf("  \033[33m!\033[0m  Stop encountered issues (continuing with restart)\n")
	} else {
		printSuccess("Stack stopped")
	}

	printInfo("Starting stack...")
	if err := runDockerCompose(composePath, path, projectName, "up", "-d", "--remove-orphans"); err != nil {
		fmt.Printf("  \033[33m!\033[0m  Some containers failed to start (run 'angee logs' to investigate)\n")
	} else {
		printSuccess("Stack restarted")
	}

	fmt.Println()
	return nil
}
