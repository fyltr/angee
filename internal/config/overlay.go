package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// LoadWithOverlay loads angee.yaml and merges an environment overlay if present.
// If env is empty, it uses cfg.Environment, falling back to "dev".
func LoadWithOverlay(rootPath string, env string) (*AngeeConfig, error) {
	base, err := Load(filepath.Join(rootPath, "angee.yaml"))
	if err != nil {
		return nil, err
	}

	if env == "" {
		env = base.Environment
	}
	if env == "" {
		env = "dev"
	}
	base.Environment = env

	overlayPath := filepath.Join(rootPath, "environments", env+".yaml")
	if _, err := os.Stat(overlayPath); os.IsNotExist(err) {
		return base, nil
	}

	overlay, err := Load(overlayPath)
	if err != nil {
		return nil, fmt.Errorf("loading %s overlay: %w", env, err)
	}

	return MergeConfigs(base, overlay), nil
}

// MergeConfigs deep-merges overlay into base. Overlay values win for scalars;
// maps are merged key-by-key; secrets are appended (deduped by name).
func MergeConfigs(base, overlay *AngeeConfig) *AngeeConfig {
	if overlay.Name != "" {
		base.Name = overlay.Name
	}
	if overlay.Version != "" {
		base.Version = overlay.Version
	}
	if overlay.Environment != "" {
		base.Environment = overlay.Environment
	}
	if overlay.SecretsBackend != nil {
		base.SecretsBackend = overlay.SecretsBackend
	}

	// Merge maps: overlay keys override base keys
	base.Repositories = mergeMaps(base.Repositories, overlay.Repositories)
	base.Services = mergeServiceMaps(base.Services, overlay.Services)
	base.MCPServers = mergeMCPMaps(base.MCPServers, overlay.MCPServers)
	base.Agents = mergeAgentMaps(base.Agents, overlay.Agents)
	base.Skills = mergeSkillMaps(base.Skills, overlay.Skills)

	// Append secrets, dedup by name
	base.Secrets = mergeSecrets(base.Secrets, overlay.Secrets)

	return base
}

// mergeMaps merges two maps, overlay wins on key conflict.
func mergeMaps[V any](base, overlay map[string]V) map[string]V {
	if len(overlay) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]V)
	}
	for k, v := range overlay {
		base[k] = v
	}
	return base
}

func mergeServiceMaps(base, overlay map[string]ServiceSpec) map[string]ServiceSpec {
	if len(overlay) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]ServiceSpec)
	}
	for k, v := range overlay {
		if existing, ok := base[k]; ok {
			base[k] = mergeServiceSpec(existing, v)
		} else {
			base[k] = v
		}
	}
	return base
}

// mergeServiceSpec merges individual service fields. Overlay non-zero values win.
func mergeServiceSpec(base, overlay ServiceSpec) ServiceSpec {
	if overlay.Image != "" {
		base.Image = overlay.Image
	}
	if overlay.Build != nil {
		base.Build = overlay.Build
	}
	if overlay.Command != "" {
		base.Command = overlay.Command
	}
	if overlay.Lifecycle != "" {
		base.Lifecycle = overlay.Lifecycle
	}
	if len(overlay.Domains) > 0 {
		base.Domains = overlay.Domains
	}
	if overlay.Resources.CPU != "" {
		base.Resources.CPU = overlay.Resources.CPU
	}
	if overlay.Resources.Memory != "" {
		base.Resources.Memory = overlay.Resources.Memory
	}
	if len(overlay.Env) > 0 {
		if base.Env == nil {
			base.Env = make(map[string]string)
		}
		for k, v := range overlay.Env {
			base.Env[k] = v
		}
	}
	if len(overlay.Volumes) > 0 {
		base.Volumes = overlay.Volumes
	}
	if len(overlay.Ports) > 0 {
		base.Ports = overlay.Ports
	}
	if overlay.Replicas > 0 {
		base.Replicas = overlay.Replicas
	}
	if len(overlay.DependsOn) > 0 {
		base.DependsOn = overlay.DependsOn
	}
	if overlay.Health != nil {
		base.Health = overlay.Health
	}
	return base
}

func mergeMCPMaps(base, overlay map[string]MCPServerSpec) map[string]MCPServerSpec {
	return mergeMaps(base, overlay)
}

func mergeAgentMaps(base, overlay map[string]AgentSpec) map[string]AgentSpec {
	if len(overlay) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]AgentSpec)
	}
	for k, v := range overlay {
		if existing, ok := base[k]; ok {
			base[k] = mergeAgentSpec(existing, v)
		} else {
			base[k] = v
		}
	}
	return base
}

func mergeAgentSpec(base, overlay AgentSpec) AgentSpec {
	if overlay.Image != "" {
		base.Image = overlay.Image
	}
	if overlay.Command != "" {
		base.Command = overlay.Command
	}
	if overlay.Lifecycle != "" {
		base.Lifecycle = overlay.Lifecycle
	}
	if overlay.Role != "" {
		base.Role = overlay.Role
	}
	if len(overlay.MCPServers) > 0 {
		base.MCPServers = overlay.MCPServers
	}
	if len(overlay.Skills) > 0 {
		base.Skills = overlay.Skills
	}
	if overlay.Resources.CPU != "" {
		base.Resources.CPU = overlay.Resources.CPU
	}
	if overlay.Resources.Memory != "" {
		base.Resources.Memory = overlay.Resources.Memory
	}
	if len(overlay.Env) > 0 {
		if base.Env == nil {
			base.Env = make(map[string]string)
		}
		for k, v := range overlay.Env {
			base.Env[k] = v
		}
	}
	if overlay.SystemPrompt != "" {
		base.SystemPrompt = overlay.SystemPrompt
	}
	if len(overlay.CredentialBindings) > 0 {
		base.CredentialBindings = overlay.CredentialBindings
	}
	if len(overlay.Permissions) > 0 {
		base.Permissions = overlay.Permissions
	}
	return base
}

func mergeSkillMaps(base, overlay map[string]SkillSpec) map[string]SkillSpec {
	return mergeMaps(base, overlay)
}

// mergeSecrets appends overlay secrets, skipping any that already exist by name.
func mergeSecrets(base, overlay []SecretRef) []SecretRef {
	if len(overlay) == 0 {
		return base
	}
	existing := make(map[string]bool)
	for _, s := range base {
		existing[s.Name] = true
	}
	for _, s := range overlay {
		if !existing[s.Name] {
			base = append(base, s)
			existing[s.Name] = true
		}
	}
	return base
}
