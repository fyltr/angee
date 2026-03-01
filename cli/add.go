package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fyltr/angee/internal/component"
	"github.com/fyltr/angee/internal/config"
	"github.com/spf13/cobra"
)

var (
	addParams []string
	addDeploy bool
	addYes    bool
)

var addCmd = &cobra.Command{
	Use:   "add <component>",
	Short: "Add a component to the stack",
	Long: `Add a component to the stack by merging its definition into angee.yaml.

Components can be:
  angee/postgres            Official component (github.com/angee-sh/postgres)
  fyltr/fyltr-django        Organization component (github.com/fyltr/fyltr-django)
  https://github.com/...    Full git URL
  ./local/path              Local directory

Examples:
  angee add angee/postgres
  angee add fyltr/fyltr-django --param Domain=myapp.io
  angee add angee/oauth-github --deploy`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func init() {
	addCmd.Flags().StringArrayVarP(&addParams, "param", "p", nil, "Component parameter (Key=Value)")
	addCmd.Flags().BoolVar(&addDeploy, "deploy", false, "Deploy after adding")
	addCmd.Flags().BoolVarP(&addYes, "yes", "y", false, "Non-interactive mode (use defaults)")
}

func runAdd(cmd *cobra.Command, args []string) error {
	source := args[0]
	rootPath := resolveRoot()

	// Parse --param flags
	params := make(map[string]string)
	for _, p := range addParams {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid parameter %q — use Key=Value format", p)
		}
		params[parts[0]] = parts[1]
	}

	printHeader("Adding component: " + source)

	var promptFn func(config.ComponentParam) (string, error)
	if !addYes {
		reader := bufio.NewReader(os.Stdin)
		promptFn = func(p config.ComponentParam) (string, error) {
			defaultHint := ""
			if p.Default != "" {
				defaultHint = fmt.Sprintf(" [%s]", p.Default)
			}
			fmt.Printf("  %s%s: ", p.Description, defaultHint)
			val, err := reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			val = strings.TrimSpace(val)
			if val == "" && p.Default != "" {
				return p.Default, nil
			}
			return val, nil
		}
	}

	record, err := component.Add(component.AddOptions{
		Source:   source,
		Params:   params,
		Deploy:   addDeploy,
		Yes:      addYes,
		RootPath: rootPath,
		PromptFn: promptFn,
	})
	if err != nil {
		printError(err.Error())
		return err
	}

	// Print what was added
	if len(record.Added.Services) > 0 {
		printSuccess("Services: " + strings.Join(record.Added.Services, ", "))
	}
	if len(record.Added.Agents) > 0 {
		printSuccess("Agents: " + strings.Join(record.Added.Agents, ", "))
	}
	if len(record.Added.MCPServers) > 0 {
		printSuccess("MCP servers: " + strings.Join(record.Added.MCPServers, ", "))
	}
	if len(record.Added.Secrets) > 0 {
		printInfo("Secrets added: " + strings.Join(record.Added.Secrets, ", "))
		printInfo("Set secret values with: angee credential set <name> <value>")
	}

	// Deploy if requested
	if addDeploy && isOperatorRunning() {
		printInfo("Deploying...")
		if err := triggerDeploy(); err != nil {
			printError("Deploy failed: " + err.Error())
			return err
		}
		printSuccess("Deployed successfully")
	}

	return nil
}
