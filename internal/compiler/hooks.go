package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/fyltr/angee-go/internal/config"
)

// AgentFileData is the data passed to agent config file templates.
type AgentFileData struct {
	AgentName  string
	Agent      config.AgentSpec
	MCPServers map[string]config.MCPServerSpec // only this agent's resolved servers
}

// templateFuncs is the FuncMap available to agent config templates.
var templateFuncs = template.FuncMap{
	"opencodeMCP": opencodeMCP,
	"toJSON":      toJSON,
}

// RenderAgentFiles renders template-based config files declared in agent.Files.
// rootPath is the ANGEE_ROOT directory. agentDir is agents/<name>/ (absolute).
// Only files with Template set are rendered; Source entries are handled at
// compile time (volume mounts) and skipped here.
//
// Files whose mount path falls under /workspace/ are written directly into
// the agent's workspace directory (so they're included in the workspace bind
// mount). Other files are written to the agent directory and need a separate
// volume mount added by the compiler.
func RenderAgentFiles(rootPath, agentDir string, agent config.AgentSpec, allMCP map[string]config.MCPServerSpec) error {
	// Resolve the MCP servers this agent references.
	resolved := make(map[string]config.MCPServerSpec)
	for _, name := range agent.MCPServers {
		if spec, ok := allMCP[name]; ok {
			resolved[name] = spec
		}
	}

	data := AgentFileData{
		AgentName:  filepath.Base(agentDir),
		Agent:      agent,
		MCPServers: resolved,
	}

	for _, f := range agent.Files {
		if f.Template == "" {
			continue // source mount — nothing to render
		}

		tmplPath := filepath.Join(rootPath, f.Template)
		content, err := os.ReadFile(tmplPath)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", f.Template, err)
		}

		t, err := template.New(filepath.Base(f.Template)).Funcs(templateFuncs).Parse(string(content))
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", f.Template, err)
		}

		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			return fmt.Errorf("rendering template %s: %w", f.Template, err)
		}

		outPath := RenderedFilePath(agentDir, f)
		if dir := filepath.Dir(outPath); dir != agentDir {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", outPath, err)
			}
		}
		if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("writing rendered file %s: %w", outPath, err)
		}
	}

	return nil
}

// RenderedFilePath returns the host path where a rendered template file
// should be written. Files mounting under /workspace/ go into the workspace
// subdirectory (avoiding nested bind mount conflicts). Others go into the
// agent directory root.
func RenderedFilePath(agentDir string, f config.FileMount) string {
	if isUnderWorkspace(f.Mount) {
		// Strip the /workspace/ prefix and write into the workspace subdir.
		rel := strings.TrimPrefix(f.Mount, "/workspace/")
		return filepath.Join(agentDir, "workspace", rel)
	}
	return filepath.Join(agentDir, filepath.Base(f.Mount))
}

// isUnderWorkspace returns true if a container mount path is under /workspace/.
func isUnderWorkspace(mount string) bool {
	return mount == "/workspace" || strings.HasPrefix(mount, "/workspace/")
}

// --- Template functions ---

// opencodeMCP generates a complete opencode.json config from MCP server specs.
func opencodeMCP(mcpServers map[string]config.MCPServerSpec) (string, error) {
	type mcpDef struct {
		Type    string   `json:"type"`              // "remote" or "local"
		URL     string   `json:"url,omitempty"`     // for remote
		Command []string `json:"command,omitempty"` // for local (command + args merged)
		Enabled bool     `json:"enabled"`
	}

	type ocConfig struct {
		Schema string            `json:"$schema"`
		MCP    map[string]mcpDef `json:"mcp"`
	}

	oc := ocConfig{
		Schema: "https://opencode.ai/config.json",
		MCP:    make(map[string]mcpDef),
	}

	for name, spec := range mcpServers {
		switch spec.Transport {
		case "streamable-http", "sse":
			oc.MCP[name] = mcpDef{
				Type:    "remote",
				URL:     spec.URL,
				Enabled: true,
			}
		case "stdio":
			cmd := append(spec.Command, spec.Args...)
			oc.MCP[name] = mcpDef{
				Type:    "local",
				Command: cmd,
				Enabled: true,
			}
		}
	}

	data, err := json.MarshalIndent(oc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// toJSON marshals any value to indented JSON.
func toJSON(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ExpandHome replaces a leading ~ with the user's home directory.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
