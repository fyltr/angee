package config

import (
	"strings"
	"testing"
)

func TestValidateEmptyName(t *testing.T) {
	cfg := &AngeeConfig{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "name is required")
	}
}

func TestValidateValidConfig(t *testing.T) {
	cfg := &AngeeConfig{
		Name: "valid-project",
		Services: map[string]ServiceSpec{
			"web": {Image: "nginx", Lifecycle: LifecyclePlatform},
			"db":  {Image: "postgres", Lifecycle: LifecycleSidecar},
		},
		MCPServers: map[string]MCPServerSpec{
			"fs": {Transport: "stdio"},
		},
		Agents: map[string]AgentSpec{
			"admin": {
				Image:      "agent:latest",
				Lifecycle:  LifecycleSystem,
				MCPServers: []string{"fs"},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidateInvalidLifecycle(t *testing.T) {
	cfg := &AngeeConfig{
		Name: "test",
		Services: map[string]ServiceSpec{
			"web": {Image: "nginx", Lifecycle: "bogus"},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid lifecycle")
	}
	if !strings.Contains(err.Error(), "invalid lifecycle") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "invalid lifecycle")
	}
}

func TestValidateInvalidAgentLifecycle(t *testing.T) {
	cfg := &AngeeConfig{
		Name: "test",
		Agents: map[string]AgentSpec{
			"bot": {Image: "bot:latest", Lifecycle: "not-real"},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid agent lifecycle")
	}
	if !strings.Contains(err.Error(), "invalid lifecycle") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "invalid lifecycle")
	}
}

func TestValidateMCPServerReference(t *testing.T) {
	cfg := &AngeeConfig{
		Name: "test",
		Agents: map[string]AgentSpec{
			"admin": {
				Image:      "agent:latest",
				MCPServers: []string{"nonexistent"},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for undefined MCP server reference")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "nonexistent")
	}
}

func TestValidateDomainConflict(t *testing.T) {
	cfg := &AngeeConfig{
		Name: "test",
		Services: map[string]ServiceSpec{
			"web1": {
				Image:     "nginx",
				Lifecycle: LifecyclePlatform,
				Domains:   []DomainSpec{{Host: "example.com", Port: 80}},
			},
			"web2": {
				Image:     "nginx",
				Lifecycle: LifecyclePlatform,
				Domains:   []DomainSpec{{Host: "example.com", Port: 80}},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for domain conflict")
	}
	if !strings.Contains(err.Error(), "domain conflict") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "domain conflict")
	}
}

func TestValidatePortConflict(t *testing.T) {
	cfg := &AngeeConfig{
		Name: "test",
		Services: map[string]ServiceSpec{
			"web1": {
				Image:     "nginx",
				Lifecycle: LifecyclePlatform,
				Ports:     []string{"8080:80"},
			},
			"web2": {
				Image:     "nginx",
				Lifecycle: LifecyclePlatform,
				Ports:     []string{"8080:3000"},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for port conflict")
	}
	if !strings.Contains(err.Error(), "port conflict") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "port conflict")
	}
}

func TestValidateCleanConfig(t *testing.T) {
	cfg := &AngeeConfig{
		Name: "clean",
		Services: map[string]ServiceSpec{
			"web": {
				Image:     "nginx",
				Lifecycle: LifecyclePlatform,
				Ports:     []string{"8080:80"},
				Domains:   []DomainSpec{{Host: "web.example.com", Port: 80}},
			},
			"api": {
				Image:     "api:latest",
				Lifecycle: LifecyclePlatform,
				Ports:     []string{"9090:80"},
				Domains:   []DomainSpec{{Host: "api.example.com", Port: 80}},
			},
		},
		MCPServers: map[string]MCPServerSpec{
			"fs": {Transport: "stdio"},
		},
		Agents: map[string]AgentSpec{
			"admin": {
				Image:      "agent:latest",
				Lifecycle:  LifecycleSystem,
				MCPServers: []string{"fs"},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	cfg := &AngeeConfig{
		// Missing name
		Services: map[string]ServiceSpec{
			"web": {Lifecycle: "invalid-lc"},
		},
		Agents: map[string]AgentSpec{
			"bot": {MCPServers: []string{"missing-mcp"}},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	// Should report all errors, not just the first
	errStr := err.Error()
	if !strings.Contains(errStr, "name is required") {
		t.Errorf("error should mention 'name is required', got: %s", errStr)
	}
	if !strings.Contains(errStr, "invalid lifecycle") {
		t.Errorf("error should mention 'invalid lifecycle', got: %s", errStr)
	}
	if !strings.Contains(errStr, "missing-mcp") {
		t.Errorf("error should mention 'missing-mcp', got: %s", errStr)
	}
}
