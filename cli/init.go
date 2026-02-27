package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fyltr/angee-go/internal/compiler"
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
	initYes      bool
	initSecrets  []string // --secret key=value (repeatable)
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new ANGEE_ROOT",
	Long: `Initialize a new ANGEE_ROOT directory with angee.yaml and a git repository.

Generated secrets (django-secret-key, db-password, etc.) are written to
~/.angee/.env which is gitignored. Supply your own values with --secret.

The --template flag accepts a Git URL or a local directory path:
  angee init --template https://github.com/fyltr/angee-django-template
  angee init --template ./path/to/local/template

Examples:
  angee init                                         # guided setup (default template)
  angee init --yes                                   # accept all defaults
  angee init --template https://github.com/org/tmpl  # template from GitHub
  angee init --template ./my-template                # local template directory
  angee init --repo https://github.com/org/app       # link a source repo
  angee init --secret django-secret-key=mysecretkey  # supply a secret
  angee init --secret db-password=mypass --yes       # non-interactive`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initTemplate, "template", "t", "https://github.com/fyltr/angee-django-template", "Template source (Git URL or local path)")
	initCmd.Flags().StringVar(&initRepo, "repo", "", "Source repository URL to link as 'base'")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing ANGEE_ROOT")
	initCmd.Flags().StringVar(&initDir, "dir", "", "Directory to initialize (default: ~/.angee)")
	initCmd.Flags().BoolVarP(&initYes, "yes", "y", false, "Accept all defaults (non-interactive)")
	initCmd.Flags().StringArrayVar(&initSecrets, "secret", nil, "Set a secret: --secret name=value (repeatable)")
}

func runInit(cmd *cobra.Command, args []string) error {
	path := resolveRoot()
	if initDir != "" {
		path = initDir
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}

	if !initForce {
		if _, err := os.Stat(filepath.Join(path, "angee.yaml")); err == nil {
			return fmt.Errorf("ANGEE_ROOT already exists at %s (use --force to reinitialize)", path)
		}
	}

	fmt.Printf("\n\033[1m  angee init\033[0m\n\n")

	// Parse --secret flags into a map
	supplied := parseSecretFlags(initSecrets)

	// Determine project name and template params
	projectName := deriveProjectName(path)
	params, err := gatherParams(projectName)
	if err != nil {
		return err
	}

	// Fetch template (clone if URL, use directly if local path)
	templateDir, cleanup, err := tmpl.FetchTemplate(initTemplate)
	if err != nil {
		return fmt.Errorf("fetching template: %w", err)
	}
	if cleanup {
		defer os.RemoveAll(templateDir)
	}

	// Load template metadata to know which secrets are needed
	meta, err := tmpl.LoadMeta(templateDir)
	if err != nil {
		return fmt.Errorf("loading template metadata: %w", err)
	}

	// Resolve secrets: flag → generate → derive
	secrets, err := tmpl.ResolveSecrets(meta, supplied, params.ProjectName)
	if err != nil {
		return err
	}

	// Initialize ANGEE_ROOT
	r, err := root.Initialize(path)
	if err != nil {
		return fmt.Errorf("initializing ANGEE_ROOT: %w", err)
	}
	printSuccess(fmt.Sprintf("Created ANGEE_ROOT at %s", path))

	if err := r.WriteGitignore(); err != nil {
		return err
	}
	printSuccess("Created .gitignore")

	// Render angee.yaml from template
	angeeYAML, err := tmpl.Render(templateDir, params)
	if err != nil {
		return fmt.Errorf("rendering template: %w", err)
	}
	if initRepo != "" {
		angeeYAML = patchRepoURL(angeeYAML, initRepo)
	}

	if err := r.WriteAngeeYAML(angeeYAML); err != nil {
		return err
	}
	printSuccess(fmt.Sprintf("Created angee.yaml (template: %s)", initTemplate))

	// Write .env with generated/supplied secrets (mode 0600, gitignored)
	envContent := tmpl.FormatEnvFile(secrets)
	if err := r.WriteEnvFile(envContent); err != nil {
		return fmt.Errorf("writing .env: %w", err)
	}
	printSuccess(fmt.Sprintf("Created .env (%d secret(s) — gitignored, never committed)", len(secrets)))

	// Write operator.yaml
	opCfg := config.DefaultOperatorConfig(path)
	if err := r.WriteOperatorConfig(opCfg); err != nil {
		return err
	}
	printSuccess("Created operator.yaml (local runtime config — gitignored)")

	// Compile angee.yaml → docker-compose.yaml so `angee up` can start
	// the stack immediately without needing the compiler.
	cfg, err := config.Load(filepath.Join(path, "angee.yaml"))
	if err != nil {
		return fmt.Errorf("loading angee.yaml for compilation: %w", err)
	}
	comp := compiler.New(path, opCfg.Docker.Network)
	cf, err := comp.Compile(cfg)
	if err != nil {
		return fmt.Errorf("compiling angee.yaml: %w", err)
	}
	if err := compiler.Write(cf, filepath.Join(path, "docker-compose.yaml")); err != nil {
		return fmt.Errorf("writing docker-compose.yaml: %w", err)
	}
	printSuccess("Compiled docker-compose.yaml")

	// Initial git commit (only tracks angee.yaml + .gitignore, not .env)
	if err := r.InitialCommit(); err != nil {
		fmt.Printf("  \033[33m!\033[0m  git commit skipped: %s\n", err)
	} else {
		printSuccess("Initial git commit: \"angee: initialize project\"")
	}

	// Print secrets summary
	printSecretsTable(secrets, r.EnvFilePath())

	fmt.Printf("\n\033[1m  Next steps:\033[0m\n\n")
	printInfo("angee up          Start the platform")
	printInfo("angee ls          View running agents and services")
	printInfo("angee admin       Chat with the admin agent")
	fmt.Println()

	return nil
}

// parseSecretFlags converts ["key=value", ...] to a map.
func parseSecretFlags(flags []string) map[string]string {
	out := make(map[string]string, len(flags))
	for _, f := range flags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) == 2 {
			out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return out
}

// printSecretsTable shows each secret, its source, and a redacted value.
func printSecretsTable(secrets []tmpl.ResolvedSecret, envPath string) {
	if len(secrets) == 0 {
		return
	}

	fmt.Printf("\n  \033[1mSecrets\033[0m → %s\n\n", envPath)

	for _, s := range secrets {
		badge := secretBadge(s.Source)
		preview := redact(s.Value)
		fmt.Printf("    %-28s %s  %s\n", s.Name, badge, preview)
	}
	fmt.Println()
	fmt.Printf("  \033[33m!\033[0m  Keep .env safe — rotate with: \033[1mangee secret set <name> <value>\033[0m\n")
}

func secretBadge(source string) string {
	switch source {
	case "flag":
		return "\033[34m[supplied]\033[0m   "
	case "generated":
		return "\033[32m[generated]\033[0m  "
	case "derived":
		return "\033[36m[derived]\033[0m    "
	default:
		return "             "
	}
}

// redact shows the first 4 chars of a secret then ****
func redact(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return v[:4] + strings.Repeat("*", min(len(v)-4, 12))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// gatherParams collects template params, prompting interactively unless --yes.
func gatherParams(projectName string) (tmpl.TemplateParams, error) {
	params := tmpl.DefaultParams(projectName)
	if initYes {
		return params, nil
	}

	reader := bufio.NewReader(os.Stdin)
	params.ProjectName = prompt(reader, "Project name", projectName)
	params.Domain = prompt(reader, "Primary domain", "localhost")
	fmt.Println()
	return params, nil
}

func prompt(r *bufio.Reader, question, defaultVal string) string {
	fmt.Printf("  \033[1m%s\033[0m [%s]: ", question, defaultVal)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

func deriveProjectName(path string) string {
	name := filepath.Base(path)
	if name == ".angee" || name == "angee" {
		if wd, err := os.Getwd(); err == nil {
			name = filepath.Base(wd)
		}
	}
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")
	return name
}

func patchRepoURL(angeeYAML, repoURL string) string {
	return strings.ReplaceAll(angeeYAML, "https://github.com/fyltr/angee-django", repoURL)
}
