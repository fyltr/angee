// Package cli implements the angee command-line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/fyltr/angee/cli.Version=v0.0.7".
// Defaults to "dev" for local builds.
var Version = "dev"

// httpClient is the package-level HTTP client used for CLI → operator calls.
// 60-second timeout protects the CLI from a deadlocked operator. Streaming
// endpoints (logs) bypass this and manage their own deadlines.
var httpClient = &http.Client{Timeout: 60 * time.Second}

var (
	// rootFlags
	angeeRoot   string
	operatorURL string
	outputJSON  bool
	apiKey      string
)

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:     "angee",
	Short:   "Agentic infrastructure as code",
	Version: Version,
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
		destroyCmd,
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
		pullCmd,
		restartCmd,
		updateCmd,
		// Project-mode forwarders (see cli/project.go + internal/projmode/).
		// These exec the runtime framework via syscall.Exec when invoked
		// inside a tree containing .angee/project.yaml.
		buildCmd,
		migrateCmd,
		doctorCmd,
		fixturesCmd,
		// Project-mode orchestrator (see cli/dev.go + internal/dev/).
		devCmd,
	)
}

// resolveRoot returns the ANGEE_ROOT path from flag, env, or by detecting
// the current directory structure. Detection order:
//  1. --root flag
//  2. ANGEE_ROOT env var
//  3. angee.yaml in cwd → we're inside ANGEE_ROOT, use cwd
//  4. .angee/ in cwd → use it
//  5. fallback: ~/.angee
func resolveRoot() string {
	if angeeRoot != "" {
		return angeeRoot
	}
	if v := os.Getenv("ANGEE_ROOT"); v != "" {
		return v
	}

	// Check current directory: are we inside ANGEE_ROOT?
	if _, err := os.Stat("angee.yaml"); err == nil {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
	}
	// Check for .angee/ in current directory. Skip when it's a project-mode
	// marker (contains project.yaml) — those are NOT compose-mode roots
	// and would mislead init / up / etc. into operating on the wrong tree.
	// See docs/ARCHITECTURE.md §12, django-angee R-15.
	if info, err := os.Stat(".angee"); err == nil && info.IsDir() {
		if _, isProj := os.Stat(filepath.Join(".angee", "project.yaml")); isProj != nil {
			if wd, err := os.Getwd(); err == nil {
				return filepath.Join(wd, ".angee")
			}
		}
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
	return httpClient.Do(req)
}

func printSuccess(msg string) {
	fmt.Printf("  \033[32m✔\033[0m %s\n", msg)
}

func printInfo(msg string) {
	fmt.Printf("  \033[36m→\033[0m %s\n", msg)
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
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == 200
}
