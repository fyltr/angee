// Package cli implements the angee command-line interface.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// rootFlags
	angeeRoot   string
	operatorURL string
	outputJSON  bool
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
