package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/fyltr/angee/api"
	"github.com/fyltr/angee/internal/compiler"
	"github.com/fyltr/angee/internal/runtime"
)

// ── JSON-RPC 2.0 types ──────────────────────────────────────────────────────

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
	ID      any           `json:"id"`
}

// jsonRPCError is a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ── MCP protocol types ───────────────────────────────────────────────────────

// mcpToolCallParams is the params object for tools/call.
type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// mcpContent is a single content item in an MCP tool result.
type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// mcpToolResult is the result of a tools/call.
type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// mcpToolDef describes one tool for tools/list.
type mcpToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// mcpInitializeResult is returned by initialize.
type mcpInitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      mcpServerInfo  `json:"serverInfo"`
}

type mcpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ── Handler ──────────────────────────────────────────────────────────────────

// handleMCP handles JSON-RPC 2.0 requests from MCP clients (agents).
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, http.StatusMethodNotAllowed, "POST required")
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
		mcpWriteResult(w, req.ID, mcpInitializeResult{
			ProtocolVersion: "2025-03-26",
			Capabilities:    map[string]any{"tools": map[string]any{}},
			ServerInfo:      mcpServerInfo{Name: "angee-operator", Version: "0.1.0"},
		})

	case "notifications/initialized":
		// Client acknowledgement — no response needed for notifications.
		w.WriteHeader(http.StatusNoContent)

	case "tools/list":
		mcpWriteResult(w, req.ID, map[string]any{"tools": mcpToolDefinitions()})

	case "tools/call":
		s.handleMCPToolCall(w, r.Context(), req)

	default:
		mcpWriteError(w, req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

// handleMCPToolCall dispatches a tools/call request to the appropriate
// business logic method.
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

// dispatchTool routes a tool name to the corresponding business logic.
func (s *Server) dispatchTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
	switch name {
	case "platform_health":
		return s.toolHealth()
	case "platform_status":
		return s.toolStatus(ctx)
	case "config_get":
		return s.toolConfigGet()
	case "config_set":
		return s.toolConfigSet(ctx, args)
	case "deploy":
		return s.toolDeploy(ctx)
	case "deploy_plan":
		return s.toolPlan(ctx)
	case "deploy_rollback":
		return s.toolRollback(ctx, args)
	case "service_logs":
		return s.toolServiceLogs(ctx, args)
	case "service_scale":
		return s.toolServiceScale(ctx, args)
	case "platform_down":
		return s.toolDown(ctx)
	case "agent_list":
		return s.toolAgentList(ctx)
	case "agent_start":
		return s.toolAgentStart(ctx, args)
	case "agent_stop":
		return s.toolAgentStop(ctx, args)
	case "agent_logs":
		return s.toolAgentLogs(ctx, args)
	case "history":
		return s.toolHistory(args)
	case "credentials_list":
		return s.toolCredentialsList(ctx)
	case "credentials_set":
		return s.toolCredentialsSet(ctx, args)
	case "credentials_delete":
		return s.toolCredentialsDelete(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// ── Tool implementations ─────────────────────────────────────────────────────
// Each method extracts business logic from the corresponding HTTP handler.

func (s *Server) toolHealth() (*api.HealthResponse, error) {
	return &api.HealthResponse{Status: "ok", Root: s.Root.Path, Runtime: s.Cfg.Runtime}, nil
}

func (s *Server) toolStatus(ctx context.Context) ([]*runtime.ServiceStatus, error) {
	statuses, err := s.Backend.Status(ctx)
	if err != nil {
		return nil, err
	}
	if statuses == nil {
		statuses = []*runtime.ServiceStatus{}
	}
	return statuses, nil
}

func (s *Server) toolConfigGet() (*api.ConfigGetResponse, error) {
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		return nil, err
	}
	return &api.ConfigGetResponse{Config: cfg}, nil
}

func (s *Server) toolConfigSet(ctx context.Context, args json.RawMessage) (*api.ConfigSetResponse, error) {
	var req api.ConfigSetRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	if req.Message == "" {
		req.Message = "angee-agent: update config"
	}

	if err := s.Root.WriteAngeeYAML(req.Content); err != nil {
		return nil, err
	}
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		return nil, fmt.Errorf("invalid angee.yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	sha, err := s.Root.CommitConfig(req.Message)
	if err != nil {
		return nil, err
	}

	resp := &api.ConfigSetResponse{SHA: sha, Message: req.Message}

	if req.Deploy {
		result, err := s.deploy(ctx, nil)
		if err != nil {
			return nil, err
		}
		resp.Deploy = applyResultToAPI(result)
	}

	return resp, nil
}

func (s *Server) toolDeploy(ctx context.Context) (*api.ApplyResult, error) {
	result, err := s.deploy(ctx, nil)
	if err != nil {
		return nil, err
	}
	return applyResultToAPI(result), nil
}

func (s *Server) toolPlan(ctx context.Context) (*api.ChangeSet, error) {
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		return nil, err
	}
	if err := s.compileAndWrite(cfg); err != nil {
		return nil, err
	}
	changeset, err := s.Backend.Diff(ctx, s.Root.ComposePath())
	if err != nil {
		return nil, err
	}
	return &api.ChangeSet{Add: changeset.Add, Update: changeset.Update, Remove: changeset.Remove}, nil
}

func (s *Server) toolRollback(ctx context.Context, args json.RawMessage) (*api.RollbackResponse, error) {
	var req struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(args, &req); err != nil || req.SHA == "" {
		return nil, fmt.Errorf("sha is required")
	}

	if err := s.Git.Revert(req.SHA); err != nil {
		if err2 := s.Git.ResetHard(req.SHA); err2 != nil {
			return nil, fmt.Errorf("rollback failed: %w", err)
		}
	}

	result, err := s.deploy(ctx, nil)
	if err != nil {
		return nil, err
	}

	sha, _ := s.Git.CurrentSHA()
	return &api.RollbackResponse{RolledBackTo: sha, Deploy: applyResultToAPI(result)}, nil
}

func (s *Server) toolServiceLogs(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Service string `json:"service"`
		Lines   int    `json:"lines"`
		Since   string `json:"since"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if params.Lines <= 0 {
		params.Lines = 100
	}

	rc, err := s.Backend.Logs(ctx, params.Service, runtime.LogOptions{
		Lines: params.Lines,
		Since: params.Since,
	})
	if err != nil {
		return "", err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Server) toolServiceScale(ctx context.Context, args json.RawMessage) (*api.ScaleResponse, error) {
	var req struct {
		Service  string `json:"service"`
		Replicas int    `json:"replicas"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if req.Service == "" {
		return nil, fmt.Errorf("service is required")
	}
	if err := s.Backend.Scale(ctx, req.Service, req.Replicas); err != nil {
		return nil, err
	}
	return &api.ScaleResponse{Service: req.Service, Replicas: req.Replicas}, nil
}

func (s *Server) toolDown(ctx context.Context) (*api.DownResponse, error) {
	if err := s.Backend.Down(ctx); err != nil {
		return nil, err
	}
	return &api.DownResponse{Status: "down"}, nil
}

func (s *Server) toolAgentList(ctx context.Context) ([]api.AgentInfo, error) {
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		return nil, err
	}

	statuses, _ := s.Backend.Status(ctx)
	statusMap := make(map[string]*runtime.ServiceStatus)
	for _, st := range statuses {
		statusMap[st.Name] = st
	}

	var agents []api.AgentInfo
	for name, agent := range cfg.Agents {
		status := "stopped"
		health := "unknown"
		if st, ok := statusMap[compiler.AgentServicePrefix+name]; ok {
			status = st.Status
			health = st.Health
		}
		agents = append(agents, api.AgentInfo{
			Name:      name,
			Lifecycle: agent.Lifecycle,
			Role:      agent.Role,
			Status:    status,
			Health:    health,
		})
	}
	return agents, nil
}

func (s *Server) toolAgentStart(ctx context.Context, args json.RawMessage) (*api.ApplyResult, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &req); err != nil || req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		return nil, err
	}
	if _, ok := cfg.Agents[req.Name]; !ok {
		return nil, fmt.Errorf("agent %q not found in angee.yaml", req.Name)
	}

	if err := s.prepareAndCompile(cfg); err != nil {
		return nil, err
	}

	result, err := s.Backend.Apply(ctx, s.Root.ComposePath())
	if err != nil {
		return nil, err
	}
	return applyResultToAPI(result), nil
}

func (s *Server) toolAgentStop(ctx context.Context, args json.RawMessage) (*api.AgentActionResponse, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &req); err != nil || req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.Backend.Stop(ctx, compiler.AgentServicePrefix+req.Name); err != nil {
		return nil, err
	}
	return &api.AgentActionResponse{Status: "stopped", Agent: req.Name}, nil
}

func (s *Server) toolAgentLogs(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Name  string `json:"name"`
		Lines int    `json:"lines"`
	}
	if err := json.Unmarshal(args, &params); err != nil || params.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if params.Lines <= 0 {
		params.Lines = 200
	}

	rc, err := s.Backend.Logs(ctx, compiler.AgentServicePrefix+params.Name, runtime.LogOptions{
		Lines: params.Lines,
	})
	if err != nil {
		return "", err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Server) toolHistory(args json.RawMessage) ([]api.CommitInfo, error) {
	var params struct {
		N int `json:"n"`
	}
	json.Unmarshal(args, &params) //nolint:errcheck
	n := params.N
	if n <= 0 {
		n = 20
	}

	commits, err := s.Git.Log(n)
	if err != nil {
		return []api.CommitInfo{}, nil
	}

	var resp []api.CommitInfo
	for _, c := range commits {
		resp = append(resp, api.CommitInfo{
			SHA:     c.SHA,
			Message: c.Message,
			Author:  c.Author,
			Date:    c.Date,
		})
	}
	return resp, nil
}

// ── Credential tool implementations ──────────────────────────────────────────

func (s *Server) toolCredentialsList(ctx context.Context) (any, error) {
	if s.Credentials == nil {
		return nil, fmt.Errorf("credentials backend not configured")
	}
	names, err := s.Credentials.List(ctx)
	if err != nil {
		return nil, err
	}
	if names == nil {
		names = []string{}
	}
	return map[string]any{"names": names, "backend": s.Credentials.Type()}, nil
}

func (s *Server) toolCredentialsSet(ctx context.Context, args json.RawMessage) (any, error) {
	if s.Credentials == nil {
		return nil, fmt.Errorf("credentials backend not configured")
	}
	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if req.Name == "" || req.Value == "" {
		return nil, fmt.Errorf("name and value are required")
	}
	if err := s.Credentials.Set(ctx, req.Name, req.Value); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok", "name": req.Name}, nil
}

func (s *Server) toolCredentialsDelete(ctx context.Context, args json.RawMessage) (any, error) {
	if s.Credentials == nil {
		return nil, fmt.Errorf("credentials backend not configured")
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.Credentials.Delete(ctx, req.Name); err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok", "name": req.Name}, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func mcpWriteResult(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", Result: result, ID: id}) //nolint:errcheck
}

func mcpWriteError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", Error: &jsonRPCError{Code: code, Message: msg}, ID: id}) //nolint:errcheck
}

// ── Tool definitions ─────────────────────────────────────────────────────────

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
				"since":   map[string]any{"type": "string", "description": "Only logs after this timestamp"},
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
		{Name: "history", Description: "Get config change history", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"n": map[string]any{"type": "integer", "description": "Number of commits (default: 20)"},
			},
		}},
		{Name: "credentials_list", Description: "List all credential names and backend type", InputSchema: obj},
		{Name: "credentials_set", Description: "Store a credential value", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string", "description": "Credential name"},
				"value": map[string]any{"type": "string", "description": "Credential value"},
			},
			"required": []string{"name", "value"},
		}},
		{Name: "credentials_delete", Description: "Delete a credential", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Credential name"},
			},
			"required": []string{"name"},
		}},
	}
}

// intDefault returns n if positive, otherwise def.
func intDefault(n, def int) int {
	if n > 0 {
		return n
	}
	return def
}

// parseInt parses a string to int, returning def on failure.
func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
