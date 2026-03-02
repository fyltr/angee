package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fyltr/angee/internal/config"
	"github.com/spf13/cobra"
)

var destroyForce bool

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Stop all services and remove ANGEE_ROOT",
	Long: `Bring down all containers and optionally remove the ANGEE_ROOT directory.
This is destructive — all agent workspaces and local config will be deleted.

Examples:
  angee destroy           # stop stack, prompt before removing
  angee destroy --force   # stop stack and remove without prompting`,
	RunE: runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVar(&destroyForce, "force", false, "Skip confirmation prompt")
}

func runDestroy(cmd *cobra.Command, args []string) error {
	path := resolveRoot()

	fmt.Printf("\n\033[1mangee destroy\033[0m\n\n")

	// Try to bring down the stack first
	composePath := filepath.Join(path, "docker-compose.yaml")
	if _, err := os.Stat(composePath); err == nil {
		cfg, err := config.Load(filepath.Join(path, "angee.yaml"))
		projectName := "angee"
		if err == nil && cfg.Name != "" {
			projectName = cfg.Name
		}

		printInfo("Stopping stack...")
		if err := runDockerCompose(composePath, path, projectName, "down", "--volumes", "--remove-orphans"); err != nil {
			fmt.Printf("  \033[33m!\033[0m  Could not stop stack (it may not be running)\n")
		} else {
			printSuccess("Stack stopped and volumes removed")
		}
	}

	if !destroyForce {
		fmt.Printf("\n  Remove ANGEE_ROOT at %s? [y/N] ", path)
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			printInfo("Aborted. Stack is stopped but ANGEE_ROOT is preserved.")
			return nil
		}
	}

	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("removing ANGEE_ROOT: %w", err)
	}

	printSuccess(fmt.Sprintf("Removed %s", path))
	return nil
}
