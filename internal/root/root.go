// Package root manages the ANGEE_ROOT filesystem structure.
package root

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/git"
)

const (
	AngeeYAML    = "angee.yaml"
	OperatorYAML = "operator.yaml"
	ComposeFile  = "docker-compose.yaml"
	Gitignore    = ".gitignore"
	AgentsDir    = "agents"
	SrcDir       = "src"
	TemplatesDir = "templates"
)

// Root represents an initialized ANGEE_ROOT directory.
type Root struct {
	Path string
	Git  *git.Repo
}

// Open returns a Root for the given path. The path must already exist.
func Open(path string) (*Root, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("ANGEE_ROOT not found at %s — run 'angee init' first", path)
	}
	return &Root{Path: path, Git: git.New(path)}, nil
}

// Initialize creates a new ANGEE_ROOT at the given path.
func Initialize(path string) (*Root, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("creating ANGEE_ROOT: %w", err)
	}

	r := &Root{Path: path, Git: git.New(path)}

	// Initialize git
	if !git.IsRepo(path) {
		if err := r.Git.Init(); err != nil {
			return nil, fmt.Errorf("git init: %w", err)
		}
		if err := r.Git.ConfigureUser("angee", "angee@localhost"); err != nil {
			return nil, fmt.Errorf("git config: %w", err)
		}
	}

	// Create directory structure
	dirs := []string{
		filepath.Join(path, AgentsDir),
		filepath.Join(path, SrcDir),
		filepath.Join(path, TemplatesDir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", d, err)
		}
	}

	return r, nil
}

// WriteGitignore writes the standard .gitignore for ANGEE_ROOT.
func (r *Root) WriteGitignore() error {
	content := strings.TrimSpace(`
# Angee — never commit secrets or local runtime config
.env
*.env
operator.yaml
.angee-local

# Agent credentials (written at deploy time)
agents/**/.env
agents/**/workspace/

# Template caches
templates/*/

# Editor
.DS_Store
*.swp
`) + "\n"

	return os.WriteFile(filepath.Join(r.Path, Gitignore), []byte(content), 0644)
}

// WriteAgeeYAML writes an angee.yaml to the root.
func (r *Root) WriteAngeeYAML(content string) error {
	return os.WriteFile(filepath.Join(r.Path, AngeeYAML), []byte(content), 0644)
}

// LoadAngeeConfig reads and parses the angee.yaml.
func (r *Root) LoadAngeeConfig() (*config.AngeeConfig, error) {
	return config.Load(filepath.Join(r.Path, AngeeYAML))
}

// LoadOperatorConfig reads operator.yaml (or returns defaults).
func (r *Root) LoadOperatorConfig() (*config.OperatorConfig, error) {
	return config.LoadOperatorConfig(r.Path)
}

// WriteOperatorConfig writes operator.yaml (chmod 600 — never committed).
func (r *Root) WriteOperatorConfig(cfg *config.OperatorConfig) error {
	return config.WriteOperatorConfig(cfg, r.Path)
}

// AngeeYAMLPath returns the absolute path to angee.yaml.
func (r *Root) AngeeYAMLPath() string {
	return filepath.Join(r.Path, AngeeYAML)
}

// ComposePath returns the absolute path to the generated docker-compose.yaml.
func (r *Root) ComposePath() string {
	return filepath.Join(r.Path, ComposeFile)
}

// AgentDir returns the directory for a specific agent.
func (r *Root) AgentDir(agentName string) string {
	return filepath.Join(r.Path, AgentsDir, agentName)
}

// AgentEnvFile returns the path to an agent's runtime .env file.
func (r *Root) AgentEnvFile(agentName string) string {
	return filepath.Join(r.Path, AgentsDir, agentName, ".env")
}

// EnsureAgentDir creates the per-agent directory structure.
func (r *Root) EnsureAgentDir(agentName string) error {
	agentDir := r.AgentDir(agentName)
	if err := os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755); err != nil {
		return err
	}
	// Create empty .env if it doesn't exist
	envPath := r.AgentEnvFile(agentName)
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return os.WriteFile(envPath, []byte("# Agent credentials — written by operator\n"), 0600)
	}
	return nil
}

// WriteEnvFile writes secret key=value pairs to ANGEE_ROOT/.env (mode 0600).
// The .env file is gitignored — it is never committed.
func (r *Root) WriteEnvFile(content string) error {
	return os.WriteFile(filepath.Join(r.Path, ".env"), []byte(content), 0600)
}

// EnvFilePath returns the absolute path to the root .env file.
func (r *Root) EnvFilePath() string {
	return filepath.Join(r.Path, ".env")
}

// InitialCommit stages all files and creates the first commit.
func (r *Root) InitialCommit() error {
	if err := r.Git.Add("."); err != nil {
		return err
	}
	_, err := r.Git.Commit("angee: initialize project")
	return err
}

// CommitConfig stages angee.yaml and commits with the given message.
func (r *Root) CommitConfig(message string) (string, error) {
	if err := r.Git.Add(AngeeYAML); err != nil {
		return "", err
	}
	changed, err := r.Git.HasChanges()
	if err != nil || !changed {
		return "", err
	}
	return r.Git.Commit(message)
}

// DefaultAngeeRoot returns the default ANGEE_ROOT path.
func DefaultAngeeRoot() string {
	if v := os.Getenv("ANGEE_ROOT"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".angee"
	}
	return filepath.Join(home, ".angee")
}
