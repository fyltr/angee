package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fyltr/angee/api"
)

// ── JSON-RPC 2.0 types ─────────────────────────────────────────────────────

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id"`
}

type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
	ID      any           `json:"id"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ── Handler ─────────────────────────────────────────────────────────────────

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, fmt.Errorf("POST required"))
		return
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		mcpWriteError(w, nil, -32700, "parse error")
		return
	}
	if req.JSONRPC != "2.0" {
		mcpWriteError(w, req.ID, -32600, "invalid request: jsonrpc must be \"2.0\"")
		return
	}

	switch req.Method {
	case "initialize":
		mcpWriteResult(w, req.ID, map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "angee-operator", "version": "0.1.0"},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusNoContent)
	case "tools/list":
		mcpWriteResult(w, req.ID, map[string]any{"tools": mcpToolDefinitions()})
	case "tools/call":
		s.handleMCPToolCall(w, r.Context(), req)
	default:
		mcpWriteError(w, req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleMCPToolCall(w http.ResponseWriter, ctx context.Context, req jsonRPCRequest) {
	var params mcpToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		mcpWriteError(w, req.ID, -32602, "invalid params")
		return
	}

	result, err := s.dispatchTool(ctx, params.Name, params.Arguments)
	if err != nil {
		mcpWriteResult(w, req.ID, mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		})
		return
	}

	text, err := json.Marshal(result)
	if err != nil {
		mcpWriteResult(w, req.ID, mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: "failed to serialize result"}},
			IsError: true,
		})
		return
	}

	mcpWriteResult(w, req.ID, mcpToolResult{
		Content: []mcpContent{{Type: "text", Text: string(text)}},
	})
}

// ── Dispatch: every case is unmarshal args → call Platform method ────────

func (s *Server) dispatchTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	switch name {
	// Platform
	case "platform_health":
		return s.Platform.HealthCheck(), nil
	case "platform_status":
		return s.Platform.Status(ctx)
	case "platform_down":
		return api.DownResponse{Status: "down"}, s.Platform.Down(ctx)

	// Config
	case "config_get":
		return s.Platform.ConfigGet()
	case "config_set":
		var p api.ConfigSetRequest
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		return s.Platform.ConfigSet(ctx, p.Content, p.Message, p.Deploy)

	// Deploy
	case "deploy":
		return s.Platform.Deploy(ctx, "")
	case "deploy_plan":
		return s.Platform.Plan(ctx)
	case "deploy_rollback":
		var p api.RollbackRequest
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		return s.Platform.Rollback(ctx, p.SHA)

	// Services
	case "service_logs":
		var p struct {
			Service string `json:"service"`
			Lines   int    `json:"lines"`
		}
		json.Unmarshal(args, &p) //nolint:errcheck
		return s.Platform.ServiceLogsText(ctx, p.Service, p.Lines)
	case "service_scale":
		var p api.ScaleRequest
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		return s.Platform.Scale(ctx, p.Service, p.Replicas)

	// Agents
	case "agent_list":
		return s.Platform.AgentList(ctx)
	case "agent_start":
		var p struct{ Name string `json:"name"` }
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		return s.Platform.AgentStart(ctx, p.Name)
	case "agent_stop":
		var p struct{ Name string `json:"name"` }
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		return api.AgentActionResponse{Status: "stopped", Agent: p.Name}, s.Platform.AgentStop(ctx, p.Name)
	case "agent_logs":
		var p struct {
			Name  string `json:"name"`
			Lines int    `json:"lines"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		return s.Platform.AgentLogs(ctx, p.Name, p.Lines)

	// Connectors
	case "connector_list":
		var p struct{ Tags []string `json:"tags"` }
		json.Unmarshal(args, &p) //nolint:errcheck
		return s.Platform.ConnectorList(p.Tags)
	case "connector_create":
		var p api.ConnectorCreateRequest
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		return s.Platform.ConnectorCreate(ctx, p)
	case "connector_delete":
		var p struct{ Name string `json:"name"` }
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		return nil, s.Platform.ConnectorDelete(ctx, p.Name)

	// Credentials
	case "credentials_list":
		return nil, fmt.Errorf("credentials backend not configured")
	case "credentials_set":
		return nil, fmt.Errorf("credentials backend not configured")
	case "credentials_delete":
		return nil, fmt.Errorf("credentials backend not configured")

	// History
	case "history":
		var p struct{ N int `json:"n"` }
		json.Unmarshal(args, &p) //nolint:errcheck
		return s.Platform.History(p.N)

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func mcpWriteResult(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", Result: result, ID: id}) //nolint:errcheck
}

func mcpWriteError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", Error: &jsonRPCError{Code: code, Message: msg}, ID: id}) //nolint:errcheck
}

// ── Tool definitions ────────────────────────────────────────────────────────

func mcpToolDefinitions() []mcpToolDef {
	obj := map[string]any{"type": "object", "properties": map[string]any{}}
	return []mcpToolDef{
		{Name: "platform_health", Description: "Check operator liveness", InputSchema: obj},
		{Name: "platform_status", Description: "Get runtime state of all services and agents", InputSchema: obj},
		{Name: "config_get", Description: "Read the current angee.yaml configuration", InputSchema: obj},
		{Name: "config_set", Description: "Write a new angee.yaml, validate, and commit", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{"type": "string", "description": "Raw YAML content"},
				"message": map[string]any{"type": "string", "description": "Git commit message"},
				"deploy":  map[string]any{"type": "boolean", "description": "Deploy immediately after saving"},
			},
			"required": []string{"content"},
		}},
		{Name: "deploy", Description: "Compile angee.yaml and apply to runtime", InputSchema: obj},
		{Name: "deploy_plan", Description: "Dry-run: see what would change without deploying", InputSchema: obj},
		{Name: "deploy_rollback", Description: "Roll back to a previous configuration", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sha": map[string]any{"type": "string", "description": "Git commit SHA to roll back to"},
			},
			"required": []string{"sha"},
		}},
		{Name: "service_logs", Description: "Get logs for a service", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"service": map[string]any{"type": "string", "description": "Service name (omit for all)"},
				"lines":   map[string]any{"type": "integer", "description": "Number of lines (default: 100)"},
			},
		}},
		{Name: "service_scale", Description: "Scale a service to N replicas", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"service":  map[string]any{"type": "string", "description": "Service name"},
				"replicas": map[string]any{"type": "integer", "description": "Desired replica count"},
			},
			"required": []string{"service", "replicas"},
		}},
		{Name: "platform_down", Description: "Bring the entire stack down", InputSchema: obj},
		{Name: "agent_list", Description: "List all agents and their status", InputSchema: obj},
		{Name: "agent_start", Description: "Start a stopped agent", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Agent name"},
			},
			"required": []string{"name"},
		}},
		{Name: "agent_stop", Description: "Stop a running agent", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Agent name"},
			},
			"required": []string{"name"},
		}},
		{Name: "agent_logs", Description: "Get logs for an agent", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string", "description": "Agent name"},
				"lines": map[string]any{"type": "integer", "description": "Number of lines (default: 200)"},
			},
			"required": []string{"name"},
		}},
		{Name: "connector_list", Description: "List all connectors", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Filter by tags"},
			},
		}},
		{Name: "connector_create", Description: "Create a new connector", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":        map[string]any{"type": "string"},
				"provider":    map[string]any{"type": "string"},
				"type":        map[string]any{"type": "string", "description": "oauth | api_key | token | setup_command"},
				"description": map[string]any{"type": "string"},
				"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"name", "provider", "type"},
		}},
		{Name: "connector_delete", Description: "Delete a connector", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Connector name"},
			},
			"required": []string{"name"},
		}},
		{Name: "history", Description: "Get config change history", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"n": map[string]any{"type": "integer", "description": "Number of commits (default: 20)"},
			},
		}},
	}
}
