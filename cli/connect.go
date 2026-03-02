package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/credentials"
	"github.com/spf13/cobra"
)

var connectRemove bool

var connectCmd = &cobra.Command{
	Use:   "connect [account]",
	Short: "Manage connected accounts",
	Long: `List, connect, or disconnect external accounts (OAuth, API keys, etc.).

With no arguments, lists all connected accounts and their status.
With an account name, runs the connection flow for that account.

Examples:
  angee connect                    # list all connected accounts
  angee connect anthropic          # run setup_command for anthropic
  angee connect google             # open browser for Google OAuth
  angee connect --remove google    # disconnect account`,
	Args: cobra.MaximumNArgs(1),
	RunE: runConnect,
}

func init() {
	connectCmd.Flags().BoolVar(&connectRemove, "remove", false, "Disconnect the specified account")
	rootCmd.AddCommand(connectCmd)
}

func runConnect(cmd *cobra.Command, args []string) error {
	rootPath := resolveRoot()
	cfgPath := filepath.Join(rootPath, "angee.yaml")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading angee.yaml: %w", err)
	}

	if len(cfg.ConnectedAccounts) == 0 {
		fmt.Println("No connected accounts defined in angee.yaml.")
		return nil
	}

	// No arguments: list all connected accounts with status
	if len(args) == 0 {
		return listConnectedAccounts(cfg, rootPath)
	}

	acctName := args[0]
	acct, ok := cfg.ConnectedAccounts[acctName]
	if !ok {
		return fmt.Errorf("connected account %q not defined in angee.yaml", acctName)
	}

	if connectRemove {
		return disconnectAccount(cfg, rootPath, acctName)
	}

	return connectAccount(cfg, rootPath, acctName, acct)
}

func listConnectedAccounts(cfg *config.AngeeConfig, rootPath string) error {
	backend, err := credentials.NewBackend(cfg, rootPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	printHeader("  Connected Accounts")
	fmt.Println()

	for name, acct := range cfg.ConnectedAccounts {
		status := "not connected"
		secretName := "account-" + name
		if _, err := backend.Get(ctx, secretName); err == nil {
			status = "connected"
		}

		icon := "\033[31m●\033[0m" // red dot
		if status == "connected" {
			icon = "\033[32m●\033[0m" // green dot
		}

		desc := acct.Description
		if desc == "" {
			desc = acct.Provider
		}

		typeLabel := acct.Type
		if acct.Required {
			typeLabel += ", required"
		}

		fmt.Printf("  %s  %-20s  %-14s  %s\n", icon, name, "("+typeLabel+")", desc)
	}
	fmt.Println()
	return nil
}

func connectAccount(cfg *config.AngeeConfig, rootPath, name string, acct config.ConnectedAccountSpec) error {
	switch acct.Type {
	case "setup_command":
		return connectSetupCommand(cfg, rootPath, name, acct)
	case "api_key":
		return connectAPIKey(cfg, rootPath, name, acct)
	case "token":
		return connectToken(cfg, rootPath, name, acct)
	case "oauth":
		return connectOAuth(name, acct)
	default:
		return fmt.Errorf("unsupported account type: %s", acct.Type)
	}
}

func connectSetupCommand(cfg *config.AngeeConfig, rootPath, name string, acct config.ConnectedAccountSpec) error {
	if acct.SetupCommand == nil || len(acct.SetupCommand.Command) == 0 {
		return fmt.Errorf("connected account %q: setup_command.command is required", name)
	}

	if acct.Description != "" {
		fmt.Printf("\n  \033[1m%s\033[0m\n", acct.Description)
	}
	if acct.SetupCommand.Prompt != "" {
		fmt.Printf("  %s\n\n", acct.SetupCommand.Prompt)
	}

	// Run the command and capture stdout
	cmdName := acct.SetupCommand.Command[0]
	cmdArgs := acct.SetupCommand.Command[1:]

	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setup command failed: %w", err)
	}

	// Parse the output
	value := parseSetupOutput(stdout.String(), acct.SetupCommand.Parse)
	if value == "" {
		return fmt.Errorf("setup command produced no output")
	}

	// Store the credential
	return storeAccountCredential(cfg, rootPath, name, value)
}

func connectAPIKey(cfg *config.AngeeConfig, rootPath, name string, acct config.ConnectedAccountSpec) error {
	if acct.Description != "" {
		fmt.Printf("\n  \033[1m%s\033[0m\n\n", acct.Description)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("  Enter API key for %s: ", acct.Provider)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return fmt.Errorf("no value provided")
	}

	return storeAccountCredential(cfg, rootPath, name, value)
}

func connectToken(cfg *config.AngeeConfig, rootPath, name string, acct config.ConnectedAccountSpec) error {
	if acct.Description != "" {
		fmt.Printf("\n  \033[1m%s\033[0m\n\n", acct.Description)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("  Enter token for %s: ", acct.Provider)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return fmt.Errorf("no value provided")
	}

	return storeAccountCredential(cfg, rootPath, name, value)
}

func connectOAuth(name string, acct config.ConnectedAccountSpec) error {
	url := resolveOperator() + "/oauth/start/" + name
	fmt.Printf("\n  To connect %s, visit:\n\n", acct.Provider)
	fmt.Printf("    \033[1m%s\033[0m\n\n", url)
	fmt.Println("  The browser flow will store tokens automatically.")
	return nil
}

func disconnectAccount(cfg *config.AngeeConfig, rootPath, name string) error {
	backend, err := credentials.NewBackend(cfg, rootPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	secretName := "account-" + name
	if err := backend.Delete(ctx, secretName); err != nil {
		return fmt.Errorf("disconnecting %s: %w", name, err)
	}

	printSuccess(fmt.Sprintf("Disconnected %s", name))
	return nil
}

func storeAccountCredential(cfg *config.AngeeConfig, rootPath, name, value string) error {
	backend, err := credentials.NewBackend(cfg, rootPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	secretName := "account-" + name
	if err := backend.Set(ctx, secretName, value); err != nil {
		return fmt.Errorf("storing credential: %w", err)
	}

	printSuccess(fmt.Sprintf("Connected %s (%s backend)", name, backend.Type()))
	return nil
}

// parseSetupOutput extracts a value from command output based on the parse mode.
func parseSetupOutput(output, mode string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}

	switch mode {
	case "last_line":
		lines := strings.Split(output, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				return line
			}
		}
		return ""
	case "stdout", "":
		return output
	default:
		return output
	}
}
