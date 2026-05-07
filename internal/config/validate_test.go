package config

import (
	"strings"
	"testing"
)

func validConfig() *AngeeConfig {
	return &AngeeConfig{
		Kind: "stack",
		Name: "valid-project",
		Sources: map[string]SourceSpec{
			"app": {Kind: "local", Ref: "current", Target: "."},
		},
		PortLeases: map[string]PortLeaseSpec{
			"web": {Default: 8100},
		},
		Services: map[string]ServiceSpec{
			"web": {
				Runtime:   "docker",
				Source:    "app",
				Image:     "nginx",
				Lifecycle: LifecyclePlatform,
				Ports:     []ServicePort{{Name: "web", Target: "80"}},
			},
		},
		MCPServers: map[string]MCPServerSpec{
			"fs": {Transport: "stdio", Command: []string{"node", "server.js"}},
		},
		Agents: &AgentRegistrySpec{
			Items: map[string]AgentSpec{
				"bot": {Service: "web", MCPServers: []string{"fs"}},
			},
		},
	}
}

func TestValidateValidConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidateEmptyName(t *testing.T) {
	err := (&AngeeConfig{}).Validate()
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name error, got %v", err)
	}
}

func TestValidateInvalidSourceKind(t *testing.T) {
	cfg := validConfig()
	cfg.Sources["app"] = SourceSpec{Kind: "bogus", Target: "."}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid kind") {
		t.Fatalf("expected invalid kind error, got %v", err)
	}
}

func TestValidateMissingServiceSource(t *testing.T) {
	cfg := validConfig()
	cfg.Services["web"] = ServiceSpec{Source: "missing", Image: "nginx"}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "source \"missing\"") {
		t.Fatalf("expected missing source error, got %v", err)
	}
}

func TestValidateMissingPortLease(t *testing.T) {
	cfg := validConfig()
	cfg.Services["web"] = ServiceSpec{Source: "app", Image: "nginx", Ports: []ServicePort{{Name: "missing", Target: "80"}}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "port lease \"missing\"") {
		t.Fatalf("expected missing port lease error, got %v", err)
	}
}

func TestValidateInvalidLifecycle(t *testing.T) {
	cfg := validConfig()
	cfg.Services["web"] = ServiceSpec{Source: "app", Image: "nginx", Lifecycle: "bogus"}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid lifecycle") {
		t.Fatalf("expected invalid lifecycle error, got %v", err)
	}
}

func TestValidateMCPServerReference(t *testing.T) {
	cfg := validConfig()
	cfg.Agents.Items["bot"] = AgentSpec{MCPServers: []string{"nonexistent"}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("expected missing MCP error, got %v", err)
	}
}

func TestValidateDomainConflict(t *testing.T) {
	cfg := validConfig()
	cfg.Services["web2"] = ServiceSpec{
		Image:     "nginx",
		Lifecycle: LifecyclePlatform,
		Domains:   []DomainSpec{{Host: "example.com", Port: 80}},
	}
	web := cfg.Services["web"]
	web.Domains = []DomainSpec{{Host: "example.com", Port: 80}}
	cfg.Services["web"] = web
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "domain conflict") {
		t.Fatalf("expected domain conflict, got %v", err)
	}
}

func TestValidatePortLeaseConflict(t *testing.T) {
	cfg := validConfig()
	cfg.PortLeases["other"] = PortLeaseSpec{Default: 8100}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "port_lease conflict") {
		t.Fatalf("expected port lease conflict, got %v", err)
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	cfg := &AngeeConfig{
		Services: map[string]ServiceSpec{"web": {Lifecycle: "invalid-lc", Source: "missing"}},
		Agents:   &AgentRegistrySpec{Items: map[string]AgentSpec{"bot": {MCPServers: []string{"missing-mcp"}}}},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	errStr := err.Error()
	for _, want := range []string{"name is required", "invalid lifecycle", "missing-mcp"} {
		if !strings.Contains(errStr, want) {
			t.Errorf("error should mention %q, got: %s", want, errStr)
		}
	}
}
