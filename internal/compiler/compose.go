// Package compiler translates angee.yaml into a docker-compose.yaml.
package compiler

import (
	"fmt"
	"os"
	"strings"

	"github.com/fyltr/angee-go/internal/config"
	"gopkg.in/yaml.v3"
)

// ComposeFile mirrors the docker-compose.yaml top-level structure.
type ComposeFile struct {
	Name     string                     `yaml:"name,omitempty"`
	Services map[string]ComposeService  `yaml:"services"`
	Volumes  map[string]ComposeVolume   `yaml:"volumes,omitempty"`
	Networks map[string]ComposeNetwork  `yaml:"networks,omitempty"`
}

// ComposeService mirrors a docker-compose service definition.
type ComposeService struct {
	Image       string            `yaml:"image,omitempty"`
	Build       *ComposeBuild     `yaml:"build,omitempty"`
	Command     string            `yaml:"command,omitempty"`
	Restart     string            `yaml:"restart,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Environment []string          `yaml:"environment,omitempty"`
	EnvFile     []string          `yaml:"env_file,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Networks    []string          `yaml:"networks,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	DependsOn   []string          `yaml:"depends_on,omitempty"`
	Healthcheck *ComposeHealthcheck `yaml:"healthcheck,omitempty"`
	Deploy      *ComposeDeploy    `yaml:"deploy,omitempty"`
}

type ComposeBuild struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
	Target     string `yaml:"target,omitempty"`
}

type ComposeHealthcheck struct {
	Test     []string `yaml:"test"`
	Interval string   `yaml:"interval,omitempty"`
	Timeout  string   `yaml:"timeout,omitempty"`
	Retries  int      `yaml:"retries,omitempty"`
}

type ComposeDeploy struct {
	Replicas  int               `yaml:"replicas,omitempty"`
	Resources ComposeResources  `yaml:"resources,omitempty"`
}

type ComposeResources struct {
	Limits   ComposeResourceValues `yaml:"limits,omitempty"`
	Reservations ComposeResourceValues `yaml:"reservations,omitempty"`
}

type ComposeResourceValues struct {
	CPUs   string `yaml:"cpus,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}

type ComposeVolume struct {
	Driver string `yaml:"driver,omitempty"`
}

type ComposeNetwork struct {
	External bool   `yaml:"external,omitempty"`
	Driver   string `yaml:"driver,omitempty"`
}

// Compiler translates an AngeeConfig into a ComposeFile.
type Compiler struct {
	Network  string // docker network name
	RootPath string // path to ANGEE_ROOT (for volume mounts)
}

// New creates a Compiler with the given settings.
func New(rootPath, network string) *Compiler {
	if network == "" {
		network = "angee-net"
	}
	return &Compiler{Network: network, RootPath: rootPath}
}

// Compile translates angee.yaml → docker-compose.yaml structure.
func (c *Compiler) Compile(cfg *config.AngeeConfig) (*ComposeFile, error) {
	out := &ComposeFile{
		Name:     cfg.Name,
		Services: make(map[string]ComposeService),
		Volumes:  make(map[string]ComposeVolume),
		Networks: map[string]ComposeNetwork{
			c.Network: {Driver: "bridge"},
		},
	}

	// Compile services
	for name, svc := range cfg.Services {
		cs, err := c.compileService(name, svc)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", name, err)
		}
		out.Services[name] = cs

		// Register named volumes
		for _, v := range svc.Volumes {
			if v.Persistent || v.Name != "" {
				out.Volumes[v.Name] = ComposeVolume{}
			}
		}
	}

	// Compile agents
	for name, agent := range cfg.Agents {
		cs, err := c.compileAgent(name, agent, cfg.MCPServers)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		out.Services["agent-"+name] = cs
	}

	return out, nil
}

func (c *Compiler) compileService(name string, svc config.ServiceSpec) (ComposeService, error) {
	cs := ComposeService{
		Image:    svc.Image,
		Networks: []string{c.Network},
		Labels:   map[string]string{"angee.managed": "true", "angee.service": name},
	}

	if svc.Build != nil {
		cs.Build = &ComposeBuild{
			Context:    svc.Build.Context,
			Dockerfile: svc.Build.Dockerfile,
			Target:     svc.Build.Target,
		}
	}

	if svc.Command != "" {
		cs.Command = svc.Command
	}

	// Lifecycle → restart policy and labels
	cs.Labels["angee.lifecycle"] = svc.Lifecycle
	switch svc.Lifecycle {
	case config.LifecyclePlatform:
		cs.Restart = "unless-stopped"
	case config.LifecycleSidecar:
		cs.Restart = "unless-stopped"
	case config.LifecycleWorker:
		cs.Restart = "unless-stopped"
	case config.LifecycleSystem:
		cs.Restart = "always"
	default:
		cs.Restart = "unless-stopped"
	}

	// Replicas
	if svc.Replicas > 0 {
		cs.Deploy = &ComposeDeploy{Replicas: svc.Replicas}
	}

	// Resources
	if svc.Resources.CPU != "" || svc.Resources.Memory != "" {
		if cs.Deploy == nil {
			cs.Deploy = &ComposeDeploy{}
		}
		cs.Deploy.Resources = ComposeResources{
			Limits: ComposeResourceValues{
				CPUs:   svc.Resources.CPU,
				Memory: svc.Resources.Memory,
			},
		}
	}

	// Environment
	for k, v := range svc.Env {
		cs.Environment = append(cs.Environment, k+"="+v)
	}

	// Volumes
	for _, v := range svc.Volumes {
		if v.Name != "" {
			cs.Volumes = append(cs.Volumes, v.Name+":"+v.Path)
		}
	}

	// Health check
	if svc.Health != nil {
		port := svc.Health.Port
		if port == 0 {
			port = 8000
		}
		interval := svc.Health.Interval
		if interval == "" {
			interval = "30s"
		}
		timeout := svc.Health.Timeout
		if timeout == "" {
			timeout = "5s"
		}
		cs.Healthcheck = &ComposeHealthcheck{
			Test:     []string{"CMD-SHELL", fmt.Sprintf("wget -q -O - http://localhost:%d%s || exit 1", port, svc.Health.Path)},
			Interval: interval,
			Timeout:  timeout,
			Retries:  3,
		}
	}

	// Traefik labels for platform services with domains
	if svc.Lifecycle == config.LifecyclePlatform && len(svc.Domains) > 0 {
		cs.Labels["traefik.enable"] = "true"
		for _, d := range svc.Domains {
			port := d.Port
			if port == 0 {
				port = 8000
			}
			routerName := name
			cs.Labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", d.Host)
			cs.Labels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", routerName)] = fmt.Sprintf("%d", port)
			if d.TLS {
				cs.Labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = "websecure"
				cs.Labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", routerName)] = "letsencrypt"
			} else {
				cs.Labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = "web"
			}
		}
	}

	// DependsOn
	cs.DependsOn = svc.DependsOn

	return cs, nil
}

func (c *Compiler) compileAgent(name string, agent config.AgentSpec, mcpServers map[string]config.MCPServerSpec) (ComposeService, error) {
	image := agent.Image
	if image == "" {
		// Default agent image
		image = "ghcr.io/fyltr/angee-agent:latest"
	}

	cs := ComposeService{
		Image:    image,
		Networks: []string{c.Network},
		Restart:  "unless-stopped",
		Labels: map[string]string{
			"angee.managed":   "true",
			"angee.type":      "agent",
			"angee.agent":     name,
			"angee.lifecycle": agent.Lifecycle,
			"angee.role":      agent.Role,
		},
	}

	if agent.Lifecycle == config.LifecycleSystem {
		cs.Restart = "always"
	}

	// Per-agent .env file (written by operator at start time)
	agentEnvFile := fmt.Sprintf("agents/%s/.env", name)
	cs.EnvFile = []string{agentEnvFile}

	// Workspace volume mount
	if agent.Workspace.Path != "" {
		// Explicit path (e.g. ANGEE_ROOT itself for the admin agent)
		cs.Volumes = append(cs.Volumes, agent.Workspace.Path+":/workspace")
	} else {
		// Per-agent workspace directory
		workspacePath := fmt.Sprintf("agents/%s/workspace", name)
		cs.Volumes = append(cs.Volumes, workspacePath+":/workspace")
	}

	// System prompt via environment
	if agent.SystemPrompt != "" {
		cs.Environment = append(cs.Environment, "ANGEE_SYSTEM_PROMPT="+agent.SystemPrompt)
	}

	cs.Environment = append(cs.Environment, "ANGEE_AGENT_NAME="+name)

	// MCP server environment vars
	var mcpList []string
	for _, mcpName := range agent.MCPServers {
		mcpSpec, ok := mcpServers[mcpName]
		if !ok {
			continue
		}
		mcpList = append(mcpList, mcpName)
		if mcpSpec.Transport == "sse" || mcpSpec.Transport == "streamable-http" {
			// Inject URL as env var
			envKey := "ANGEE_MCP_" + strings.ToUpper(strings.ReplaceAll(mcpName, "-", "_")) + "_URL"
			cs.Environment = append(cs.Environment, envKey+"="+mcpSpec.URL)
		}
	}
	if len(mcpList) > 0 {
		cs.Environment = append(cs.Environment, "ANGEE_MCP_SERVERS="+strings.Join(mcpList, ","))
	}

	// Resources
	if agent.Resources.CPU != "" || agent.Resources.Memory != "" {
		cs.Deploy = &ComposeDeploy{
			Resources: ComposeResources{
				Limits: ComposeResourceValues{
					CPUs:   agent.Resources.CPU,
					Memory: agent.Resources.Memory,
				},
			},
		}
	}

	return cs, nil
}

// Write serializes a ComposeFile to a YAML file.
func Write(cf *ComposeFile, path string) error {
	data, err := yaml.Marshal(cf)
	if err != nil {
		return fmt.Errorf("serializing docker-compose.yaml: %w", err)
	}
	header := "# Generated by angee-operator. DO NOT EDIT MANUALLY.\n# Source of truth: angee.yaml\n\n"
	return os.WriteFile(path, append([]byte(header), data...), 0644)
}
