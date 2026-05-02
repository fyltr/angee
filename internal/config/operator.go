package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// OperatorConfig is the local runtime configuration (operator.yaml).
// This file is NOT committed to git — it tells the operator which
// runtime backend to use and how to find the Django control plane.
type OperatorConfig struct {
	Runtime        string           `yaml:"runtime"` // docker-compose | kubernetes
	Port           int              `yaml:"port,omitempty"`
	AngeeRoot      string           `yaml:"angee_root,omitempty"`
	DjangoURL      string           `yaml:"django_url,omitempty"`
	APIKey         string           `yaml:"api_key,omitempty"`
	BindAddress    string           `yaml:"bind_address,omitempty"`
	CORSOrigins    []string         `yaml:"cors_origins,omitempty"`
	TemplateSource string           `yaml:"template_source,omitempty"` // URL/path used by angee init
	Docker         DockerConfig     `yaml:"docker,omitempty"`
	Kubernetes     KubernetesConfig `yaml:"kubernetes,omitempty"`
}

// DockerConfig holds Docker Compose backend settings.
type DockerConfig struct {
	Socket  string `yaml:"socket,omitempty"`
	Network string `yaml:"network,omitempty"`
}

// KubernetesConfig holds Kubernetes backend settings.
type KubernetesConfig struct {
	Context      string `yaml:"context,omitempty"`
	Namespace    string `yaml:"namespace,omitempty"`
	IngressClass string `yaml:"ingress_class,omitempty"`
	StorageClass string `yaml:"storage_class,omitempty"`
	ChartRef     string `yaml:"chart,omitempty"`
}

// DefaultOperatorConfig returns sensible defaults.
//
// BindAddress defaults to 127.0.0.1 (loopback only). Operators must opt in to
// non-loopback binds AND set APIKey — see Server.Start which refuses to listen
// on a non-loopback address with no API key configured.
func DefaultOperatorConfig(angeeRoot string) *OperatorConfig {
	return &OperatorConfig{
		Runtime:     "docker-compose",
		Port:        9000,
		AngeeRoot:   angeeRoot,
		DjangoURL:   "http://localhost:8000",
		BindAddress: "127.0.0.1",
		CORSOrigins: []string{"http://localhost:*"},
		Docker: DockerConfig{
			Socket:  "/var/run/docker.sock",
			Network: "angee-net",
		},
	}
}

// IsLoopbackBind reports whether the configured BindAddress is a loopback
// address (127.0.0.0/8, ::1, or "localhost"). Used at startup to decide
// whether an API key is mandatory.
func (c *OperatorConfig) IsLoopbackBind() bool {
	addr := c.BindAddress
	if addr == "" || addr == "localhost" {
		return true
	}
	// Strip an optional port if a host:port form was passed.
	if i := lastIndex(addr, ':'); i >= 0 && !hasIPv6Brackets(addr) {
		addr = addr[:i]
	}
	if addr == "::1" {
		return true
	}
	if len(addr) >= 4 && addr[:4] == "127." {
		return true
	}
	return false
}

func lastIndex(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func hasIPv6Brackets(s string) bool {
	return len(s) > 0 && s[0] == '['
}

// LoadOperatorConfig reads operator.yaml from the given path.
func LoadOperatorConfig(angeeRoot string) (*OperatorConfig, error) {
	path := filepath.Join(angeeRoot, "operator.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultOperatorConfig(angeeRoot), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading operator.yaml: %w", err)
	}
	cfg := DefaultOperatorConfig(angeeRoot)
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing operator.yaml: %w", err)
	}
	if cfg.AngeeRoot == "" {
		cfg.AngeeRoot = angeeRoot
	}
	return cfg, nil
}

// WriteOperatorConfig writes operator.yaml to the angeeRoot directory.
func WriteOperatorConfig(cfg *OperatorConfig, angeeRoot string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serializing operator.yaml: %w", err)
	}
	return os.WriteFile(filepath.Join(angeeRoot, "operator.yaml"), data, 0600)
}
