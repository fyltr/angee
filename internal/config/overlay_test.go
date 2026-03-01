package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeConfigs_ScalarOverride(t *testing.T) {
	base := &AngeeConfig{
		Name:        "my-project",
		Environment: "dev",
	}
	overlay := &AngeeConfig{
		Environment: "prod",
	}
	result := MergeConfigs(base, overlay)
	if result.Environment != "prod" {
		t.Errorf("expected env 'prod', got %q", result.Environment)
	}
	if result.Name != "my-project" {
		t.Errorf("expected name preserved, got %q", result.Name)
	}
}

func TestMergeConfigs_ServiceEnvMerge(t *testing.T) {
	base := &AngeeConfig{
		Services: map[string]ServiceSpec{
			"django": {
				Image: "django:latest",
				Env: map[string]string{
					"DEBUG":   "true",
					"DB_HOST": "localhost",
				},
				Replicas: 1,
			},
		},
	}
	overlay := &AngeeConfig{
		Services: map[string]ServiceSpec{
			"django": {
				Env: map[string]string{
					"DEBUG": "false",
				},
				Replicas: 3,
			},
		},
	}
	result := MergeConfigs(base, overlay)
	svc := result.Services["django"]

	if svc.Image != "django:latest" {
		t.Errorf("image should be preserved, got %q", svc.Image)
	}
	if svc.Env["DEBUG"] != "false" {
		t.Errorf("DEBUG should be overridden to 'false', got %q", svc.Env["DEBUG"])
	}
	if svc.Env["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST should be preserved, got %q", svc.Env["DB_HOST"])
	}
	if svc.Replicas != 3 {
		t.Errorf("replicas should be 3, got %d", svc.Replicas)
	}
}

func TestMergeConfigs_AddNewService(t *testing.T) {
	base := &AngeeConfig{
		Services: map[string]ServiceSpec{
			"postgres": {Image: "postgres:16"},
		},
	}
	overlay := &AngeeConfig{
		Services: map[string]ServiceSpec{
			"redis": {Image: "redis:7"},
		},
	}
	result := MergeConfigs(base, overlay)
	if _, ok := result.Services["postgres"]; !ok {
		t.Error("postgres should still exist")
	}
	if _, ok := result.Services["redis"]; !ok {
		t.Error("redis should be added")
	}
}

func TestMergeConfigs_SecretDedup(t *testing.T) {
	base := &AngeeConfig{
		Secrets: []SecretRef{
			{Name: "db-password"},
			{Name: "api-key"},
		},
	}
	overlay := &AngeeConfig{
		Secrets: []SecretRef{
			{Name: "api-key"},       // duplicate
			{Name: "django-secret"}, // new
		},
	}
	result := MergeConfigs(base, overlay)
	if len(result.Secrets) != 3 {
		t.Errorf("expected 3 secrets, got %d", len(result.Secrets))
	}
}

func TestMergeConfigs_AgentOverride(t *testing.T) {
	base := &AngeeConfig{
		Agents: map[string]AgentSpec{
			"admin": {
				Image:     "opencode:latest",
				Lifecycle: "system",
				Role:      "operator",
			},
		},
	}
	overlay := &AngeeConfig{
		Agents: map[string]AgentSpec{
			"admin": {
				Lifecycle: "on-demand",
			},
		},
	}
	result := MergeConfigs(base, overlay)
	agent := result.Agents["admin"]
	if agent.Image != "opencode:latest" {
		t.Errorf("image should be preserved, got %q", agent.Image)
	}
	if agent.Lifecycle != "on-demand" {
		t.Errorf("lifecycle should be overridden, got %q", agent.Lifecycle)
	}
	if agent.Role != "operator" {
		t.Errorf("role should be preserved, got %q", agent.Role)
	}
}

func TestLoadWithOverlay(t *testing.T) {
	dir := t.TempDir()

	// Write base config
	base := `name: test-project
environment: dev
services:
  postgres:
    image: postgres:16
    lifecycle: sidecar
`
	os.WriteFile(filepath.Join(dir, "angee.yaml"), []byte(base), 0644)

	// Write prod overlay
	os.MkdirAll(filepath.Join(dir, "environments"), 0755)
	overlay := `services:
  postgres:
    replicas: 3
`
	os.WriteFile(filepath.Join(dir, "environments", "prod.yaml"), []byte(overlay), 0644)

	// Load with prod overlay
	cfg, err := LoadWithOverlay(dir, "prod")
	if err != nil {
		t.Fatalf("LoadWithOverlay: %v", err)
	}

	if cfg.Environment != "prod" {
		t.Errorf("expected env 'prod', got %q", cfg.Environment)
	}
	if cfg.Services["postgres"].Replicas != 3 {
		t.Errorf("expected 3 replicas, got %d", cfg.Services["postgres"].Replicas)
	}
	if cfg.Services["postgres"].Image != "postgres:16" {
		t.Errorf("image should be preserved, got %q", cfg.Services["postgres"].Image)
	}
}

func TestLoadWithOverlay_NoOverlayFile(t *testing.T) {
	dir := t.TempDir()
	base := `name: test-project
`
	os.WriteFile(filepath.Join(dir, "angee.yaml"), []byte(base), 0644)

	cfg, err := LoadWithOverlay(dir, "staging")
	if err != nil {
		t.Fatalf("LoadWithOverlay: %v", err)
	}
	if cfg.Name != "test-project" {
		t.Errorf("expected 'test-project', got %q", cfg.Name)
	}
}
