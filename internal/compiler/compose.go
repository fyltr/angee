// Package compiler translates angee.yaml into a docker-compose.yaml.
package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fyltr/angee/internal/config"
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

// AgentServicePrefix is the docker-compose service name prefix for agents.
// An agent named "admin" becomes the service "agent-admin".
const AgentServicePrefix = "agent-"

// Compiler translates an AngeeConfig into a ComposeFile.
type Compiler struct {
	Network              string                            // docker network name
	RootPath             string                            // path to ANGEE_ROOT (for volume mounts)
	APIKey               string                            // operator API key, auto-injected into operator-role agents
	CredentialOutputs    map[string][]config.CredentialOutput // credential name → outputs (loaded from installed components)
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
		cs, err := c.compileAgent(name, agent, cfg)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		out.Services[AgentServicePrefix+name] = cs
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
	cs.Restart = restartPolicy(svc.Lifecycle)

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

	// Health checks are performed by the operator (Kubernetes-style HTTP
	// probes from outside the container), so we do not generate Docker
	// HEALTHCHECK directives. This avoids requiring curl/wget inside the
	// target image.

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

func (c *Compiler) compileAgent(name string, agent config.AgentSpec, cfg *config.AngeeConfig) (ComposeService, error) {
	mcpServers := cfg.MCPServers
	image := agent.Image
	if image == "" {
		// Default agent image
		image = "ghcr.io/fyltr/angee-agent:latest"
	}

	cs := ComposeService{
		Image:     image,
		Command:   agent.Command,
		Networks:  []string{c.Network},
		Restart:   restartPolicy(agent.Lifecycle),
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

	// Per-agent .env file (written by operator at start time)
	agentEnvFile := fmt.Sprintf("./agents/%s/.env", name)
	cs.EnvFile = []string{agentEnvFile}

	// Workspace volume mount
	if agent.Workspace.Path != "" {
		// Explicit path (e.g. ANGEE_ROOT itself for the admin agent)
		cs.Volumes = append(cs.Volumes, ensureBindMountPrefix(agent.Workspace.Path)+":/workspace")
	} else if agent.Workspace.Repository != "" {
		// Mount the repository directory as workspace
		repoPath := fmt.Sprintf("src/%s", agent.Workspace.Repository)
		if spec, ok := cfg.Repositories[agent.Workspace.Repository]; ok && spec.Path != "" {
			repoPath = spec.Path
		}
		cs.Volumes = append(cs.Volumes, ensureBindMountPrefix(repoPath)+":/workspace")
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

	// Auto-inject operator API key for operator-role agents
	if agent.Role == "operator" && c.APIKey != "" {
		cs.Environment = append(cs.Environment, "ANGEE_OPERATOR_API_KEY="+c.APIKey)
	}

	// Resolve skills: merge skill MCP servers and system prompts
	resolvedMCPServers := agent.MCPServers
	if len(agent.Skills) > 0 && cfg.Skills != nil {
		for _, skillName := range agent.Skills {
			skill, ok := cfg.Skills[skillName]
			if !ok {
				continue
			}
			for _, srv := range skill.MCPServers {
				if !contains(resolvedMCPServers, srv) {
					resolvedMCPServers = append(resolvedMCPServers, srv)
				}
			}
			if skill.SystemPrompt != "" {
				if cs.Environment == nil {
					cs.Environment = []string{}
				}
				// Append skill system prompt to agent's system prompt
				cs.Environment = append(cs.Environment, "ANGEE_SKILL_PROMPT_"+strings.ToUpper(strings.ReplaceAll(skillName, "-", "_"))+"="+skill.SystemPrompt)
			}
		}
	}

	// MCP server environment vars
	var mcpList []string
	for _, mcpName := range resolvedMCPServers {
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

	// Credential binding resolution: resolve credential outputs into env vars and file mounts.
	if len(agent.CredentialBindings) > 0 && c.CredentialOutputs != nil {
		for _, credName := range agent.CredentialBindings {
			outputs, ok := c.CredentialOutputs[credName]
			if !ok {
				continue
			}
			for _, out := range outputs {
				switch out.Type {
				case "env":
					if out.Key != "" {
						// Inject as ${secret:credName} → resolves to env var via the secrets backend.
						// The actual value comes from the credential stored in vault/env.
						cs.Environment = append(cs.Environment, out.Key+"="+resolveSecretRefs("${secret:"+credName+"}"))
					}
				case "file":
					if out.Template != "" && out.Mount != "" {
						// Add a file mount for credential config files.
						src := fmt.Sprintf("./agents/%s/%s", name, filepath.Base(out.Mount))
						cs.Volumes = append(cs.Volumes, src+":"+out.Mount+":ro")
					}
				}
			}
		}
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

// restartPolicy returns the docker-compose restart policy for a lifecycle value.
func restartPolicy(lifecycle string) string {
	if lifecycle == config.LifecycleSystem {
		return "always"
	}
	return "unless-stopped"
}

// contains returns true if s is in the slice.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
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

// LoadCredentialOutputs scans installed component records for credential
// definitions and builds a map of credential name → outputs. This is used
// by the compiler to resolve agent credential_bindings at compile time.
func LoadCredentialOutputs(rootPath string) map[string][]config.CredentialOutput {
	outputs := make(map[string][]config.CredentialOutput)
	compDir := filepath.Join(rootPath, ".angee", "components")
	entries, err := os.ReadDir(compDir)
	if err != nil {
		return outputs
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(compDir, entry.Name()))
		if err != nil {
			continue
		}
		var record config.InstalledComponent
		if err := yaml.Unmarshal(data, &record); err != nil {
			continue
		}
		if record.Type != "credential" {
			continue
		}
		// Load the original component to get credential outputs.
		// The install record tracks what was added, but the actual credential
		// definition outputs need to be stored separately. For now, we look
		// for the credential definition in the angee.yaml secrets/credential
		// structure, or we can load from a cached component config.
		// Simplified: store credential outputs in the install record.
		// See the CredentialOutputs field below.
	}

	// Also scan for credential components that have a cached component.yaml
	// in .angee/components/<name>/
	cacheDir := filepath.Join(rootPath, ".angee", "credential-outputs")
	entries, err = os.ReadDir(cacheDir)
	if err != nil {
		return outputs
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cacheDir, entry.Name()))
		if err != nil {
			continue
		}
		var comp config.ComponentConfig
		if err := yaml.Unmarshal(data, &comp); err != nil {
			continue
		}
		if comp.Credential != nil && len(comp.Credential.Outputs) > 0 {
			outputs[comp.Credential.Name] = comp.Credential.Outputs
		}
	}

	return outputs
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
