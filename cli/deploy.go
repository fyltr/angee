package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/fyltr/angee-go/api"
	"github.com/spf13/cobra"
)

var deployMessage string

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the current angee.yaml to the runtime",
	Long: `Compile angee.yaml and apply it to the running platform.

This is equivalent to asking the admin agent to deploy, but bypasses
the agent for direct operator interaction.`,
	RunE: runDeploy,
}

func init() {
	deployCmd.Flags().StringVarP(&deployMessage, "message", "m", "", "Commit message for this deploy")
}

func runDeploy(cmd *cobra.Command, args []string) error {
	if !isOperatorRunning() {
		return fmt.Errorf("operator not running — start with 'angee up'")
	}

	payload, _ := json.Marshal(api.DeployRequest{Message: deployMessage})

	resp, err := doRequest("POST", resolveOperator()+"/deploy", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("deploying: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("deploy failed: %s", body)
	}

	if outputJSON {
		fmt.Println(string(body))
		return nil
	}

	var result api.ApplyResult
	json.Unmarshal(body, &result) //nolint:errcheck

	printSuccess("Deployed")
	if len(result.ServicesStarted) > 0 {
		printInfo(fmt.Sprintf("Started: %v", result.ServicesStarted))
	}
	if len(result.ServicesUpdated) > 0 {
		printInfo(fmt.Sprintf("Updated: %v", result.ServicesUpdated))
	}
	if len(result.ServicesRemoved) > 0 {
		printInfo(fmt.Sprintf("Removed: %v", result.ServicesRemoved))
	}
	return nil
}
