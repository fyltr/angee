package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/fyltr/angee-go/internal/config"
	"github.com/fyltr/angee-go/internal/root"
	"github.com/spf13/cobra"
)

var (
	initTemplate string
	initRepo     string
	initForce    bool
	initDir      string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new ANGEE_ROOT",
	Long: `Initialize a new ANGEE_ROOT directory with a default angee.yaml and git repository.

Examples:
  angee init                                          # Init at ~/.angee
  angee init --dir /opt/myproject                     # Init at a custom path
  angee init --template official/angee-django         # Use a template
  angee init --repo https://github.com/org/app        # Link a source repo`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initTemplate, "template", "", "Template name or URL")
	initCmd.Flags().StringVar(&initRepo, "repo", "", "Source repository URL to link")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing ANGEE_ROOT")
	initCmd.Flags().StringVar(&initDir, "dir", "", "Directory to initialize (default: ~/.angee)")
}

func runInit(cmd *cobra.Command, args []string) error {
	path := resolveRoot()
	if initDir != "" {
		path = initDir
	}

	// Check if already initialized
	if !initForce {
		if _, err := os.Stat(filepath.Join(path, "angee.yaml")); err == nil {
			return fmt.Errorf("ANGEE_ROOT already exists at %s (use --force to reinitialize)", path)
		}
	}

	fmt.Printf("\n\033[1mangee init\033[0m\n\n")

	// Get project name from directory
	projectName := filepath.Base(path)
	if projectName == ".angee" || projectName == "angee" {
		if wd, err := os.Getwd(); err == nil {
			projectName = filepath.Base(wd)
		}
	}
	projectName = strings.ToLower(strings.ReplaceAll(projectName, " ", "-"))

	// Initialize root structure
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

	// Generate angee.yaml from template or default
	var angeeYAML string
	if initTemplate != "" {
		angeeYAML, err = fetchTemplate(initTemplate, projectName)
		if err != nil {
			return fmt.Errorf("fetching template: %w", err)
		}
		printSuccess(fmt.Sprintf("Applied template: %s", initTemplate))
	} else {
		angeeYAML = renderDefaultTemplate(projectName)
		printSuccess("Created angee.yaml (default template)")
	}

	if err := r.WriteAngeeYAML(angeeYAML); err != nil {
		return err
	}

	// Write operator.yaml (not committed)
	opCfg := config.DefaultOperatorConfig(path)
	if err := r.WriteOperatorConfig(opCfg); err != nil {
		return err
	}
	printSuccess("Created operator.yaml (local runtime config — not committed)")

	// Initial git commit
	if err := r.InitialCommit(); err != nil {
		// May fail if git user not configured globally — non-fatal
		fmt.Printf("  \033[33m!\033[0m  git commit skipped: %s\n", err)
	} else {
		printSuccess("Initial git commit: \"angee: initialize project\"")
	}

	fmt.Printf("\n\033[1mNext steps:\033[0m\n\n")
	printInfo("angee up          Start the platform")
	printInfo("angee ls          View running agents and services")
	printInfo("angee admin       Chat with the admin agent")
	fmt.Println()

	return nil
}

// renderDefaultTemplate returns the default angee.yaml content.
func renderDefaultTemplate(projectName string) string {
	tmpl := template.Must(template.New("default").Parse(defaultAngeeTemplate))
	var sb strings.Builder
	tmpl.Execute(&sb, map[string]string{ //nolint:errcheck
		"ProjectName": projectName,
	})
	return sb.String()
}

// fetchTemplate retrieves a template by name or URL (stub for Phase 4).
func fetchTemplate(nameOrURL, projectName string) (string, error) {
	// TODO Phase 4: fetch from official registry or GitHub URL
	return renderDefaultTemplate(projectName), nil
}

// defaultAngeeTemplate is the built-in template for new projects.
const defaultAngeeTemplate = `# angee.yaml — source of truth for {{ .ProjectName }}
# Edit this file or ask your admin agent to make changes.
name: {{ .ProjectName }}

# Platform services
services:
  web:
    image: "nginx:alpine"
    lifecycle: platform
    domains:
      - host: localhost
        port: 80
    health:
      path: /
      port: 80

  postgres:
    image: "pgvector/pgvector:pg16"
    lifecycle: sidecar
    volumes:
      - name: pgdata
        path: /var/lib/postgresql/data
        persistent: true
    env:
      POSTGRES_USER: angee
      POSTGRES_PASSWORD: angee
      POSTGRES_DB: "{{ .ProjectName }}"

  redis:
    image: "redis:7-alpine"
    lifecycle: sidecar

# MCP servers available to agents
mcp_servers:
  angee-operator:
    transport: streamable-http
    url: http://operator:9000/mcp
    credentials:
      source: service_account
      scopes:
        - config.read
        - config.write
        - deploy
        - rollback
        - status
        - logs
        - scale

  angee-files:
    transport: stdio
    image: ghcr.io/fyltr/angee-filesystem-mcp:latest
    args:
      - --root
      - /workspace
      - --readonly
    credentials:
      source: none

# Default agents (always running)
agents:
  admin:
    image: ghcr.io/fyltr/angee-admin-agent:latest
    lifecycle: system
    role: operator
    description: "Platform admin — manages deployment, config, and infrastructure"
    mcp_servers:
      - angee-operator
      - angee-files
    workspace:
      path: /angee-root
      persistent: true

  developer:
    image: ghcr.io/fyltr/angee-developer-agent:latest
    lifecycle: system
    role: user
    description: "Developer agent — writes code, reviews PRs, and helps build features"
    mcp_servers:
      - angee-files
    workspace:
      persistent: true
    resources:
      cpu: "2.0"
      memory: "4Gi"
`
