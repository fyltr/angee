package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull latest container images",
	Long: `Pull the latest container images for all services and agents defined in
angee.yaml. Does NOT restart — run 'angee restart' after pulling to apply.

Usage:
  angee pull              Pull latest images
  angee pull && angee restart`,
	RunE: runPull,
}

func runPull(cmd *cobra.Command, args []string) error {
	path := resolveRoot()
	fmt.Printf("\n\033[1mangee pull\033[0m\n\n")

	composePath, projectName, err := localCompile(path)
	if err != nil {
		return err
	}

	printInfo("Pulling latest images...")
	if err := runDockerCompose(composePath, path, projectName, "pull"); err != nil {
		fmt.Printf("  \033[33m!\033[0m  Some images failed to pull (this is normal for locally-built images)\n")
	} else {
		printSuccess("Pulled latest images")
	}

	fmt.Println()
	printInfo("Run 'angee restart' to apply updated images")
	fmt.Println()
	return nil
}
