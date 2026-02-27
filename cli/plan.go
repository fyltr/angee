package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
	Long: `Stop the workspace stack (user services and agents).
The system stack (operator, Django, postgres, redis) continues running.
Use 'angee system down' to stop everything.`,
	RunE: runDown,
}

func runPlan(cmd *cobra.Command, args []string) error {
	if !isOperatorRunning() {
		return fmt.Errorf("operator not running — start with 'angee up'")
	}

	resp, err := http.Get(resolveOperator() + "/plan")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if outputJSON {
		fmt.Println(string(body))
		return nil
	}

	var changeset struct {
		Add    []string `json:"add"`
		Update []string `json:"update"`
		Remove []string `json:"remove"`
	}
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
	if !isOperatorRunning() {
		return fmt.Errorf("operator not running")
	}

	resp, err := http.Post(resolveOperator()+"/down", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("down failed (status %d)", resp.StatusCode)
	}

	printSuccess("Workspace stack stopped")
	printInfo("System stack (operator, django, postgres, redis) still running")
	return nil
}
