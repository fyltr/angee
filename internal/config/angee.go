// Package config defines the types for angee.yaml and operator.yaml.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AngeeConfig is the target top-level structure of angee.yaml.
type AngeeConfig struct {
	Version    string                   `yaml:"version,omitempty" json:"version,omitempty"`
	Kind       string                   `yaml:"kind,omitempty" json:"kind,omitempty"`
	Name       string                   `yaml:"name" json:"name"`
	Template   *TemplateSpec            `yaml:"template,omitempty" json:"template,omitempty"`
	Operator   *OperatorRuntimeSpec     `yaml:"operator,omitempty" json:"operator,omitempty"`
	Deployment *DeploymentSpec          `yaml:"deployment,omitempty" json:"deployment,omitempty"`
	Runtime    *RuntimeSpec             `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Sources    map[string]SourceSpec    `yaml:"sources,omitempty" json:"sources,omitempty"`
	Volumes    map[string]VolumeSpec    `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Secrets    map[string]SecretSpec    `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	PortLeases map[string]PortLeaseSpec `yaml:"port_leases,omitempty" json:"port_leases,omitempty"`
	Services   map[string]ServiceSpec   `yaml:"services,omitempty" json:"services,omitempty"`
	Jobs       map[string]JobSpec       `yaml:"jobs,omitempty" json:"jobs,omitempty"`
	Workflows  map[string]WorkflowSpec  `yaml:"workflows,omitempty" json:"workflows,omitempty"`
	Workspaces *WorkspaceRegistrySpec   `yaml:"workspaces,omitempty" json:"workspaces,omitempty"`
	Agents     *AgentRegistrySpec       `yaml:"agents,omitempty" json:"agents,omitempty"`
	MCPServers map[string]MCPServerSpec `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty"`
}

type TemplateSpec struct {
	Active      string `yaml:"active,omitempty" json:"active,omitempty"`
	Source      string `yaml:"source,omitempty" json:"source,omitempty"`
	AnswersFile string `yaml:"answers_file,omitempty" json:"answers_file,omitempty"`
}

type OperatorRuntimeSpec struct {
	Mode         string            `yaml:"mode,omitempty" json:"mode,omitempty"`
	Manifest     string            `yaml:"manifest,omitempty" json:"manifest,omitempty"`
	URL          string            `yaml:"url,omitempty" json:"url,omitempty"`
	StateSources []StateSourceSpec `yaml:"state_sources,omitempty" json:"state_sources,omitempty"`
}

type StateSourceSpec struct {
	Kind           string `yaml:"kind" json:"kind"`
	Path           string `yaml:"path,omitempty" json:"path,omitempty"`
	URL            string `yaml:"url,omitempty" json:"url,omitempty"`
	DatabaseURLEnv string `yaml:"database_url_env,omitempty" json:"database_url_env,omitempty"`
	Optional       bool   `yaml:"optional,omitempty" json:"optional,omitempty"`
}

type DeploymentSpec struct {
	Backend string   `yaml:"backend,omitempty" json:"backend,omitempty"`
	Files   []string `yaml:"files,omitempty" json:"files,omitempty"`
}

type RuntimeSpec struct {
	Adapter string `yaml:"adapter,omitempty" json:"adapter,omitempty"`
}

// SourceSpec defines a named content tree materialized into a workspace,
// service, or job.
type SourceSpec struct {
	Kind   string `yaml:"kind" json:"kind"`
	Ref    string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Repo   string `yaml:"repo,omitempty" json:"repo,omitempty"`
	Tree   string `yaml:"tree,omitempty" json:"tree,omitempty"`
	Target string `yaml:"target" json:"target"`
	URL    string `yaml:"url,omitempty" json:"url,omitempty"`
	Path   string `yaml:"path,omitempty" json:"path,omitempty"`
}

// VolumeSpec defines persistent storage that can be mounted into services,
// jobs, workspaces, or sources.
type VolumeSpec struct {
	Driver     string `yaml:"driver,omitempty" json:"driver,omitempty"`
	Path       string `yaml:"path,omitempty" json:"path,omitempty"`
	Size       string `yaml:"size,omitempty" json:"size,omitempty"`
	Persistent bool   `yaml:"persistent,omitempty" json:"persistent,omitempty"`
}

type SecretSpec struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Generated   bool   `yaml:"generated,omitempty" json:"generated,omitempty"`
	Derived     string `yaml:"derived,omitempty" json:"derived,omitempty"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
	Length      int    `yaml:"length,omitempty" json:"length,omitempty"`
	Charset     string `yaml:"charset,omitempty" json:"charset,omitempty"`
}

type PortLeaseSpec struct {
	Default   int    `yaml:"default,omitempty" json:"default,omitempty"`
	Band      string `yaml:"band,omitempty" json:"band,omitempty"`
	ExportEnv string `yaml:"export_env,omitempty" json:"export_env,omitempty"`
}

// Lifecycle values for services.
const (
	LifecyclePlatform = "platform"
	LifecycleSidecar  = "sidecar"
	LifecycleWorker   = "worker"
	LifecycleSystem   = "system"
	LifecycleAgent    = "agent"
	LifecycleJob      = "job"
)

// ServiceSpec defines a long-running workload. It is backend-neutral, but the
// Docker Compose compiler uses image/build/ports/volumes fields when present.
type ServiceSpec struct {
	Runtime        string            `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Source         string            `yaml:"source,omitempty" json:"source,omitempty"`
	Cwd            string            `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Command        []string          `yaml:"command,omitempty" json:"command,omitempty"`
	Image          string            `yaml:"image,omitempty" json:"image,omitempty"`
	Build          *BuildSpec        `yaml:"build,omitempty" json:"build,omitempty"`
	ComposeService string            `yaml:"compose_service,omitempty" json:"compose_service,omitempty"`
	Lifecycle      string            `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	Domains        []DomainSpec      `yaml:"domains,omitempty" json:"domains,omitempty"`
	Resources      ResourceSpec      `yaml:"resources,omitempty" json:"resources,omitempty"`
	Env            map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Volumes        []ServiceVolume   `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Ports          []ServicePort     `yaml:"ports,omitempty" json:"ports,omitempty"`
	Health         *HealthSpec       `yaml:"health,omitempty" json:"health,omitempty"`
	Replicas       int               `yaml:"replicas,omitempty" json:"replicas,omitempty"`
	DependsOn      []string          `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	After          []string          `yaml:"after,omitempty" json:"after,omitempty"`
}

type BuildSpec struct {
	Context    string `yaml:"context" json:"context"`
	Dockerfile string `yaml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	Target     string `yaml:"target,omitempty" json:"target,omitempty"`
}

type DomainSpec struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port,omitempty" json:"port,omitempty"`
	TLS  bool   `yaml:"tls,omitempty" json:"tls,omitempty"`
}

type ResourceSpec struct {
	CPU    string `yaml:"cpu,omitempty" json:"cpu,omitempty"`
	Memory string `yaml:"memory,omitempty" json:"memory,omitempty"`
}

type ServiceVolume struct {
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	Source   string `yaml:"source,omitempty" json:"source,omitempty"`
	Target   string `yaml:"target,omitempty" json:"target,omitempty"`
	Path     string `yaml:"path,omitempty" json:"path,omitempty"`
	ReadOnly bool   `yaml:"read_only,omitempty" json:"read_only,omitempty"`
}

type ServicePort struct {
	Name   string `yaml:"name,omitempty" json:"name,omitempty"`
	Target string `yaml:"target,omitempty" json:"target,omitempty"`
	Host   string `yaml:"host,omitempty" json:"host,omitempty"`
}

type HealthSpec struct {
	Path     string `yaml:"path" json:"path"`
	Port     int    `yaml:"port,omitempty" json:"port,omitempty"`
	Interval string `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout  string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

type JobSpec struct {
	Kind    string            `yaml:"kind,omitempty" json:"kind,omitempty"`
	Runtime string            `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Source  string            `yaml:"source,omitempty" json:"source,omitempty"`
	Cwd     string            `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Command []string          `yaml:"command,omitempty" json:"command,omitempty"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	After   []string          `yaml:"after,omitempty" json:"after,omitempty"`
}

type WorkflowSpec struct {
	Backend    string             `yaml:"backend,omitempty" json:"backend,omitempty"`
	Activities []WorkflowActivity `yaml:"activities,omitempty" json:"activities,omitempty"`
}

type WorkflowActivity struct {
	Run      string   `yaml:"run" json:"run"`
	Name     string   `yaml:"name,omitempty" json:"name,omitempty"`
	Source   string   `yaml:"source,omitempty" json:"source,omitempty"`
	Service  string   `yaml:"service,omitempty" json:"service,omitempty"`
	Services []string `yaml:"services,omitempty" json:"services,omitempty"`
	Backend  string   `yaml:"backend,omitempty" json:"backend,omitempty"`
	Command  string   `yaml:"command,omitempty" json:"command,omitempty"`
	File     string   `yaml:"file,omitempty" json:"file,omitempty"`
}

type WorkspaceRegistrySpec struct {
	DefaultTemplate string                   `yaml:"default_template,omitempty" json:"default_template,omitempty"`
	Prefix          string                   `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Items           map[string]WorkspaceSpec `yaml:"items,omitempty" json:"items,omitempty"`
}

type WorkspaceSpec struct {
	Template string            `yaml:"template,omitempty" json:"template,omitempty"`
	Sources  map[string]string `yaml:"sources,omitempty" json:"sources,omitempty"`
	Branch   string            `yaml:"branch,omitempty" json:"branch,omitempty"`
	Path     string            `yaml:"path,omitempty" json:"path,omitempty"`
}

type AgentRegistrySpec struct {
	DefaultTemplate string               `yaml:"default_template,omitempty" json:"default_template,omitempty"`
	Prefix          string               `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Items           map[string]AgentSpec `yaml:"items,omitempty" json:"items,omitempty"`
}

type AgentSpec struct {
	Image        string            `yaml:"image,omitempty" json:"image,omitempty"`
	Command      []string          `yaml:"command,omitempty" json:"command,omitempty"`
	Template     string            `yaml:"template,omitempty" json:"template,omitempty"`
	Workspace    string            `yaml:"workspace,omitempty" json:"workspace,omitempty"`
	Service      string            `yaml:"service,omitempty" json:"service,omitempty"`
	Lifecycle    string            `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	Role         string            `yaml:"role,omitempty" json:"role,omitempty"`
	MCPServers   []string          `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty"`
	Files        []FileMount       `yaml:"files,omitempty" json:"files,omitempty"`
	Env          map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Resources    ResourceSpec      `yaml:"resources,omitempty" json:"resources,omitempty"`
	Description  string            `yaml:"description,omitempty" json:"description,omitempty"`
	SystemPrompt string            `yaml:"system_prompt,omitempty" json:"system_prompt,omitempty"`
}

// FileMount declares a config file to generate or bind-mount into an agent container.
// Exactly one of Template or Source should be set.
type FileMount struct {
	Template string `yaml:"template,omitempty" json:"template,omitempty"`
	Source   string `yaml:"source,omitempty" json:"source,omitempty"`
	Mount    string `yaml:"mount" json:"mount"`
	Optional bool   `yaml:"optional,omitempty" json:"optional,omitempty"`
}

type MCPServerSpec struct {
	Transport   string         `yaml:"transport" json:"transport"`
	URL         string         `yaml:"url,omitempty" json:"url,omitempty"`
	Image       string         `yaml:"image,omitempty" json:"image,omitempty"`
	Command     []string       `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string       `yaml:"args,omitempty" json:"args,omitempty"`
	Credentials CredentialSpec `yaml:"credentials,omitempty" json:"credentials,omitempty"`
}

type CredentialSpec struct {
	Source      string   `yaml:"source" json:"source"`
	AccountType string   `yaml:"account_type,omitempty" json:"account_type,omitempty"`
	RunAs       string   `yaml:"run_as,omitempty" json:"run_as,omitempty"`
	Scopes      []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`
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
