package cli

import (
	"fmt"
	"strings"

	"github.com/fyltr/angee/internal/component"
	"github.com/spf13/cobra"
)

var (
	removeDeploy bool
	removeForce  bool
)

var removeCmd = &cobra.Command{
	Use:   "remove <component>",
	Short: "Remove a component from the stack",
	Long: `Remove a component from the stack by undoing its angee.yaml changes.

Examples:
  angee remove fyltr/fyltr-django
  angee remove angee/postgres --force --deploy`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

var listComponentsCmd = &cobra.Command{
	Use:   "components",
	Short: "List installed components",
	RunE:  runListComponents,
}

func init() {
	removeCmd.Flags().BoolVar(&removeDeploy, "deploy", false, "Deploy after removing")
	removeCmd.Flags().BoolVar(&removeForce, "force", false, "Skip confirmation")
}

func runRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	rootPath := resolveRoot()

	printHeader("Removing component: " + name)

	if err := component.Remove(component.RemoveOptions{
		Name:     name,
		Deploy:   removeDeploy,
		Force:    removeForce,
		RootPath: rootPath,
	}); err != nil {
		printError(err.Error())
		return err
	}

	printSuccess("Removed " + name)

	if removeDeploy && isOperatorRunning() {
		printInfo("Deploying...")
		if err := triggerDeploy(); err != nil {
			printError("Deploy failed: " + err.Error())
			return err
		}
		printSuccess("Deployed successfully")
	}

	return nil
}

func runListComponents(cmd *cobra.Command, args []string) error {
	rootPath := resolveRoot()

	components, err := component.List(rootPath)
	if err != nil {
		return err
	}

	if len(components) == 0 {
		fmt.Println("No components installed.")
		return nil
	}

	printHeader("Installed Components")
	for _, c := range components {
		services := strings.Join(c.Added.Services, ", ")
		agents := strings.Join(c.Added.Agents, ", ")
		parts := []string{}
		if services != "" {
			parts = append(parts, "services: "+services)
		}
		if agents != "" {
			parts = append(parts, "agents: "+agents)
		}
		detail := ""
		if len(parts) > 0 {
			detail = " (" + strings.Join(parts, "; ") + ")"
		}
		fmt.Printf("  %s %s [%s]%s\n", c.Name, c.Version, c.Type, detail)
	}

	return nil
}
