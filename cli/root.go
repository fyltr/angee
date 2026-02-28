// Package cli implements the angee command-line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	// rootFlags
	angeeRoot   string
	operatorURL string
	outputJSON  bool
	apiKey      string
)

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:   "angee",
	Short: "Agentic infrastructure as code",
	Long: `angee — self-managed agent containerisation and orchestration engine.

Get started:
  angee init         Initialize ANGEE_ROOT
  angee up           Start the platform
  angee ls           List running agents and services
  angee admin        Chat with the admin agent
  angee develop      Chat with the developer agent
  angee chat <name>  Connect to any agent`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&angeeRoot, "root", "", "ANGEE_ROOT path (default: ~/.angee)")
	rootCmd.PersistentFlags().StringVar(&operatorURL, "operator", "", "Operator URL (default: http://localhost:9000)")
	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key for operator authentication")

	// Register all subcommands
	rootCmd.AddCommand(
		initCmd,
		upCmd,
		downCmd,
		lsCmd,
		deployCmd,
		rollbackCmd,
		logsCmd,
		planCmd,
		chatCmd,
		adminCmd,
		developCmd,
		statusCmd,
		askCmd,
		updateCmd,
	)
}

// resolveRoot returns the ANGEE_ROOT path from flag, env, or default.
func resolveRoot() string {
	if angeeRoot != "" {
		return angeeRoot
	}
	if v := os.Getenv("ANGEE_ROOT"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".angee"
	}
	return home + "/.angee"
}

// resolveOperator returns the operator base URL.
func resolveOperator() string {
	if operatorURL != "" {
		return operatorURL
	}
	if v := os.Getenv("ANGEE_OPERATOR_URL"); v != "" {
		return v
	}
	return "http://localhost:9000"
}

// resolveAPIKey returns the API key from flag, env, or empty.
func resolveAPIKey() string {
	if apiKey != "" {
		return apiKey
	}
	if v := os.Getenv("ANGEE_API_KEY"); v != "" {
		return v
	}
	return ""
}

// newRequest creates an http.Request with auth and content-type headers set.
func newRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if key := resolveAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if body != nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// doRequest creates and executes an HTTP request with auth headers.
func doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := newRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func printSuccess(msg string) {
	fmt.Printf("  \033[32m✔\033[0m %s\n", msg)
}

func printInfo(msg string) {
	fmt.Printf("  \033[36m→\033[0m %s\n", msg)
}

func printError(msg string) {
	fmt.Fprintf(os.Stderr, "  \033[31m✗\033[0m %s\n", msg)
}

func printHeader(msg string) {
	fmt.Printf("\n\033[1m%s\033[0m\n", msg)
}

// isOperatorRunning checks if the operator HTTP health endpoint responds.
func isOperatorRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := newRequest("GET", resolveOperator()+"/health", nil)
	if err != nil {
		return false
	}
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// triggerDeploy POSTs to the operator's deploy endpoint.
func triggerDeploy() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := newRequest("POST", resolveOperator()+"/deploy", nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
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
