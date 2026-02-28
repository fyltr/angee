package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fyltr/angee/api"
	"github.com/fyltr/angee/internal/config"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Preview what 'angee deploy' would change",
	Long:  `Show a diff of what would change if you ran 'angee deploy' right now.`,
	RunE:  runPlan,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show detailed platform status",
	RunE:  runStatus,
}

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop all services and agents",
	Long: `Stop and remove all containers in the stack.

Example:
  angee down`,
	RunE: runDown,
}

func runPlan(cmd *cobra.Command, args []string) error {
	if !isOperatorRunning() {
		return fmt.Errorf("operator not running — start with 'angee up'")
	}

	resp, err := doRequest("GET", resolveOperator()+"/plan", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if outputJSON {
		fmt.Println(string(body))
		return nil
	}

	var changeset api.ChangeSet
	json.Unmarshal(body, &changeset) //nolint:errcheck

	total := len(changeset.Add) + len(changeset.Update) + len(changeset.Remove)
	if total == 0 {
		fmt.Println("\n  No changes. Platform is up to date.")
		return nil
	}

	fmt.Println()
	for _, s := range changeset.Add {
		fmt.Printf("  \033[32m+ %s\033[0m\n", s)
	}
	for _, s := range changeset.Update {
		fmt.Printf("  \033[33m~ %s\033[0m\n", s)
	}
	for _, s := range changeset.Remove {
		fmt.Printf("  \033[31m- %s\033[0m\n", s)
	}
	fmt.Printf("\n  %d change(s). Run 'angee deploy' to apply.\n\n", total)
	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	return runLs(cmd, args)
}

func runDown(cmd *cobra.Command, args []string) error {
	path := resolveRoot()

	cfg, err := config.Load(filepath.Join(path, "angee.yaml"))
	if err != nil {
		return fmt.Errorf("loading angee.yaml: %w", err)
	}

	projectName := cfg.Name
	if projectName == "" {
		projectName = "angee"
	}

	composePath := filepath.Join(path, "docker-compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("no docker-compose.yaml found at %s — run 'angee up' first", path)
	}

	fmt.Printf("\n\033[1mangee down\033[0m\n\n")

	if err := runDockerCompose(composePath, path, projectName, "down"); err != nil {
		return fmt.Errorf("docker compose down: %w", err)
	}

	printSuccess("Stack stopped")
	return nil
}
