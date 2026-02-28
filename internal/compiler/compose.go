// Package compiler translates angee.yaml into a docker-compose.yaml.
package compiler

import (
	"fmt"
	"os"
	"path/filepath"
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
	StdinOpen   bool              `yaml:"stdin_open,omitempty"`
	Tty         bool              `yaml:"tty,omitempty"`
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
				Memory: normalizeMemory(svc.Resources.Memory),
			},
		}
	}

	// Environment — translate ${secret:name} → ${ENV_NAME} for docker-compose interpolation
	for k, v := range svc.Env {
		cs.Environment = append(cs.Environment, k+"="+resolveSecretRefs(v))
	}

	// Ports
	cs.Ports = svc.Ports

	// Volumes
	for _, v := range svc.Volumes {
		if v.Name != "" {
			cs.Volumes = append(cs.Volumes, v.Name+":"+v.Path)
		}
	}
	cs.Volumes = append(cs.Volumes, svc.RawVolumes...)

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
		Image:     image,
		Command:   agent.Command,
		Networks:  []string{c.Network},
		Restart:   "unless-stopped",
		StdinOpen: true,
		Tty:       true,
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
	agentEnvFile := fmt.Sprintf("./agents/%s/.env", name)
	cs.EnvFile = []string{agentEnvFile}

	// Workspace volume mount
	if agent.Workspace.Path != "" {
		// Explicit path (e.g. ANGEE_ROOT itself for the admin agent)
		cs.Volumes = append(cs.Volumes, ensureBindMountPrefix(agent.Workspace.Path)+":/workspace")
	} else {
		// Per-agent workspace directory
		workspacePath := fmt.Sprintf("./agents/%s/workspace", name)
		cs.Volumes = append(cs.Volumes, workspacePath+":/workspace")
	}

	// Agent-declared environment variables (e.g. ANTHROPIC_API_KEY)
	for k, v := range agent.Env {
		cs.Environment = append(cs.Environment, k+"="+resolveSecretRefs(v))
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

	// Agent-declared config file mounts.
	// Template files under /workspace/ are written into the workspace dir by
	// RenderAgentFiles, so they don't need a separate volume mount (they're
	// already part of the workspace bind mount above).
	for _, f := range agent.Files {
		if f.Template != "" {
			if isUnderWorkspace(f.Mount) {
				continue // already inside workspace bind mount
			}
			src := fmt.Sprintf("./agents/%s/%s", name, filepath.Base(f.Mount))
			cs.Volumes = append(cs.Volumes, src+":"+f.Mount+":ro")
		} else if f.Source != "" {
			src := ExpandHome(f.Source)
			if f.Optional {
				if _, err := os.Stat(src); os.IsNotExist(err) {
					continue
				}
			}
			cs.Volumes = append(cs.Volumes, src+":"+f.Mount+":ro")
		}
	}

	// Resources
	if agent.Resources.CPU != "" || agent.Resources.Memory != "" {
		cs.Deploy = &ComposeDeploy{
			Resources: ComposeResources{
				Limits: ComposeResourceValues{
					CPUs:   agent.Resources.CPU,
					Memory: normalizeMemory(agent.Resources.Memory),
				},
			},
		}
	}

	return cs, nil
}

// ensureBindMountPrefix ensures a relative path starts with "./" so Docker
// Compose treats it as a bind mount rather than a named volume reference.
func ensureBindMountPrefix(p string) string {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
		return p
	}
	return "./" + p
}

// normalizeMemory converts Kubernetes-style IEC suffixes to Docker Compose
// compatible suffixes: "1Gi" → "1g", "256Mi" → "256m", "512Ki" → "512k".
// Values already using Docker suffixes (e.g. "512m") are left unchanged.
func normalizeMemory(v string) string {
	replacements := []struct{ from, to string }{
		{"Gi", "g"},
		{"Mi", "m"},
		{"Ki", "k"},
	}
	for _, r := range replacements {
		if strings.HasSuffix(v, r.from) {
			return strings.TrimSuffix(v, r.from) + r.to
		}
	}
	return v
}

// resolveSecretRefs translates ${secret:name} references into ${ENV_NAME}
// so Docker Compose can interpolate them from the .env file.
// e.g. ${secret:db-url} → ${DB_URL}, ${secret:django-secret-key} → ${DJANGO_SECRET_KEY}
func resolveSecretRefs(v string) string {
	for {
		start := strings.Index(v, "${secret:")
		if start == -1 {
			return v
		}
		end := strings.Index(v[start:], "}")
		if end == -1 {
			return v
		}
		end += start
		secretName := v[start+len("${secret:") : end]
		envKey := strings.ToUpper(strings.ReplaceAll(secretName, "-", "_"))
		v = v[:start] + "${" + envKey + "}" + v[end+1:]
	}
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
