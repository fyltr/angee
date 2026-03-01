package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/credentials"
	"github.com/spf13/cobra"
)

var credentialCmd = &cobra.Command{
	Use:   "credential",
	Short: "Manage stack credentials",
	Long: `Manage secrets and credentials in the stack.

Examples:
  angee credential list
  angee credential set anthropic-api-key sk-ant-xxx
  angee credential get anthropic-api-key
  angee credential delete old-secret`,
}

var credListCmd = &cobra.Command{
	Use:   "list",
	Short: "List credential names",
	RunE:  runCredList,
}

var credGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get credential metadata",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredGet,
}

var credSetCmd = &cobra.Command{
	Use:   "set <name> <value>",
	Short: "Set a credential value",
	Args:  cobra.ExactArgs(2),
	RunE:  runCredSet,
}

var credDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a credential",
	Args:  cobra.ExactArgs(1),
	RunE:  runCredDelete,
}

func init() {
	credentialCmd.AddCommand(credListCmd, credGetCmd, credSetCmd, credDeleteCmd)
}

func getBackend() (credentials.Backend, error) {
	rootPath := resolveRoot()
	cfgPath := rootPath + "/angee.yaml"

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		// No angee.yaml — use env backend directly
		return credentials.NewEnvBackend(rootPath + "/.env"), nil
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	return credentials.NewBackend(cfg, rootPath)
}

func runCredList(cmd *cobra.Command, args []string) error {
	backend, err := getBackend()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	names, err := backend.List(ctx)
	if err != nil {
		return err
	}

	if len(names) == 0 {
		fmt.Println("No credentials stored.")
		return nil
	}

	printHeader(fmt.Sprintf("Credentials (%s backend)", backend.Type()))
	for _, name := range names {
		fmt.Printf("  %s\n", name)
	}
	return nil
}

func runCredGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	backend, err := getBackend()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	val, err := backend.Get(ctx, name)
	if err != nil {
		return err
	}

	// Show masked value
	masked := val
	if len(masked) > 8 {
		masked = masked[:4] + strings.Repeat("*", len(masked)-8) + masked[len(masked)-4:]
	} else {
		masked = strings.Repeat("*", len(masked))
	}
	fmt.Printf("  %s = %s\n", name, masked)
	return nil
}

func runCredSet(cmd *cobra.Command, args []string) error {
	name, value := args[0], args[1]
	backend, err := getBackend()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := backend.Set(ctx, name, value); err != nil {
		return err
	}
	printSuccess(fmt.Sprintf("Set %s (%s backend)", name, backend.Type()))
	return nil
}

func runCredDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	backend, err := getBackend()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := backend.Delete(ctx, name); err != nil {
		return err
	}
	printSuccess(fmt.Sprintf("Deleted %s", name))
	return nil
}
