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
	Runtime        string           `yaml:"runtime"`     // docker-compose | kubernetes
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
func DefaultOperatorConfig(angeeRoot string) *OperatorConfig {
	return &OperatorConfig{
		Runtime:     "docker-compose",
		Port:        9000,
		AngeeRoot:   angeeRoot,
		DjangoURL:   "http://localhost:8000",
		BindAddress: "0.0.0.0",
		CORSOrigins: []string{"http://localhost:*"},
		Docker: DockerConfig{
			Socket:  "/var/run/docker.sock",
			Network: "angee-net",
		},
	}
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
