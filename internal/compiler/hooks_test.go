package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fyltr/angee-go/internal/config"
)

func TestRenderAgentFilesTemplate(t *testing.T) {
	rootDir := t.TempDir()
	agentDir := t.TempDir()

	// Create workspace subdir (normally created by EnsureAgentDir)
	if err := os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755); err != nil {
		t.Fatal(err)
	}

	// Write a template file into the "root"
	tmplContent := `{{ opencodeMCP .MCPServers }}
`
	if err := os.WriteFile(filepath.Join(rootDir, "opencode.json.tmpl"), []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	agent := config.AgentSpec{
		Image:      "ghcr.io/anomalyco/opencode:latest",
		MCPServers: []string{"angee-operator", "angee-files", "django-mcp"},
		Files: []config.FileMount{
			{
				Template: "opencode.json.tmpl",
				Mount:    "/workspace/opencode.json",
			},
		},
	}

	allMCP := map[string]config.MCPServerSpec{
		"angee-operator": {
			Transport: "streamable-http",
			URL:       "http://operator:9000/mcp",
		},
		"angee-files": {
			Transport: "stdio",
			Command:   []string{"node", "/usr/local/lib/mcp-filesystem/dist/index.js"},
			Args:      []string{"/workspace"},
		},
		"django-mcp": {
			Transport: "sse",
			URL:       "http://django:8000/mcp/",
		},
	}

	if err := RenderAgentFiles(rootDir, agentDir, agent, allMCP); err != nil {
		t.Fatalf("RenderAgentFiles() error: %v", err)
	}

	// File should be in workspace/ subdir since mount is under /workspace/
	data, err := os.ReadFile(filepath.Join(agentDir, "workspace", "opencode.json"))
	if err != nil {
		t.Fatalf("reading opencode.json: %v", err)
	}

	var oc struct {
		Schema string `json:"$schema"`
		MCP    map[string]struct {
			Type    string   `json:"type"`
			URL     string   `json:"url,omitempty"`
			Command []string `json:"command,omitempty"`
			Enabled bool     `json:"enabled"`
		} `json:"mcp"`
	}
	if err := json.Unmarshal(data, &oc); err != nil {
		t.Fatalf("parsing opencode.json: %v", err)
	}

	if oc.Schema != "https://opencode.ai/config.json" {
		t.Errorf("schema = %q, want opencode.ai schema", oc.Schema)
	}

	// Check remote server (streamable-http)
	op, ok := oc.MCP["angee-operator"]
	if !ok {
		t.Fatal("expected angee-operator in MCP config")
	}
	if op.Type != "remote" {
		t.Errorf("angee-operator type = %q, want %q", op.Type, "remote")
	}
	if op.URL != "http://operator:9000/mcp" {
		t.Errorf("angee-operator url = %q", op.URL)
	}
	if !op.Enabled {
		t.Error("angee-operator should be enabled")
	}

	// Check remote server (sse)
	dj, ok := oc.MCP["django-mcp"]
	if !ok {
		t.Fatal("expected django-mcp in MCP config")
	}
	if dj.Type != "remote" {
		t.Errorf("django-mcp type = %q, want %q", dj.Type, "remote")
	}

	// Check local server (stdio)
	files, ok := oc.MCP["angee-files"]
	if !ok {
		t.Fatal("expected angee-files in MCP config")
	}
	if files.Type != "local" {
		t.Errorf("angee-files type = %q, want %q", files.Type, "local")
	}
	wantCmd := []string{"node", "/usr/local/lib/mcp-filesystem/dist/index.js", "/workspace"}
	if len(files.Command) != len(wantCmd) {
		t.Fatalf("angee-files command = %v, want %v", files.Command, wantCmd)
	}
	for i, c := range wantCmd {
		if files.Command[i] != c {
			t.Errorf("angee-files command[%d] = %q, want %q", i, files.Command[i], c)
		}
	}
}

func TestRenderAgentFilesResolvesReferencedServers(t *testing.T) {
	rootDir := t.TempDir()
	agentDir := t.TempDir()
	os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755)

	tmplContent := `{{ opencodeMCP .MCPServers }}
`
	if err := os.WriteFile(filepath.Join(rootDir, "opencode.json.tmpl"), []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	agent := config.AgentSpec{
		Image:      "ghcr.io/anomalyco/opencode:latest",
		MCPServers: []string{"my-server"},
		Files: []config.FileMount{
			{Template: "opencode.json.tmpl", Mount: "/workspace/opencode.json"},
		},
	}

	allMCP := map[string]config.MCPServerSpec{
		"my-server": {
			Transport: "streamable-http",
			URL:       "http://my:8080/mcp",
		},
		"other-server": {
			Transport: "sse",
			URL:       "http://other:9090/sse",
		},
	}

	if err := RenderAgentFiles(rootDir, agentDir, agent, allMCP); err != nil {
		t.Fatalf("RenderAgentFiles() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(agentDir, "workspace", "opencode.json"))
	if err != nil {
		t.Fatalf("reading opencode.json: %v", err)
	}

	var oc struct {
		MCP map[string]json.RawMessage `json:"mcp"`
	}
	if err := json.Unmarshal(data, &oc); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if _, ok := oc.MCP["my-server"]; !ok {
		t.Error("expected my-server in config")
	}
	if _, ok := oc.MCP["other-server"]; ok {
		t.Error("other-server should NOT be in config (not referenced by agent)")
	}
}

func TestRenderAgentFilesNoFilesIsNotError(t *testing.T) {
	agent := config.AgentSpec{
		Image: "nginx:latest",
	}
	if err := RenderAgentFiles(t.TempDir(), t.TempDir(), agent, nil); err != nil {
		t.Fatalf("RenderAgentFiles() error: %v", err)
	}
}

func TestRenderAgentFilesEmptyMCPServers(t *testing.T) {
	rootDir := t.TempDir()
	agentDir := t.TempDir()
	os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755)

	tmplContent := `{{ opencodeMCP .MCPServers }}
`
	if err := os.WriteFile(filepath.Join(rootDir, "opencode.json.tmpl"), []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	agent := config.AgentSpec{
		Image:      "ghcr.io/anomalyco/opencode:latest",
		MCPServers: nil,
		Files: []config.FileMount{
			{Template: "opencode.json.tmpl", Mount: "/workspace/opencode.json"},
		},
	}

	if err := RenderAgentFiles(rootDir, agentDir, agent, nil); err != nil {
		t.Fatalf("RenderAgentFiles() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(agentDir, "workspace", "opencode.json"))
	if err != nil {
		t.Fatalf("reading opencode.json: %v", err)
	}

	var oc struct {
		MCP map[string]json.RawMessage `json:"mcp"`
	}
	if err := json.Unmarshal(data, &oc); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if len(oc.MCP) != 0 {
		t.Errorf("expected empty MCP map, got %d entries", len(oc.MCP))
	}
}

func TestRenderAgentFilesSkipsSourceEntries(t *testing.T) {
	rootDir := t.TempDir()
	agentDir := t.TempDir()

	agent := config.AgentSpec{
		Image: "opencode:latest",
		Files: []config.FileMount{
			{Source: "/some/host/file.json", Mount: "/container/file.json"},
		},
	}

	// Should not error — source entries are skipped during render
	if err := RenderAgentFiles(rootDir, agentDir, agent, nil); err != nil {
		t.Fatalf("RenderAgentFiles() error: %v", err)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"~/.config/auth.json", filepath.Join(home, ".config/auth.json")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tt := range tests {
		got := ExpandHome(tt.input)
		if got != tt.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRenderAgentFilesNonWorkspaceMount(t *testing.T) {
	rootDir := t.TempDir()
	agentDir := t.TempDir()

	tmplContent := `{"test": true}
`
	if err := os.WriteFile(filepath.Join(rootDir, "config.json.tmpl"), []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	agent := config.AgentSpec{
		Image: "agent:latest",
		Files: []config.FileMount{
			{Template: "config.json.tmpl", Mount: "/root/.config/app.json"},
		},
	}

	if err := RenderAgentFiles(rootDir, agentDir, agent, nil); err != nil {
		t.Fatalf("RenderAgentFiles() error: %v", err)
	}

	// Non-workspace mounts write to agentDir root (not workspace/)
	data, err := os.ReadFile(filepath.Join(agentDir, "app.json"))
	if err != nil {
		t.Fatalf("reading app.json: %v", err)
	}
	if string(data) != `{"test": true}`+"\n" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestRenderedFilePath(t *testing.T) {
	tests := []struct {
		mount string
		want  string // relative to agentDir
	}{
		{"/workspace/opencode.json", "workspace/opencode.json"},
		{"/workspace/sub/dir/file.json", "workspace/sub/dir/file.json"},
		{"/root/.config/app.json", "app.json"},
		{"/etc/config.json", "config.json"},
	}

	agentDir := "/agents/test"
	for _, tt := range tests {
		f := config.FileMount{Template: "x.tmpl", Mount: tt.mount}
		got := RenderedFilePath(agentDir, f)
		want := filepath.Join(agentDir, tt.want)
		if got != want {
			t.Errorf("RenderedFilePath(%q) = %q, want %q", tt.mount, got, want)
		}
	}
}

func TestToJSON(t *testing.T) {
	result, err := toJSON(map[string]string{"hello": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "{\n  \"hello\": \"world\"\n}" {
		t.Errorf("unexpected JSON: %s", result)
	}
}
