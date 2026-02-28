package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update template and skills",
	Long: `Update platform components.

Subcommands:
  angee update template  Re-fetch and re-render the template
  angee update skills    Update skill definitions (placeholder)

To pull latest images, use 'angee pull'.
To restart the stack, use 'angee restart'.`,
}

var updateTemplateCmd = &cobra.Command{
	Use:   "template",
	Short: "Re-fetch and re-render the template",
	Long: `Re-fetch the template used during 'angee init', re-render angee.yaml.tmpl
with current parameters, and show a diff against the current angee.yaml.

The template source URL is stored in operator.yaml during init.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("angee update template is not yet implemented")
		fmt.Println("This will re-fetch the template from the source stored in operator.yaml,")
		fmt.Println("re-render angee.yaml.tmpl, and show a diff for review.")
		return nil
	},
}

var updateSkillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Update skill definitions",
	Long:  `Update skill definitions from a skill registry. (Placeholder for future implementation.)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("angee update skills is not yet implemented")
		fmt.Println("This will update skill definitions from a skill registry.")
		return nil
	},
}

func init() {
	updateCmd.AddCommand(updateTemplateCmd, updateSkillsCmd)
}
