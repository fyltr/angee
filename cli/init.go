package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fyltr/angee-go/internal/config"
	"github.com/fyltr/angee-go/internal/root"
	"github.com/fyltr/angee-go/internal/tmpl"
	"github.com/spf13/cobra"
)

var (
	initTemplate string
	initRepo     string
	initForce    bool
	initDir      string
	initYes      bool // skip interactive prompts
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new ANGEE_ROOT",
	Long: `Initialize a new ANGEE_ROOT directory with angee.yaml and a git repository.

Examples:
  angee init                                               # guided setup at ~/.angee
  angee init --template official/angee-django              # Django + pgvector + Celery
  angee init --dir /opt/myproject --yes                    # non-interactive
  angee init --repo https://github.com/org/app             # link a source repo`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initTemplate, "template", "t", "official/angee-django", "Template name (official/angee-django is the default)")
	initCmd.Flags().StringVar(&initRepo, "repo", "", "Source repository URL to link as 'base'")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing ANGEE_ROOT")
	initCmd.Flags().StringVar(&initDir, "dir", "", "Directory to initialize (default: ~/.angee)")
	initCmd.Flags().BoolVarP(&initYes, "yes", "y", false, "Accept all defaults (non-interactive)")
}

func runInit(cmd *cobra.Command, args []string) error {
	path := resolveRoot()
	if initDir != "" {
		path = initDir
	}
	// Expand ~
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}

	// Check if already initialized
	if !initForce {
		if _, err := os.Stat(filepath.Join(path, "angee.yaml")); err == nil {
			return fmt.Errorf("ANGEE_ROOT already exists at %s (use --force to reinitialize)", path)
		}
	}

	fmt.Printf("\n\033[1m  angee init\033[0m\n\n")

	// Determine project name
	projectName := deriveProjectName(path)

	// Gather template params (interactive or from flags)
	params, err := gatherParams(projectName)
	if err != nil {
		return err
	}

	// Override repo if --repo flag provided
	if initRepo != "" {
		fmt.Printf("  \033[36m→\033[0m  Linking repo: %s\n\n", initRepo)
	}

	// Initialize ANGEE_ROOT structure
	r, err := root.Initialize(path)
	if err != nil {
		return fmt.Errorf("initializing ANGEE_ROOT: %w", err)
	}
	printSuccess(fmt.Sprintf("Created ANGEE_ROOT at %s", path))

	// Write .gitignore
	if err := r.WriteGitignore(); err != nil {
		return err
	}
	printSuccess("Created .gitignore")

	// Render and write angee.yaml
	angeeYAML, err := renderTemplate(initTemplate, params)
	if err != nil {
		return fmt.Errorf("rendering template %s: %w", initTemplate, err)
	}

	// If --repo provided, patch the repositories section to use that URL
	if initRepo != "" {
		angeeYAML = patchRepoURL(angeeYAML, initRepo)
	}

	if err := r.WriteAngeeYAML(angeeYAML); err != nil {
		return err
	}
	printSuccess(fmt.Sprintf("Created angee.yaml (template: %s)", initTemplate))

	// Write operator.yaml (not committed — local runtime config)
	opCfg := config.DefaultOperatorConfig(path)
	if err := r.WriteOperatorConfig(opCfg); err != nil {
		return err
	}
	printSuccess("Created operator.yaml (local runtime config — gitignored)")

	// Initial git commit
	if err := r.InitialCommit(); err != nil {
		fmt.Printf("  \033[33m!\033[0m  git commit skipped: %s\n", err)
	} else {
		printSuccess("Initial git commit: \"angee: initialize project\"")
	}

	// Print secrets that need to be configured
	printRequiredSecrets(angeeYAML)

	fmt.Printf("\n\033[1m  Next steps:\033[0m\n\n")
	printInfo("angee up          Start the platform")
	printInfo("angee ls          View running agents and services")
	printInfo("angee admin       Chat with the admin agent")
	fmt.Println()

	return nil
}

// gatherParams collects template parameters, prompting if not --yes.
func gatherParams(projectName string) (tmpl.TemplateParams, error) {
	params := tmpl.DefaultParams(projectName)

	if initYes {
		return params, nil
	}

	reader := bufio.NewReader(os.Stdin)

	params.ProjectName = prompt(reader, "Project name", projectName)
	params.Domain = prompt(reader, "Primary domain", "localhost")

	if params.Domain != "localhost" {
		params.DBPassword = prompt(reader, "Postgres password", "angee")
	}

	fmt.Println()
	return params, nil
}

// prompt prints a question and reads a line, returning the default if empty.
func prompt(r *bufio.Reader, question, defaultVal string) string {
	fmt.Printf("  \033[1m%s\033[0m [%s]: ", question, defaultVal)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

// renderTemplate renders the named template with the given params.
// Supports:
//   - "official/angee-django"  → embedded official template
//   - "https://..."            → fetch from URL (Phase 4)
//   - "./path"                 → local filesystem template (Phase 4)
func renderTemplate(name string, params tmpl.TemplateParams) (string, error) {
	// Official templates are embedded in the binary
	if strings.HasPrefix(name, "official/") {
		shortName := strings.TrimPrefix(name, "official/")
		content, err := tmpl.RenderOfficial(shortName, params)
		if err != nil {
			return "", fmt.Errorf("official template %q not found: %w", name, err)
		}
		return content, nil
	}

	// TODO Phase 4: fetch from GitHub URL or local path
	return "", fmt.Errorf("template %q: only official/* templates are supported in this version", name)
}

// deriveProjectName picks a project name from the ANGEE_ROOT path.
func deriveProjectName(path string) string {
	name := filepath.Base(path)
	if name == ".angee" || name == "angee" {
		if wd, err := os.Getwd(); err == nil {
			name = filepath.Base(wd)
		}
	}
	// Normalize: lowercase, replace spaces and underscores with hyphens
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")
	return name
}

// patchRepoURL replaces the default repo URL with the user-provided --repo value.
func patchRepoURL(angeeYAML, repoURL string) string {
	return strings.ReplaceAll(
		angeeYAML,
		"https://github.com/fyltr/angee-django",
		repoURL,
	)
}

// printRequiredSecrets parses the rendered angee.yaml for a secrets: section
// and prints the secrets that need to be configured before first deploy.
func printRequiredSecrets(angeeYAML string) {
	// Simple scan for "name:" lines under "secrets:" block
	inSecrets := false
	var secrets []string
	for _, line := range strings.Split(angeeYAML, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "secrets:" {
			inSecrets = true
			continue
		}
		if inSecrets {
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && trimmed != "" {
				break // left secrets block
			}
			if strings.HasPrefix(trimmed, "name: ") {
				secrets = append(secrets, strings.TrimPrefix(trimmed, "name: "))
			}
		}
	}

	if len(secrets) == 0 {
		return
	}

	fmt.Printf("\n  \033[33m!\033[0m  \033[1mRequired secrets\033[0m (set before first deploy):\n\n")
	for _, s := range secrets {
		fmt.Printf("      angee secret set %s <value>\n", s)
	}
	fmt.Println()
}
