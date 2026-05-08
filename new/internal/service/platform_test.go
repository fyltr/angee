package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fyltr/angee/internal/manifest"
)

func TestStackPrepareWritesSecretSafeGeneratedFiles(t *testing.T) {
	root := t.TempDir()
	stack := &manifest.Stack{
		Version: manifest.VersionCurrent,
		Kind:    manifest.KindStack,
		Name:    "notes",
		SecretsBackend: manifest.SecretsBackend{
			Type: "env-file",
			Path: ".env",
		},
		Secrets: map[string]manifest.Secret{
			"postgres-password": {Required: true, Import: "env:POSTGRES_PASSWORD"},
		},
		Ports: map[string]manifest.Port{
			"postgres": {Value: 5432},
		},
		Services: map[string]manifest.Service{
			"postgres": {
				Runtime: manifest.RuntimeContainer,
				Image:   "postgres:16",
				Env: map[string]string{
					"POSTGRES_PASSWORD": "${secret.postgres-password}",
				},
				Ports: []string{"127.0.0.1:${ports.postgres}:5432"},
			},
		},
	}
	if err := manifest.SaveFile(manifest.Path(root), stack); err != nil {
		t.Fatalf("SaveFile() error = %v", err)
	}
	t.Setenv("POSTGRES_PASSWORD", "super-secret")

	platform, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	compiled, err := platform.StackPrepare(context.Background())
	if err != nil {
		t.Fatalf("StackPrepare() error = %v", err)
	}
	text, err := compiled.Text()
	if err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	if strings.Contains(text, "super-secret") {
		t.Fatal("compiled runtime files contain resolved secret")
	}
	if !strings.Contains(text, "${ANGEE_SECRET_POSTGRES_PASSWORD}") {
		t.Fatalf("compiled text missing secret env placeholder:\n%s", text)
	}
	envData, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatalf("ReadFile(.env) error = %v", err)
	}
	if !strings.Contains(string(envData), "ANGEE_SECRET_POSTGRES_PASSWORD") || !strings.Contains(string(envData), "super-secret") {
		t.Fatalf("env file does not contain runtime secret env var: %s", envData)
	}
}
