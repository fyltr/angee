package component

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fyltr/angee/internal/config"
	"gopkg.in/yaml.v3"
)

// RemoveOptions configures the angee remove operation.
type RemoveOptions struct {
	Name     string // component name: "fyltr/fyltr-django"
	Deploy   bool   // deploy after removing
	Force    bool   // skip confirmation
	RootPath string // ANGEE_ROOT path
}

// Remove uninstalls a component from the stack.
func Remove(opts RemoveOptions) error {
	// 1. Load installation record
	record, err := loadInstallRecord(opts.RootPath, opts.Name)
	if err != nil {
		return fmt.Errorf("component %q is not installed: %w", opts.Name, err)
	}

	// 2. Load current stack config
	cfgPath := filepath.Join(opts.RootPath, "angee.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading angee.yaml: %w", err)
	}

	// 3. Remove entries that were added by this component
	removeEntries(cfg, record)

	// 4. Write updated angee.yaml
	if err := config.Write(cfg, cfgPath); err != nil {
		return fmt.Errorf("writing angee.yaml: %w", err)
	}

	// 5. Clean up files
	cleanupFiles(opts.RootPath, record)

	// 6. Remove installation record
	deleteInstallRecord(opts.RootPath, opts.Name)

	return nil
}

// List returns all installed components.
func List(rootPath string) ([]*config.InstalledComponent, error) {
	dir := filepath.Join(rootPath, ".angee", "components")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var components []*config.InstalledComponent
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var comp config.InstalledComponent
		if err := yaml.Unmarshal(data, &comp); err != nil {
			continue
		}
		components = append(components, &comp)
	}
	return components, nil
}

func removeEntries(cfg *config.AngeeConfig, record *config.InstalledComponent) {
	for _, name := range record.Added.Services {
		delete(cfg.Services, name)
	}
	for _, name := range record.Added.MCPServers {
		delete(cfg.MCPServers, name)
	}
	for _, name := range record.Added.Agents {
		delete(cfg.Agents, name)
	}
	for _, name := range record.Added.Skills {
		delete(cfg.Skills, name)
	}
	for _, name := range record.Added.Repositories {
		delete(cfg.Repositories, name)
	}

	// Remove secrets by name
	secretsToRemove := make(map[string]bool)
	for _, name := range record.Added.Secrets {
		secretsToRemove[name] = true
	}
	var kept []config.SecretRef
	for _, s := range cfg.Secrets {
		if !secretsToRemove[s.Name] {
			kept = append(kept, s)
		}
	}
	cfg.Secrets = kept
}

func cleanupFiles(rootPath string, record *config.InstalledComponent) {
	for _, f := range record.Added.Files {
		path := filepath.Join(rootPath, f)
		os.Remove(path)
	}
}

func loadInstallRecord(rootPath, name string) (*config.InstalledComponent, error) {
	filename := strings.ReplaceAll(name, "/", "-") + ".yaml"
	path := filepath.Join(rootPath, ".angee", "components", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record config.InstalledComponent
	if err := yaml.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func deleteInstallRecord(rootPath, name string) {
	filename := strings.ReplaceAll(name, "/", "-") + ".yaml"
	os.Remove(filepath.Join(rootPath, ".angee", "components", filename))
}
