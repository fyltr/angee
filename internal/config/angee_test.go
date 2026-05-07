package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTargetYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "angee.yaml")
	content := `
version: "1"
kind: stack
name: test-project
template:
  active: stacks/dev
  source: examples/templates/stacks/dev
operator:
  mode: adhoc
  state_sources:
    - kind: file
      path: .angee/state
sources:
  app:
    kind: local
    ref: current
    tree: .
    target: .
secrets:
  app-secret:
    generated: true
    length: 32
port_leases:
  web:
    default: 8100
    band: app
services:
  web:
    runtime: docker
    source: app
    image: nginx
    lifecycle: platform
    command: ["nginx", "-g", "daemon off;"]
    ports:
      - name: web
        target: "80"
workspaces:
  default_template: workspaces/feature-dev
  prefix: workspaces
agents:
  default_template: agents/angee-developer
  prefix: agents
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
		t.Errorf("Name = %q, want test-project", cfg.Name)
	}
	if cfg.Kind != "stack" {
		t.Errorf("Kind = %q, want stack", cfg.Kind)
	}
	if cfg.Template.Active != "stacks/dev" {
		t.Errorf("Template.Active = %q", cfg.Template.Active)
	}
	if cfg.Sources["app"].Kind != "local" {
		t.Errorf("source app kind = %q", cfg.Sources["app"].Kind)
	}
	if cfg.PortLeases["web"].Default != 8100 {
		t.Errorf("web port default = %d", cfg.PortLeases["web"].Default)
	}
	if got := cfg.Services["web"].Command; len(got) != 3 || got[0] != "nginx" {
		t.Errorf("web command = %v", got)
	}
	if cfg.Workspaces.DefaultTemplate != "workspaces/feature-dev" {
		t.Errorf("workspace template = %q", cfg.Workspaces.DefaultTemplate)
	}
	if cfg.Agents.DefaultTemplate != "agents/angee-developer" {
		t.Errorf("agent template = %q", cfg.Agents.DefaultTemplate)
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
		Version: "1",
		Kind:    "stack",
		Name:    "roundtrip",
		Sources: map[string]SourceSpec{
			"app": {Kind: "local", Ref: "current", Target: "."},
		},
		Services: map[string]ServiceSpec{
			"api": {Runtime: "docker", Source: "app", Image: "my-api:latest"},
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
	if loaded.Services["api"].Image != "my-api:latest" {
		t.Errorf("api.Image = %q", loaded.Services["api"].Image)
	}
}
