// Package config defines the types for angee.yaml and operator.yaml.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AngeeConfig is the top-level structure of angee.yaml.
type AngeeConfig struct {
	Name         string                     `yaml:"name"`
	Version      string                     `yaml:"version,omitempty"`
	Repositories map[string]RepositorySpec  `yaml:"repositories,omitempty"`
	Services     map[string]ServiceSpec     `yaml:"services,omitempty"`
	MCPServers   map[string]MCPServerSpec   `yaml:"mcp_servers,omitempty"`
	Agents       map[string]AgentSpec       `yaml:"agents,omitempty"`
	Secrets      []SecretRef               `yaml:"secrets,omitempty"`
}

// RepositorySpec defines a source repository linked to the project.
type RepositorySpec struct {
	URL    string `yaml:"url"`
	Branch string `yaml:"branch,omitempty"`
	Role   string `yaml:"role,omitempty"` // base | custom | dependency
}

// SecretRef declares a secret that must exist before deploy.
type SecretRef struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
}

// Lifecycle values for services and agents.
const (
	LifecyclePlatform = "platform" // web-facing, Traefik labels
	LifecycleSidecar  = "sidecar"  // internal only, no ports exposed
	LifecycleWorker   = "worker"   // background processing
	LifecycleSystem   = "system"   // always-on system agent
	LifecycleAgent    = "agent"    // AI agent
	LifecycleJob      = "job"      // one-shot / scheduled
)

// ServiceSpec defines a platform service (web, DB, workers, etc.).
type ServiceSpec struct {
	Image     string            `yaml:"image,omitempty"`
	Build     *BuildSpec        `yaml:"build,omitempty"`
	Command   string            `yaml:"command,omitempty"`
	Lifecycle string            `yaml:"lifecycle,omitempty"`
	Domains   []DomainSpec      `yaml:"domains,omitempty"`
	Resources ResourceSpec      `yaml:"resources,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	Volumes   []VolumeSpec      `yaml:"volumes,omitempty"`
	Health    *HealthSpec       `yaml:"health,omitempty"`
	Replicas  int               `yaml:"replicas,omitempty"`
	DependsOn []string          `yaml:"depends_on,omitempty"`
}

// BuildSpec defines how to build a service image from source.
type BuildSpec struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
	Target     string `yaml:"target,omitempty"`
}

// DomainSpec defines a domain/port binding for a service.
type DomainSpec struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port,omitempty"`
	TLS  bool   `yaml:"tls,omitempty"`
}

// ResourceSpec defines CPU and memory constraints.
type ResourceSpec struct {
	CPU    string `yaml:"cpu,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}

// VolumeSpec defines a persistent volume mount.
type VolumeSpec struct {
	Name       string `yaml:"name"`
	Path       string `yaml:"path"`
	Size       string `yaml:"size,omitempty"`
	Persistent bool   `yaml:"persistent,omitempty"`
}

// HealthSpec defines an HTTP health check.
type HealthSpec struct {
	Path     string `yaml:"path"`
	Port     int    `yaml:"port,omitempty"`
	Interval string `yaml:"interval,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
}

// MCPServerSpec defines an MCP server that agents can connect to.
type MCPServerSpec struct {
	Transport   string            `yaml:"transport"` // stdio | sse | streamable-http
	URL         string            `yaml:"url,omitempty"`
	Image       string            `yaml:"image,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
	Args        []string          `yaml:"args,omitempty"`
	Credentials CredentialSpec    `yaml:"credentials,omitempty"`
}

// CredentialSpec defines how credentials are sourced for an MCP server.
type CredentialSpec struct {
	Source      string   `yaml:"source"`       // connect.account | service_account | sso | none
	AccountType string   `yaml:"account_type,omitempty"`
	RunAs       string   `yaml:"run_as,omitempty"`
	Scopes      []string `yaml:"scopes,omitempty"`
}

// AgentSpec defines an AI agent.
type AgentSpec struct {
	Image        string        `yaml:"image,omitempty"`
	Template     string        `yaml:"template,omitempty"`
	Version      string        `yaml:"version,omitempty"`
	Lifecycle    string        `yaml:"lifecycle,omitempty"`  // system | on-demand
	Role         string        `yaml:"role,omitempty"`       // operator | user
	MCPServers   []string      `yaml:"mcp_servers,omitempty"`
	RunAs        string        `yaml:"run_as,omitempty"`
	Workspace    WorkspaceSpec `yaml:"workspace,omitempty"`
	Resources    ResourceSpec  `yaml:"resources,omitempty"`
	SystemPrompt string        `yaml:"system_prompt,omitempty"`
	Description  string        `yaml:"description,omitempty"`
}

// WorkspaceSpec defines the git workspace for an agent.
type WorkspaceSpec struct {
	Path       string `yaml:"path,omitempty"`       // explicit path (for system agents)
	Repository string `yaml:"repository,omitempty"` // which repo from repositories:
	Branch     string `yaml:"branch,omitempty"`
	Persistent bool   `yaml:"persistent,omitempty"`
}

// Load reads and parses an angee.yaml file.
func Load(path string) (*AngeeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading angee.yaml: %w", err)
	}
	var cfg AngeeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing angee.yaml: %w", err)
	}
	return &cfg, nil
}

// Write serializes an AngeeConfig to a YAML file.
func Write(cfg *AngeeConfig, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serializing angee.yaml: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
