package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ComponentConfig is the parsed angee-component.yaml from a component repo.
type ComponentConfig struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`        // service | agent | application | module | credential
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`

	// Dependencies
	Requires []string `yaml:"requires,omitempty"` // components that must exist in the stack
	Extends  string   `yaml:"extends,omitempty"`  // for modules: parent component

	// Parameters resolved at install time ({{ .ParamName }} substitution)
	Parameters []ComponentParam `yaml:"parameters,omitempty"`

	// Stack fragments — merged into angee.yaml
	Repositories map[string]RepositorySpec `yaml:"repositories,omitempty"`
	Services     map[string]ServiceSpec    `yaml:"services,omitempty"`
	MCPServers   map[string]MCPServerSpec  `yaml:"mcp_servers,omitempty"`
	Agents       map[string]AgentSpec      `yaml:"agents,omitempty"`
	Skills       map[string]SkillSpec      `yaml:"skills,omitempty"`
	Secrets      []SecretRef               `yaml:"secrets,omitempty"`

	// Credential definition (type: credential only)
	Credential *CredentialDef `yaml:"credential,omitempty"`

	// Module-specific (type: module only)
	Django *DjangoModuleDef `yaml:"django,omitempty"`

	// File manifest
	Files []ComponentFile `yaml:"files,omitempty"`

	// Deploy-time templates copied to ANGEE_ROOT
	Templates []string `yaml:"templates,omitempty"`

	// Lifecycle hooks
	Hooks *ComponentHooks `yaml:"hooks,omitempty"`
}

// ComponentParam describes a parameter that can be set at install time.
type ComponentParam struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Default     string `yaml:"default,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
}

// ComponentFile declares a file that the component contributes.
type ComponentFile struct {
	Source     string `yaml:"source"`               // path within component repo
	Target     string `yaml:"target"`               // workspace | root | config | service-config
	Agent      string `yaml:"agent,omitempty"`       // target agent (for workspace/config)
	Service    string `yaml:"service,omitempty"`     // target service (for service-config)
	Mount      string `yaml:"mount,omitempty"`       // container path (for config/service-config)
	Phase      string `yaml:"phase,omitempty"`       // install | deploy
	Credential string `yaml:"credential,omitempty"`  // credential name (for config files with cred data)
	Executable bool   `yaml:"executable,omitempty"`  // set +x after copy
}

// CredentialDef defines a credential that a component provides.
type CredentialDef struct {
	Name     string             `yaml:"name"`
	Type     string             `yaml:"type"` // oauth_client | session_token | api_key
	Provider *CredentialProvider `yaml:"provider,omitempty"`
	OAuth    *OAuthEndpoints    `yaml:"oauth,omitempty"`
	VaultPaths map[string]string `yaml:"vault_paths,omitempty"`
	Outputs  []CredentialOutput `yaml:"outputs,omitempty"`
}

// CredentialProvider defines the external auth provider.
type CredentialProvider struct {
	Name       string   `yaml:"name"`
	AuthURL    string   `yaml:"auth_url,omitempty"`
	TokenURL   string   `yaml:"token_url,omitempty"`
	Scopes     []string `yaml:"scopes,omitempty"`
	AuthMethod string   `yaml:"auth_method,omitempty"` // for non-OAuth (e.g. browser_session)
}

// OAuthEndpoints defines the operator endpoints for OAuth flows.
type OAuthEndpoints struct {
	CallbackPath string `yaml:"callback_path,omitempty"`
	ConsentPath  string `yaml:"consent_path,omitempty"`
	TokenPath    string `yaml:"token_path,omitempty"`
}

// CredentialOutput defines how a credential reaches agent containers.
type CredentialOutput struct {
	Type      string `yaml:"type"`                  // env | file
	Key       string `yaml:"key,omitempty"`          // env var name (type: env)
	ValuePath string `yaml:"value_path,omitempty"`   // field within vault secret (type: env)
	Template  string `yaml:"template,omitempty"`     // template file path (type: file)
	Mount     string `yaml:"mount,omitempty"`         // container path (type: file)
	Merge     bool   `yaml:"merge,omitempty"`         // merge into existing file
}

// DjangoModuleDef holds metadata for Django module components.
type DjangoModuleDef struct {
	App        string `yaml:"app"`
	Migrations bool   `yaml:"migrations,omitempty"`
	URLs       string `yaml:"urls,omitempty"`
}

// ComponentHooks defines lifecycle hooks for a component.
type ComponentHooks struct {
	PostInstall string `yaml:"post_install,omitempty"`
	PreDeploy   string `yaml:"pre_deploy,omitempty"`
	PostDeploy  string `yaml:"post_deploy,omitempty"`
	PreRemove   string `yaml:"pre_remove,omitempty"`
}

// InstalledComponent records what a component added to the stack.
type InstalledComponent struct {
	Name        string            `yaml:"name"`
	Version     string            `yaml:"version"`
	Type        string            `yaml:"type"`
	Source      string            `yaml:"source"`
	InstalledAt string            `yaml:"installed_at"`
	Parameters  map[string]string `yaml:"parameters,omitempty"`
	Added       InstalledResources `yaml:"added"`
}

// InstalledResources tracks which angee.yaml entries a component added.
type InstalledResources struct {
	Repositories []string `yaml:"repositories,omitempty"`
	Services     []string `yaml:"services,omitempty"`
	MCPServers   []string `yaml:"mcp_servers,omitempty"`
	Agents       []string `yaml:"agents,omitempty"`
	Skills       []string `yaml:"skills,omitempty"`
	Secrets      []string `yaml:"secrets,omitempty"`
	Files        []string `yaml:"files,omitempty"`
}

// LoadComponent reads and parses an angee-component.yaml file.
func LoadComponent(path string) (*ComponentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading component config: %w", err)
	}
	var comp ComponentConfig
	if err := yaml.Unmarshal(data, &comp); err != nil {
		return nil, fmt.Errorf("parsing component config: %w", err)
	}
	return &comp, nil
}
