package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/fyltr/angee/api"
	"github.com/spf13/cobra"
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback [sha|HEAD~N]",
	Short: "Roll back to a previous deployment",
	Long: `Roll back ANGEE_ROOT to a previous git commit and redeploy.

Examples:
  angee rollback HEAD~1         # previous deploy
  angee rollback abc123def      # specific commit SHA`,
	Args: cobra.ExactArgs(1),
	RunE: runRollback,
}

func runRollback(cmd *cobra.Command, args []string) error {
	if !isOperatorRunning() {
		return fmt.Errorf("operator not running — start with 'angee up'")
	}

	sha := args[0]
	payload, _ := json.Marshal(api.RollbackRequest{SHA: sha})

	resp, err := doRequest("POST", resolveOperator()+"/rollback", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("rollback: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("rollback failed: %s", body)
	}

	var result api.RollbackResponse
	json.Unmarshal(body, &result) //nolint:errcheck

	printSuccess(fmt.Sprintf("Rolled back to %s", result.RolledBackTo))
	return nil
}
