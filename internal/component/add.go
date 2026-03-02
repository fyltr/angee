// Package component implements the angee add / angee remove lifecycle.
package component

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/secrets"
	"gopkg.in/yaml.v3"
)

// AddOptions configures the angee add operation.
type AddOptions struct {
	Source     string            // component ref: "angee/postgres", URL, or local path
	Params    map[string]string // --param Key=Value overrides
	Deploy    bool              // deploy after adding
	Yes       bool              // non-interactive mode
	RootPath  string            // ANGEE_ROOT path
	PromptFn  func(param config.ComponentParam) (string, error) // interactive prompt
}

// Add installs a component into the stack.
func Add(opts AddOptions) (*config.InstalledComponent, error) {
	// 1. Resolve and fetch component source
	compDir, cleanupDir, err := FetchComponent(opts.Source)
	if err != nil {
		return nil, fmt.Errorf("fetching component: %w", err)
	}
	if cleanupDir != "" {
		defer os.RemoveAll(cleanupDir)
	}

	// 2. Load component definition (angee-component.yaml or component.yaml)
	compPath := ResolveComponentFile(compDir)
	comp, err := config.LoadComponent(compPath)
	if err != nil {
		return nil, fmt.Errorf("loading component: %w", err)
	}

	// 3. Load current stack config
	cfg, err := config.Load(filepath.Join(opts.RootPath, "angee.yaml"))
	if err != nil {
		return nil, fmt.Errorf("loading angee.yaml: %w", err)
	}

	// 4. Check dependencies
	if err := checkRequires(comp, cfg, opts.RootPath); err != nil {
		return nil, err
	}

	// 5. Resolve parameters
	params, err := resolveParams(comp, cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("resolving parameters: %w", err)
	}

	// 6. Render component.yaml as Go template with resolved params
	rendered, err := renderComponent(compPath, params)
	if err != nil {
		return nil, fmt.Errorf("rendering component: %w", err)
	}

	// 7. Parse rendered component
	var renderedComp config.ComponentConfig
	if err := yaml.Unmarshal([]byte(rendered), &renderedComp); err != nil {
		return nil, fmt.Errorf("parsing rendered component: %w", err)
	}

	// 8. Validate no conflicts
	if err := validateNoConflicts(cfg, &renderedComp); err != nil {
		return nil, err
	}

	// 9. Merge into angee.yaml
	record := mergeComponent(cfg, &renderedComp)

	// 9b. Cache credential outputs for compile-time resolution
	if renderedComp.Credential != nil && len(renderedComp.Credential.Outputs) > 0 {
		if err := cacheCredentialOutputs(opts.RootPath, &renderedComp); err != nil {
			return nil, fmt.Errorf("caching credential outputs: %w", err)
		}
	}

	// 9c. Generate secrets and append to .env
	if err := resolveComponentSecrets(opts.RootPath, cfg.Name, &renderedComp, record); err != nil {
		return nil, fmt.Errorf("resolving secrets: %w", err)
	}

	// 10. Write updated angee.yaml
	if err := config.Write(cfg, filepath.Join(opts.RootPath, "angee.yaml")); err != nil {
		return nil, fmt.Errorf("writing angee.yaml: %w", err)
	}

	// 11. Clone repositories
	if err := cloneRepositories(cfg, opts.RootPath); err != nil {
		return nil, fmt.Errorf("cloning repositories: %w", err)
	}

	// 12. Process file manifest
	if err := processFiles(compDir, opts.RootPath, &renderedComp); err != nil {
		return nil, fmt.Errorf("processing files: %w", err)
	}

	// 13. Record installation
	record.Name = comp.Name
	record.Version = comp.Version
	record.Type = comp.Type
	record.Source = opts.Source
	record.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	record.Parameters = params
	if err := writeInstallRecord(opts.RootPath, record); err != nil {
		return nil, fmt.Errorf("writing install record: %w", err)
	}

	return record, nil
}

// FetchComponent resolves a component reference to a local directory.
func FetchComponent(source string) (dir string, cleanupDir string, err error) {
	// Local path — check for component definition file
	if info, statErr := os.Stat(source); statErr == nil && info.IsDir() {
		if hasComponentFile(source) {
			return source, "", nil
		}
	}

	// Check for angee-component.yaml or component.yaml directly (file, not directory)
	if hasComponentFile(source) {
		return source, "", nil
	}

	// Resolve shorthand: bare name "postgres" → look in embedded components first
	if !strings.Contains(source, "://") && !strings.HasPrefix(source, "/") && !strings.HasPrefix(source, ".") && !strings.Contains(source, "/") {
		if embeddedDir, ok := resolveEmbeddedComponent(source); ok {
			return embeddedDir, "", nil
		}
	}

	// Resolve shorthand: "angee/postgres" → "https://github.com/angee-sh/postgres"
	repoURL := source
	if !strings.Contains(source, "://") && !strings.HasPrefix(source, "/") && !strings.HasPrefix(source, ".") {
		parts := strings.SplitN(source, "/", 2)
		if len(parts) == 2 {
			repoURL = fmt.Sprintf("https://github.com/%s/%s", parts[0], parts[1])
		}
	}

	// Git clone
	cloneDir, err := os.MkdirTemp("", "angee-component-*")
	if err != nil {
		return "", "", err
	}
	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, cloneDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(cloneDir)
		return "", "", fmt.Errorf("cloning %s: %w", repoURL, err)
	}
	return cloneDir, cloneDir, nil
}

// hasComponentFile checks if a directory contains a component definition file
// (either angee-component.yaml or component.yaml).
func hasComponentFile(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "angee-component.yaml")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "component.yaml")); err == nil {
		return true
	}
	return false
}

// ResolveComponentFile returns the path to the component definition file in a directory.
func ResolveComponentFile(dir string) string {
	if p := filepath.Join(dir, "angee-component.yaml"); fileExists(p) {
		return p
	}
	return filepath.Join(dir, "component.yaml")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// resolveEmbeddedComponent looks for a component in the angee binary's embedded
// component directory. This is resolved at build time via the CLI's component path.
// Falls back to looking relative to the executable path.
func resolveEmbeddedComponent(name string) (string, bool) {
	// Check ANGEE_COMPONENTS_PATH env var (set by CLI)
	if envPath := os.Getenv("ANGEE_COMPONENTS_PATH"); envPath != "" {
		dir := filepath.Join(envPath, name)
		if hasComponentFile(dir) {
			return dir, true
		}
	}

	// Check relative to executable
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		// Look in ../templates/components/ relative to binary
		dir := filepath.Join(exeDir, "..", "templates", "components", name)
		if hasComponentFile(dir) {
			return dir, true
		}
		// Also check in source tree during development
		dir = filepath.Join(exeDir, "..", "..", "templates", "components", name)
		if hasComponentFile(dir) {
			return dir, true
		}
	}

	return "", false
}

// checkRequires verifies all component dependencies exist in the stack.
func checkRequires(comp *config.ComponentConfig, cfg *config.AngeeConfig, rootPath string) error {
	for _, req := range comp.Requires {
		// Check if any service, agent, or MCP server matches
		name := req
		// Strip namespace prefix: "angee/postgres" → "postgres"
		if idx := strings.LastIndex(req, "/"); idx >= 0 {
			name = req[idx+1:]
		}

		found := false
		if _, ok := cfg.Services[name]; ok {
			found = true
		}
		if _, ok := cfg.Agents[name]; ok {
			found = true
		}
		if _, ok := cfg.MCPServers[name]; ok {
			found = true
		}
		// Also check installed components
		if componentInstalled(rootPath, name) {
			found = true
		}

		if !found {
			return fmt.Errorf("component %q requires %q which is not installed.\nAdd it first: angee add %s", comp.Name, req, req)
		}
	}
	return nil
}

// componentInstalled checks if a component is installed by looking in .angee/components/.
func componentInstalled(rootPath, name string) bool {
	componentsDir := filepath.Join(rootPath, ".angee", "components")
	entries, err := os.ReadDir(componentsDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		// Match by filename: "postgres.yaml" matches name "postgres"
		baseName := strings.TrimSuffix(entry.Name(), ".yaml")
		if baseName == name || baseName == strings.ReplaceAll(name, "/", "-") {
			return true
		}
	}
	return false
}

// resolveParams resolves component parameters from flags, defaults, or prompts.
func resolveParams(comp *config.ComponentConfig, cfg *config.AngeeConfig, opts AddOptions) (map[string]string, error) {
	params := make(map[string]string)

	// Always include stack-level context
	params["ProjectName"] = cfg.Name

	for _, p := range comp.Parameters {
		if val, ok := opts.Params[p.Name]; ok {
			params[p.Name] = val
		} else if p.Default != "" {
			params[p.Name] = p.Default
		} else if p.Required && !opts.Yes {
			if opts.PromptFn != nil {
				val, err := opts.PromptFn(p)
				if err != nil {
					return nil, err
				}
				params[p.Name] = val
			} else {
				return nil, fmt.Errorf("parameter %q is required — provide it with --param %s=<value>", p.Name, p.Name)
			}
		}
	}

	return params, nil
}

// renderComponent renders angee-component.yaml as a Go template with params.
func renderComponent(compPath string, params map[string]string) (string, error) {
	content, err := os.ReadFile(compPath)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("component").Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parsing component template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("rendering component template: %w", err)
	}

	return buf.String(), nil
}

// validateNoConflicts checks that the component won't create naming conflicts.
func validateNoConflicts(cfg *config.AngeeConfig, comp *config.ComponentConfig) error {
	for name := range comp.Services {
		if _, exists := cfg.Services[name]; exists {
			return fmt.Errorf("service %q already exists in angee.yaml", name)
		}
	}
	for name := range comp.Agents {
		if _, exists := cfg.Agents[name]; exists {
			return fmt.Errorf("agent %q already exists in angee.yaml", name)
		}
	}
	return nil
}

// mergeComponent merges the rendered component into the stack config
// and returns an installation record tracking what was added.
func mergeComponent(cfg *config.AngeeConfig, comp *config.ComponentConfig) *config.InstalledComponent {
	record := &config.InstalledComponent{}

	// Repositories
	if cfg.Repositories == nil && len(comp.Repositories) > 0 {
		cfg.Repositories = make(map[string]config.RepositorySpec)
	}
	for k, v := range comp.Repositories {
		cfg.Repositories[k] = v
		record.Added.Repositories = append(record.Added.Repositories, k)
	}

	// Services
	if cfg.Services == nil && len(comp.Services) > 0 {
		cfg.Services = make(map[string]config.ServiceSpec)
	}
	for k, v := range comp.Services {
		cfg.Services[k] = v
		record.Added.Services = append(record.Added.Services, k)
	}

	// MCP Servers
	if cfg.MCPServers == nil && len(comp.MCPServers) > 0 {
		cfg.MCPServers = make(map[string]config.MCPServerSpec)
	}
	for k, v := range comp.MCPServers {
		cfg.MCPServers[k] = v
		record.Added.MCPServers = append(record.Added.MCPServers, k)
	}

	// Agents
	if cfg.Agents == nil && len(comp.Agents) > 0 {
		cfg.Agents = make(map[string]config.AgentSpec)
	}
	for k, v := range comp.Agents {
		cfg.Agents[k] = v
		record.Added.Agents = append(record.Added.Agents, k)
	}

	// Skills
	if cfg.Skills == nil && len(comp.Skills) > 0 {
		cfg.Skills = make(map[string]config.SkillSpec)
	}
	for k, v := range comp.Skills {
		cfg.Skills[k] = v
		record.Added.Skills = append(record.Added.Skills, k)
	}

	// Secrets (append, dedup)
	existing := make(map[string]bool)
	for _, s := range cfg.Secrets {
		existing[s.Name] = true
	}
	for _, s := range comp.Secrets {
		if !existing[s.Name] {
			cfg.Secrets = append(cfg.Secrets, s)
			record.Added.Secrets = append(record.Added.Secrets, s.Name)
			existing[s.Name] = true
		}
	}

	return record
}

// cloneRepositories clones any new repositories defined by the component.
func cloneRepositories(cfg *config.AngeeConfig, rootPath string) error {
	for name, repo := range cfg.Repositories {
		if repo.URL == "" {
			continue
		}
		destPath := repo.Path
		if destPath == "" {
			destPath = filepath.Join("src", name)
		}
		absPath := filepath.Join(rootPath, destPath)

		// Skip if directory already exists and is non-empty
		if entries, err := os.ReadDir(absPath); err == nil && len(entries) > 0 {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}

		args := []string{"clone"}
		if repo.Branch != "" {
			args = append(args, "--branch", repo.Branch)
		}
		args = append(args, repo.URL, absPath)

		cmd := exec.Command("git", args...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cloning %s: %w", repo.URL, err)
		}
	}
	return nil
}

// processFiles handles the component file manifest.
func processFiles(compDir, rootPath string, comp *config.ComponentConfig) error {
	// Copy deploy-time templates to ANGEE_ROOT
	for _, tmplName := range comp.Templates {
		src := filepath.Join(compDir, tmplName)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			src = filepath.Join(compDir, "templates", tmplName)
		}
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		dst := filepath.Join(rootPath, tmplName)
		if _, err := os.Stat(dst); err == nil {
			continue // don't overwrite
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return err
		}
	}

	// Process file manifest
	for _, f := range comp.Files {
		srcPath := filepath.Join(compDir, f.Source)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f.Source, err)
		}

		var dstPath string
		switch f.Target {
		case "workspace":
			agent := f.Agent
			if agent == "" {
				// Infer from path: agents/<name>/... → agent name
				parts := strings.Split(f.Source, "/")
				if len(parts) >= 2 && parts[0] == "agents" {
					agent = parts[1]
				}
			}
			if agent == "" {
				continue
			}
			dstDir := filepath.Join(rootPath, "agents", agent, "workspace")
			if err := os.MkdirAll(dstDir, 0755); err != nil {
				return err
			}
			dstPath = filepath.Join(dstDir, filepath.Base(f.Source))

		case "root":
			dstPath = filepath.Join(rootPath, filepath.Base(f.Source))

		case "config":
			agent := f.Agent
			if agent == "" {
				continue
			}
			dstDir := filepath.Join(rootPath, "agents", agent)
			if err := os.MkdirAll(dstDir, 0755); err != nil {
				return err
			}
			dstPath = filepath.Join(dstDir, filepath.Base(f.Source))

		default:
			dstPath = filepath.Join(rootPath, filepath.Base(f.Source))
		}

		// Don't overwrite existing files
		if _, err := os.Stat(dstPath); err == nil {
			continue
		}

		perm := os.FileMode(0644)
		if f.Executable {
			perm = 0755
		}
		if err := os.WriteFile(dstPath, data, perm); err != nil {
			return fmt.Errorf("writing %s: %w", dstPath, err)
		}
	}

	// Copy agent workspace files from agents/ directory in component
	agentsDir := filepath.Join(compDir, "agents")
	if info, err := os.Stat(agentsDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(agentsDir)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			agentName := entry.Name()
			srcDir := filepath.Join(agentsDir, agentName)
			dstDir := filepath.Join(rootPath, "agents", agentName, "workspace")
			if err := os.MkdirAll(dstDir, 0755); err != nil {
				return err
			}
			files, _ := os.ReadDir(srcDir)
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				srcFile := filepath.Join(srcDir, f.Name())
				dstFile := filepath.Join(dstDir, f.Name())
				if _, err := os.Stat(dstFile); err == nil {
					continue
				}
				d, err := os.ReadFile(srcFile)
				if err != nil {
					continue
				}
				os.WriteFile(dstFile, d, 0644)
			}
		}
	}

	return nil
}

// resolveComponentSecrets generates values for new secrets declared by the component.
// For each secret, it tries to store in OpenBao (if configured and reachable),
// falling back to appending to the .env file. Existing secrets are not overwritten.
func resolveComponentSecrets(rootPath, projectName string, comp *config.ComponentConfig, record *config.InstalledComponent) error {
	if len(comp.Secrets) == 0 {
		return nil
	}

	envPath := filepath.Join(rootPath, ".env")
	existing := loadEnvKeys(envPath)

	// Load angee.yaml to check for OpenBao config
	cfg, _ := config.Load(filepath.Join(rootPath, "angee.yaml"))

	// First pass: generate non-derived secrets
	resolved := make(map[string]string)
	var envEntries []string // only secrets that fell through to .env

	for _, s := range comp.Secrets {
		envKey := secretToEnvKey(s.Name)
		if existing[envKey] {
			resolved[s.Name] = readEnvValue(envPath, envKey)
			continue
		}

		if s.Derived != "" {
			continue // handle in second pass
		}

		if s.Generated {
			value, err := generateSecret(s.Length, s.Charset)
			if err != nil {
				return fmt.Errorf("generating %s: %w", s.Name, err)
			}
			resolved[s.Name] = value

			// Try OpenBao, fall back to .env
			if cfg != nil {
				backend, _ := secrets.StoreSecret(context.Background(), cfg, rootPath, s.Name, value)
				if backend == "openbao" {
					continue // stored in OpenBao, skip .env append
				}
			}
			envEntries = append(envEntries, fmt.Sprintf("%s=%s", envKey, value))
		}
	}

	// Second pass: derived secrets
	for _, s := range comp.Secrets {
		envKey := secretToEnvKey(s.Name)
		if existing[envKey] || s.Derived == "" {
			continue
		}
		value := s.Derived
		for k, v := range resolved {
			value = strings.ReplaceAll(value, "${"+k+"}", v)
		}
		value = strings.ReplaceAll(value, "${project}", projectName)
		resolved[s.Name] = value

		// Try OpenBao, fall back to .env
		if cfg != nil {
			backend, _ := secrets.StoreSecret(context.Background(), cfg, rootPath, s.Name, value)
			if backend == "openbao" {
				continue
			}
		}
		envEntries = append(envEntries, fmt.Sprintf("%s=%s", envKey, value))
	}

	// Append remaining entries to .env
	if len(envEntries) > 0 {
		f, err := os.OpenFile(envPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		content := "\n# Added by: angee add " + comp.Name + "\n"
		for _, entry := range envEntries {
			content += entry + "\n"
		}
		if _, err := f.WriteString(content); err != nil {
			return err
		}
	}

	return nil
}

// secretToEnvKey converts a secret name to an env var key: "db-password" → "DB_PASSWORD"
func secretToEnvKey(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// loadEnvKeys reads an .env file and returns a set of defined variable names.
func loadEnvKeys(path string) map[string]bool {
	keys := make(map[string]bool)
	data, err := os.ReadFile(path)
	if err != nil {
		return keys
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			keys[line[:idx]] = true
		}
	}
	return keys
}

// readEnvValue reads a specific env var's value from a .env file.
func readEnvValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return line[len(key)+1:]
		}
	}
	return ""
}

// generateSecret creates a cryptographically random string.
func generateSecret(length int, charset string) (string, error) {
	if charset == "" {
		charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	}
	if length == 0 {
		length = 32
	}
	runes := []rune(charset)
	result := make([]rune, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(runes))))
		if err != nil {
			return "", err
		}
		result[i] = runes[n.Int64()]
	}
	return string(result), nil
}

// cacheCredentialOutputs writes the full ComponentConfig to .angee/credential-outputs/
// so the compiler can resolve credential_bindings at deploy time.
func cacheCredentialOutputs(rootPath string, comp *config.ComponentConfig) error {
	dir := filepath.Join(rootPath, ".angee", "credential-outputs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(comp)
	if err != nil {
		return err
	}
	filename := strings.ReplaceAll(comp.Credential.Name, "/", "-") + ".yaml"
	return os.WriteFile(filepath.Join(dir, filename), data, 0644)
}

// writeInstallRecord writes the component installation record to .angee/components/.
func writeInstallRecord(rootPath string, record *config.InstalledComponent) error {
	dir := filepath.Join(rootPath, ".angee", "components")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Sanitize name for filename: "fyltr/fyltr-django" → "fyltr-fyltr-django"
	filename := strings.ReplaceAll(record.Name, "/", "-") + ".yaml"
	data, err := yaml.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, filename), data, 0644)
}
