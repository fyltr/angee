package credentials

import (
	"fmt"
	"path/filepath"

	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/credentials/openbao"
)

// NewBackend creates a credentials backend from the stack configuration.
// When no backend is configured, it returns an EnvBackend (preserving existing behavior).
func NewBackend(cfg *config.AngeeConfig, rootPath string) (Backend, error) {
	if cfg.SecretsBackend == nil || cfg.SecretsBackend.Type == "" || cfg.SecretsBackend.Type == "env" {
		return NewEnvBackend(filepath.Join(rootPath, ".env")), nil
	}

	switch cfg.SecretsBackend.Type {
	case "openbao":
		if cfg.SecretsBackend.OpenBao == nil {
			return nil, fmt.Errorf("secrets_backend.type is 'openbao' but openbao config is missing")
		}
		return openbao.New(cfg.SecretsBackend.OpenBao, cfg.Environment)
	default:
		return nil, fmt.Errorf("unknown secrets backend type: %q", cfg.SecretsBackend.Type)
	}
}
