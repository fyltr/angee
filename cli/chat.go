package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fyltr/angee-go/api"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat [agent]",
	Short: "Attach to an agent (default: admin)",
	Long: `Open an interactive chat session with an agent.

Examples:
  angee chat              # chat with admin agent
  angee chat developer    # chat with developer agent
  angee chat my-agent     # chat with any named agent`,
	RunE: func(cmd *cobra.Command, args []string) error {
		agent := "admin"
		if len(args) > 0 {
			agent = args[0]
		}
		return runChat(agent)
	},
}

// adminCmd is a shortcut for `angee chat admin`
var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Chat with the admin agent (shortcut for 'angee chat admin')",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runChat("admin")
	},
}

// developCmd is a shortcut for `angee chat developer`
var developCmd = &cobra.Command{
	Use:     "develop",
	Aliases: []string{"developer", "dev"},
	Short:   "Chat with the developer agent (shortcut for 'angee chat developer')",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runChat("developer")
	},
}

// askCmd sends a single message to an agent (non-interactive).
var askCmd = &cobra.Command{
	Use:   "ask [message]",
	Short: "Send a one-shot message to the admin agent",
	Long: `Send a single message to an agent and print the response.

Examples:
  angee ask "scale web to 3 replicas"
  angee ask --agent developer "add a health check endpoint"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runAsk,
}

var askAgent string

func init() {
	askCmd.Flags().StringVar(&askAgent, "agent", "admin", "Agent to send the message to")
}

func runChat(agentName string) error {
	// Verify operator is reachable
	if !isOperatorRunning() {
		return fmt.Errorf("operator not running — start with 'angee up'")
	}

	fmt.Printf("\n\033[1mConnected to %s agent\033[0m\n", agentName)
	fmt.Printf("\033[90m(type your message and press Enter — /exit to quit)\033[0m\n\n")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("\033[36myou:\033[0m ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "/exit" || line == "/quit" || line == "exit" || line == "quit" {
			fmt.Println("\n  Disconnected.")
			break
		}

		response, err := sendMessage(agentName, line)
		if err != nil {
			printError(fmt.Sprintf("error: %s", err))
			continue
		}

		fmt.Printf("\n\033[1m%s:\033[0m %s\n\n", agentName, response)
	}
	return nil
}

func runAsk(cmd *cobra.Command, args []string) error {
	message := strings.Join(args, " ")
	agent := askAgent

	if !isOperatorRunning() {
		return fmt.Errorf("operator not running — start with 'angee up'")
	}

	response, err := sendMessage(agent, message)
	if err != nil {
		return err
	}

	fmt.Println(response)
	return nil
}

func sendMessage(agentName, message string) (string, error) {
	payload, _ := json.Marshal(api.ChatRequest{
		Message: message,
		Agent:   agentName,
	})

	url := fmt.Sprintf("%s/agents/%s/chat", resolveOperator(), agentName)
	resp, err := doRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("connecting to agent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("agent %q not found — check 'angee ls'", agentName)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("agent returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result api.ChatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		// Return raw text if not JSON
		return strings.TrimSpace(string(body)), nil
	}
	if result.Response != "" {
		return result.Response, nil
	}
	return result.Message, nil
}
