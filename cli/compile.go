package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fyltr/angee/internal/compiler"
	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/root"
)

// localCompile loads angee.yaml, validates, renders agent files, compiles
// docker-compose.yaml, and returns the compose path and project name.
// Used by up, pull, restart — the commands that run without the operator.
func localCompile(rootPath string) (composePath, projectName string, err error) {
	cfg, err := config.Load(filepath.Join(rootPath, "angee.yaml"))
	if err != nil {
		return "", "", fmt.Errorf("loading angee.yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return "", "", err
	}

	projectName = cfg.Name
	if projectName == "" {
		projectName = "angee"
	}

	r, err := root.Open(rootPath)
	if err != nil {
		return "", "", fmt.Errorf("opening ANGEE_ROOT: %w", err)
	}

	for agentName, agent := range cfg.Agents {
		if err := r.EnsureAgentDir(agentName); err != nil {
			return "", "", fmt.Errorf("creating agent dir %s: %w", agentName, err)
		}
		if err := compiler.RenderAgentFiles(rootPath, r.AgentDir(agentName), agent, cfg.MCPServers); err != nil {
			return "", "", fmt.Errorf("agent files for %s: %w", agentName, err)
		}
	}

	opCfg, err := r.LoadOperatorConfig()
	if err != nil {
		return "", "", fmt.Errorf("loading operator config: %w", err)
	}

	comp := compiler.New(rootPath, opCfg.Docker.Network)
	if opCfg.APIKey != "" {
		comp.APIKey = opCfg.APIKey
	} else if key := os.Getenv("ANGEE_API_KEY"); key != "" {
		comp.APIKey = key
	}

	cf, err := comp.Compile(cfg)
	if err != nil {
		return "", "", fmt.Errorf("compiling: %w", err)
	}

	composePath = filepath.Join(rootPath, "docker-compose.yaml")
	if err := compiler.Write(cf, composePath); err != nil {
		return "", "", fmt.Errorf("writing docker-compose.yaml: %w", err)
	}

	return composePath, projectName, nil
}
