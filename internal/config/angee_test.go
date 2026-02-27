package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "angee.yaml")
	content := `
name: test-project
version: "1.0"
services:
  web:
    image: nginx
    lifecycle: platform
    domains:
      - host: example.com
        port: 80
agents:
  admin:
    image: ghcr.io/fyltr/angee-agent:latest
    lifecycle: system
    role: operator
mcp_servers:
  filesystem:
    transport: stdio
    command: ["npx", "mcp-fs"]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Name != "test-project" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-project")
	}
	if cfg.Version != "1.0" {
		t.Errorf("Version = %q, want %q", cfg.Version, "1.0")
	}
	if len(cfg.Services) != 1 {
		t.Errorf("Services count = %d, want 1", len(cfg.Services))
	}
	if len(cfg.Agents) != 1 {
		t.Errorf("Agents count = %d, want 1", len(cfg.Agents))
	}
	if len(cfg.MCPServers) != 1 {
		t.Errorf("MCPServers count = %d, want 1", len(cfg.MCPServers))
	}

	web := cfg.Services["web"]
	if web.Image != "nginx" {
		t.Errorf("web.Image = %q, want %q", web.Image, "nginx")
	}
	if web.Lifecycle != LifecyclePlatform {
		t.Errorf("web.Lifecycle = %q, want %q", web.Lifecycle, LifecyclePlatform)
	}

	admin := cfg.Agents["admin"]
	if admin.Role != "operator" {
		t.Errorf("admin.Role = %q, want %q", admin.Role, "operator")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/angee.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "angee.yaml")
	if err := os.WriteFile(path, []byte(":::bad yaml{{{\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestWriteAndRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "angee.yaml")

	original := &AngeeConfig{
		Name:    "roundtrip",
		Version: "2.0",
		Services: map[string]ServiceSpec{
			"api": {Image: "my-api:latest", Lifecycle: LifecyclePlatform},
		},
		Agents: map[string]AgentSpec{
			"bot": {Image: "bot:v1", Role: "user"},
		},
	}

	if err := Write(original, path); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.Name != original.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, original.Name)
	}
	if loaded.Version != original.Version {
		t.Errorf("Version = %q, want %q", loaded.Version, original.Version)
	}
	if len(loaded.Services) != 1 {
		t.Errorf("Services count = %d, want 1", len(loaded.Services))
	}
	if loaded.Services["api"].Image != "my-api:latest" {
		t.Errorf("api.Image = %q, want %q", loaded.Services["api"].Image, "my-api:latest")
	}
}

func TestServicesWithAllLifecycles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "angee.yaml")

	lifecycles := []string{
		LifecyclePlatform, LifecycleSidecar, LifecycleWorker,
		LifecycleSystem, LifecycleAgent, LifecycleJob,
	}

	cfg := &AngeeConfig{
		Name:     "lifecycle-test",
		Services: make(map[string]ServiceSpec),
	}
	for _, lc := range lifecycles {
		cfg.Services["svc-"+lc] = ServiceSpec{
			Image:     "test:latest",
			Lifecycle: lc,
		}
	}

	if err := Write(cfg, path); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	for _, lc := range lifecycles {
		svc, ok := loaded.Services["svc-"+lc]
		if !ok {
			t.Errorf("service svc-%s not found", lc)
			continue
		}
		if svc.Lifecycle != lc {
			t.Errorf("svc-%s.Lifecycle = %q, want %q", lc, svc.Lifecycle, lc)
		}
	}
}
