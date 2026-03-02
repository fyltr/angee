package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fyltr/angee/internal/config"
	"github.com/spf13/cobra"
)

var destroyForce bool

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy the stack and clean up all resources",
	Long: `Stop all containers, remove volumes, and delete ANGEE_ROOT.

This is a destructive operation that cannot be undone.
By default it prompts for confirmation unless --force is passed.

Example:
  angee destroy
  angee destroy --force`,
	RunE: runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVarP(&destroyForce, "force", "f", false, "Skip confirmation prompt")
}

func runDestroy(cmd *cobra.Command, args []string) error {
	path := resolveRoot()

	fmt.Printf("\n\033[1mangee destroy\033[0m\n\n")

	// Confirm unless --force
	if !destroyForce {
		printInfo(fmt.Sprintf("This will destroy the stack at %s", path))
		printInfo("All containers, volumes, and configuration will be removed.")
		fmt.Print("\n  Type 'yes' to confirm: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) != "yes" {
				fmt.Println("  Aborted.")
				return nil
			}
		}
		fmt.Println()
	}

	// 1. Stop containers and remove volumes if docker-compose.yaml exists
	composePath := filepath.Join(path, "docker-compose.yaml")
	if _, err := os.Stat(composePath); err == nil {
		projectName := "angee"
		cfgPath := filepath.Join(path, "angee.yaml")
		if cfg, err := config.Load(cfgPath); err == nil && cfg.Name != "" {
			projectName = cfg.Name
		}

		printInfo("Stopping containers and removing volumes...")
		if err := runDockerCompose(composePath, path, projectName, "down", "--volumes", "--remove-orphans"); err != nil {
			printError(fmt.Sprintf("docker compose down: %v", err))
			// Continue with cleanup even if compose fails
		} else {
			printSuccess("Containers and volumes removed")
		}
	} else {
		printInfo("No docker-compose.yaml found — skipping container cleanup")
	}

	// 2. Remove ANGEE_ROOT
	printInfo(fmt.Sprintf("Removing %s ...", path))
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	printSuccess("ANGEE_ROOT removed")

	fmt.Printf("\n  Stack destroyed. Run 'angee init' to start fresh.\n\n")
	return nil
}
