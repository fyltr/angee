package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fyltr/angee/internal/config"
)

func newTestCompiler(t *testing.T) *Compiler {
	t.Helper()
	return New(t.TempDir(), "test-net")
}

func TestCompileMinimalConfig(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "minimal",
		Services: map[string]config.ServiceSpec{
			"web": {Image: "nginx", Lifecycle: config.LifecyclePlatform},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if cf.Name != "minimal" {
		t.Errorf("Name = %q, want %q", cf.Name, "minimal")
	}
	if _, ok := cf.Services["web"]; !ok {
		t.Fatal("expected service 'web'")
	}
	web := cf.Services["web"]
	if web.Image != "nginx" {
		t.Errorf("web.Image = %q, want %q", web.Image, "nginx")
	}
}

func TestCompilePlatformTraefikLabels(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "traefik-test",
		Services: map[string]config.ServiceSpec{
			"web": {
				Image:     "nginx",
				Lifecycle: config.LifecyclePlatform,
				Domains: []config.DomainSpec{
					{Host: "example.com", Port: 80, TLS: true},
				},
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	web := cf.Services["web"]
	if web.Labels["traefik.enable"] != "true" {
		t.Error("expected traefik.enable=true")
	}
	if !strings.Contains(web.Labels["traefik.http.routers.web.rule"], "Host(`example.com`)") {
		t.Error("expected Host rule for example.com")
	}
	if web.Labels["traefik.http.routers.web.entrypoints"] != "websecure" {
		t.Error("expected websecure entrypoint for TLS")
	}
	if web.Labels["traefik.http.routers.web.tls.certresolver"] != "letsencrypt" {
		t.Error("expected letsencrypt certresolver")
	}
}

func TestCompileSidecar(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "sidecar-test",
		Services: map[string]config.ServiceSpec{
			"db": {Image: "postgres:16", Lifecycle: config.LifecycleSidecar},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	db := cf.Services["db"]
	if db.Restart != "unless-stopped" {
		t.Errorf("Restart = %q, want %q", db.Restart, "unless-stopped")
	}
}

func TestCompileAgent(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "agent-test",
		Agents: map[string]config.AgentSpec{
			"admin": {
				Image:        "agent:latest",
				Lifecycle:    config.LifecycleSystem,
				Role:         "operator",
				SystemPrompt: "You are the admin agent.",
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	svc, ok := cf.Services["agent-admin"]
	if !ok {
		t.Fatal("expected service 'agent-admin'")
	}

	if svc.Image != "agent:latest" {
		t.Errorf("Image = %q, want %q", svc.Image, "agent:latest")
	}
	if svc.Restart != "always" {
		t.Errorf("Restart = %q, want %q for system lifecycle", svc.Restart, "always")
	}
	if svc.Labels["angee.type"] != "agent" {
		t.Error("expected angee.type=agent label")
	}
	if svc.Labels["angee.role"] != "operator" {
		t.Error("expected angee.role=operator label")
	}

	// Check environment vars
	hasName := false
	hasPrompt := false
	for _, env := range svc.Environment {
		if env == "ANGEE_AGENT_NAME=admin" {
			hasName = true
		}
		if strings.HasPrefix(env, "ANGEE_SYSTEM_PROMPT=") {
			hasPrompt = true
		}
	}
	if !hasName {
		t.Error("expected ANGEE_AGENT_NAME=admin in environment")
	}
	if !hasPrompt {
		t.Error("expected ANGEE_SYSTEM_PROMPT in environment")
	}

	// Check workspace volume
	hasWorkspace := false
	for _, v := range svc.Volumes {
		if strings.HasSuffix(v, ":/workspace") {
			hasWorkspace = true
		}
	}
	if !hasWorkspace {
		t.Error("expected workspace volume mount")
	}
}

func TestCompileAgentMCPEnv(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "mcp-test",
		MCPServers: map[string]config.MCPServerSpec{
			"my-server": {
				Transport: "sse",
				URL:       "http://mcp:8080/sse",
			},
		},
		Agents: map[string]config.AgentSpec{
			"bot": {
				Image:      "agent:latest",
				MCPServers: []string{"my-server"},
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	svc := cf.Services["agent-bot"]
	hasMCPURL := false
	hasMCPList := false
	for _, env := range svc.Environment {
		if env == "ANGEE_MCP_MY_SERVER_URL=http://mcp:8080/sse" {
			hasMCPURL = true
		}
		if env == "ANGEE_MCP_SERVERS=my-server" {
			hasMCPList = true
		}
	}
	if !hasMCPURL {
		t.Error("expected MCP URL env var")
	}
	if !hasMCPList {
		t.Error("expected ANGEE_MCP_SERVERS env var")
	}
}

func TestCompileResources(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "resources-test",
		Services: map[string]config.ServiceSpec{
			"web": {
				Image:     "nginx",
				Lifecycle: config.LifecyclePlatform,
				Resources: config.ResourceSpec{CPU: "0.5", Memory: "512M"},
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	web := cf.Services["web"]
	if web.Deploy == nil {
		t.Fatal("expected Deploy to be set")
	}
	if web.Deploy.Resources.Limits.CPUs != "0.5" {
		t.Errorf("CPU limit = %q, want %q", web.Deploy.Resources.Limits.CPUs, "0.5")
	}
	if web.Deploy.Resources.Limits.Memory != "512M" {
		t.Errorf("Memory limit = %q, want %q", web.Deploy.Resources.Limits.Memory, "512M")
	}
}

func TestCompileResourcesNormalizesMemory(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "norm-test",
		Services: map[string]config.ServiceSpec{
			"django": {
				Image:     "django:latest",
				Lifecycle: config.LifecyclePlatform,
				Resources: config.ResourceSpec{CPU: "1.0", Memory: "1Gi"},
			},
			"celery": {
				Image:     "celery:latest",
				Lifecycle: config.LifecycleWorker,
				Resources: config.ResourceSpec{Memory: "256Mi"},
			},
		},
		Agents: map[string]config.AgentSpec{
			"dev": {
				Image:     "agent:latest",
				Resources: config.ResourceSpec{Memory: "4Gi"},
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	// Service: "1Gi" → "1g"
	django := cf.Services["django"]
	if django.Deploy.Resources.Limits.Memory != "1g" {
		t.Errorf("django memory = %q, want %q", django.Deploy.Resources.Limits.Memory, "1g")
	}

	// Service: "256Mi" → "256m"
	celery := cf.Services["celery"]
	if celery.Deploy.Resources.Limits.Memory != "256m" {
		t.Errorf("celery memory = %q, want %q", celery.Deploy.Resources.Limits.Memory, "256m")
	}

	// Agent: "4Gi" → "4g"
	dev := cf.Services["agent-dev"]
	if dev.Deploy.Resources.Limits.Memory != "4g" {
		t.Errorf("agent-dev memory = %q, want %q", dev.Deploy.Resources.Limits.Memory, "4g")
	}
}

func TestCompileAgentWorkspaceVolume(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "vol-test",
		Agents: map[string]config.AgentSpec{
			"dev": {Image: "agent:latest"},
			"admin": {
				Image:     "agent:latest",
				Workspace: config.WorkspaceSpec{Path: "."},
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	// Default workspace: should be ./agents/dev/workspace:/workspace
	dev := cf.Services["agent-dev"]
	found := false
	for _, v := range dev.Volumes {
		if v == "./agents/dev/workspace:/workspace" {
			found = true
		}
	}
	if !found {
		t.Errorf("agent-dev volumes = %v, want ./agents/dev/workspace:/workspace", dev.Volumes)
	}

	// Explicit path ".": becomes "./.:/workspace"
	admin := cf.Services["agent-admin"]
	found = false
	for _, v := range admin.Volumes {
		if v == "./.:/workspace" {
			found = true
		}
	}
	if !found {
		t.Errorf("agent-admin volumes = %v, want [./.:/workspace]", admin.Volumes)
	}

	// env_file should also have ./ prefix
	if len(dev.EnvFile) == 0 || dev.EnvFile[0] != "./agents/dev/.env" {
		t.Errorf("agent-dev env_file = %v, want [./agents/dev/.env]", dev.EnvFile)
	}
}

func TestEnsureBindMountPrefix(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"agents/dev/workspace", "./agents/dev/workspace"},
		{"./already/prefixed", "./already/prefixed"},
		{"../parent/path", "../parent/path"},
		{"/absolute/path", "/absolute/path"},
		{".", "./."},
		{"", ""},
	}
	for _, tt := range tests {
		got := ensureBindMountPrefix(tt.input)
		if got != tt.want {
			t.Errorf("ensureBindMountPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeMemory(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"1Gi", "1g"},
		{"256Mi", "256m"},
		{"512Ki", "512k"},
		{"512m", "512m"},   // already docker-style
		{"1g", "1g"},       // already docker-style
		{"1024", "1024"},   // bare number
		{"", ""},           // empty
	}
	for _, tt := range tests {
		got := normalizeMemory(tt.input)
		if got != tt.want {
			t.Errorf("normalizeMemory(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCompileHealthCheck(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "health-test",
		Services: map[string]config.ServiceSpec{
			"api": {
				Image:     "api:latest",
				Lifecycle: config.LifecyclePlatform,
				Health:    &config.HealthSpec{Path: "/healthz", Port: 8000, Interval: "10s", Timeout: "3s"},
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	api := cf.Services["api"]
	if api.Healthcheck == nil {
		t.Fatal("expected healthcheck to be set")
	}
	if len(api.Healthcheck.Test) < 2 {
		t.Fatal("expected CMD-SHELL test")
	}
	if api.Healthcheck.Test[0] != "CMD-SHELL" {
		t.Errorf("Test[0] = %q, want %q", api.Healthcheck.Test[0], "CMD-SHELL")
	}
	if !strings.Contains(api.Healthcheck.Test[1], "/healthz") {
		t.Error("expected healthcheck test to contain /healthz")
	}
	if api.Healthcheck.Interval != "10s" {
		t.Errorf("Interval = %q, want %q", api.Healthcheck.Interval, "10s")
	}
}

func TestCompileVolumes(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "vol-test",
		Services: map[string]config.ServiceSpec{
			"db": {
				Image:     "postgres",
				Lifecycle: config.LifecycleSidecar,
				Volumes: []config.VolumeSpec{
					{Name: "pgdata", Path: "/var/lib/postgresql/data", Persistent: true},
				},
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if _, ok := cf.Volumes["pgdata"]; !ok {
		t.Error("expected named volume 'pgdata' to be registered")
	}

	db := cf.Services["db"]
	hasVolume := false
	for _, v := range db.Volumes {
		if v == "pgdata:/var/lib/postgresql/data" {
			hasVolume = true
		}
	}
	if !hasVolume {
		t.Error("expected volume mount pgdata:/var/lib/postgresql/data")
	}
}

func TestWriteCompose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yaml")

	cf := &ComposeFile{
		Name: "test",
		Services: map[string]ComposeService{
			"web": {Image: "nginx", Restart: "unless-stopped"},
		},
	}

	if err := Write(cf, path); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "# Generated by angee-operator") {
		t.Error("expected header comment")
	}
	if !strings.Contains(content, "nginx") {
		t.Error("expected nginx in output")
	}
}

func TestRestartPolicy(t *testing.T) {
	tests := []struct {
		lifecycle string
		want      string
	}{
		{config.LifecycleSystem, "always"},
		{config.LifecyclePlatform, "unless-stopped"},
		{config.LifecycleSidecar, "unless-stopped"},
		{config.LifecycleWorker, "unless-stopped"},
		{config.LifecycleAgent, "unless-stopped"},
		{"", "unless-stopped"},
	}
	for _, tt := range tests {
		got := restartPolicy(tt.lifecycle)
		if got != tt.want {
			t.Errorf("restartPolicy(%q) = %q, want %q", tt.lifecycle, got, tt.want)
		}
	}
}

func TestAgentServicePrefixConstant(t *testing.T) {
	if AgentServicePrefix != "agent-" {
		t.Errorf("AgentServicePrefix = %q, want %q", AgentServicePrefix, "agent-")
	}
}

func TestCompileAgentAPIKeyInjection(t *testing.T) {
	c := newTestCompiler(t)
	c.APIKey = "test-api-key-123"

	cfg := &config.AngeeConfig{
		Name: "apikey-test",
		Agents: map[string]config.AgentSpec{
			"admin": {
				Image:     "agent:latest",
				Lifecycle: config.LifecycleSystem,
				Role:      "operator",
			},
			"dev": {
				Image:     "agent:latest",
				Lifecycle: config.LifecycleSystem,
				Role:      "user",
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	// operator-role agent should have API key
	admin := cf.Services["agent-admin"]
	hasKey := false
	for _, env := range admin.Environment {
		if env == "ANGEE_OPERATOR_API_KEY=test-api-key-123" {
			hasKey = true
		}
	}
	if !hasKey {
		t.Error("expected ANGEE_OPERATOR_API_KEY in operator-role agent environment")
	}

	// user-role agent should NOT have API key
	dev := cf.Services["agent-dev"]
	for _, env := range dev.Environment {
		if strings.HasPrefix(env, "ANGEE_OPERATOR_API_KEY=") {
			t.Error("user-role agent should not have ANGEE_OPERATOR_API_KEY")
		}
	}
}

func TestCompileAgentAPIKeyNotInjectedWhenEmpty(t *testing.T) {
	c := newTestCompiler(t)
	// APIKey is empty by default

	cfg := &config.AngeeConfig{
		Name: "no-key-test",
		Agents: map[string]config.AgentSpec{
			"admin": {
				Image: "agent:latest",
				Role:  "operator",
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	admin := cf.Services["agent-admin"]
	for _, env := range admin.Environment {
		if strings.HasPrefix(env, "ANGEE_OPERATOR_API_KEY=") {
			t.Error("should not inject empty API key")
		}
	}
}

func TestCompileSkillMergesMCPServers(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "skill-test",
		MCPServers: map[string]config.MCPServerSpec{
			"operator": {Transport: "streamable-http", URL: "http://operator:9000/mcp"},
			"github":   {Transport: "sse", URL: "http://github:8080/sse"},
		},
		Skills: map[string]config.SkillSpec{
			"deploy-skill": {
				Description:  "Deployment capability",
				MCPServers:   []string{"operator"},
				SystemPrompt: "You can deploy the platform.",
			},
		},
		Agents: map[string]config.AgentSpec{
			"bot": {
				Image:      "agent:latest",
				MCPServers: []string{"github"},
				Skills:     []string{"deploy-skill"},
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	svc := cf.Services["agent-bot"]

	// Should have both github (direct) and operator (from skill) MCP URLs
	hasGithub := false
	hasOperator := false
	hasMCPList := false
	hasSkillPrompt := false
	for _, env := range svc.Environment {
		if env == "ANGEE_MCP_GITHUB_URL=http://github:8080/sse" {
			hasGithub = true
		}
		if env == "ANGEE_MCP_OPERATOR_URL=http://operator:9000/mcp" {
			hasOperator = true
		}
		if env == "ANGEE_MCP_SERVERS=github,operator" {
			hasMCPList = true
		}
		if strings.HasPrefix(env, "ANGEE_SKILL_PROMPT_DEPLOY_SKILL=") {
			hasSkillPrompt = true
		}
	}

	if !hasGithub {
		t.Error("expected ANGEE_MCP_GITHUB_URL from direct MCP server")
	}
	if !hasOperator {
		t.Error("expected ANGEE_MCP_OPERATOR_URL from skill MCP server")
	}
	if !hasMCPList {
		t.Errorf("expected ANGEE_MCP_SERVERS=github,operator, got env: %v", svc.Environment)
	}
	if !hasSkillPrompt {
		t.Error("expected ANGEE_SKILL_PROMPT_DEPLOY_SKILL from skill system prompt")
	}
}

func TestCompileSkillNoDuplicateMCPServers(t *testing.T) {
	c := newTestCompiler(t)
	cfg := &config.AngeeConfig{
		Name: "skill-dedup-test",
		MCPServers: map[string]config.MCPServerSpec{
			"operator": {Transport: "streamable-http", URL: "http://operator:9000/mcp"},
		},
		Skills: map[string]config.SkillSpec{
			"deploy-skill": {
				MCPServers: []string{"operator"},
			},
		},
		Agents: map[string]config.AgentSpec{
			"bot": {
				Image:      "agent:latest",
				MCPServers: []string{"operator"}, // already has operator
				Skills:     []string{"deploy-skill"},
			},
		},
	}

	cf, err := c.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	svc := cf.Services["agent-bot"]
	count := 0
	for _, env := range svc.Environment {
		if env == "ANGEE_MCP_OPERATOR_URL=http://operator:9000/mcp" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 ANGEE_MCP_OPERATOR_URL, got %d", count)
	}
}
