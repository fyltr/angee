package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/fyltr/angee-go/internal/config"
	"github.com/fyltr/angee-go/internal/runtime"
)

// handleHealth returns operator liveness.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok", "root": s.Root.Path, "runtime": s.Cfg.Runtime})
}

// handleConfigGet returns the current angee.yaml content.
func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, cfg)
}

// handleConfigSet validates, commits, and optionally deploys a new angee.yaml.
func (s *Server) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
		Message string `json:"message"`
		Deploy  bool   `json:"deploy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if req.Content == "" {
		jsonErr(w, 400, "content is required")
		return
	}
	if req.Message == "" {
		req.Message = "angee-agent: update config"
	}

	// Write + validate
	if err := s.Root.WriteAngeeYAML(req.Content); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	// Verify it parses
	if _, err := s.Root.LoadAngeeConfig(); err != nil {
		jsonErr(w, 400, fmt.Sprintf("invalid angee.yaml: %s", err))
		return
	}

	// Commit
	sha, err := s.Root.CommitConfig(req.Message)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	resp := map[string]any{"sha": sha, "message": req.Message}

	if req.Deploy {
		result, err := s.deploy(r.Context(), nil)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		resp["deploy"] = result
	}

	jsonOK(w, resp)
}

// handleDeploy compiles angee.yaml and applies it to the runtime.
func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck

	result, err := s.deploy(r.Context(), nil)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonOK(w, result)
}

// deploy is the internal deploy implementation.
func (s *Server) deploy(ctx context.Context, _ *config.AngeeConfig) (*runtime.ApplyResult, error) {
	angeeConfig, err := s.Root.LoadAngeeConfig()
	if err != nil {
		return nil, err
	}

	// Ensure per-agent directories exist
	for agentName := range angeeConfig.Agents {
		if err := s.Root.EnsureAgentDir(agentName); err != nil {
			return nil, fmt.Errorf("agent dir for %s: %w", agentName, err)
		}
	}

	// Compile → docker-compose.yaml
	if err := s.compileAndWrite(angeeConfig); err != nil {
		return nil, fmt.Errorf("compiling: %w", err)
	}

	// Apply
	result, err := s.Backend.Apply(ctx, s.Root.ComposePath())
	if err != nil {
		return nil, fmt.Errorf("applying: %w", err)
	}

	s.Log.Info("deployed",
		"started", len(result.ServicesStarted),
		"updated", len(result.ServicesUpdated),
		"removed", len(result.ServicesRemoved),
	)

	return result, nil
}

// handleRollback reverts to a previous git commit and redeploys.
func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SHA string `json:"sha"` // git SHA or relative ref like "HEAD~1"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SHA == "" {
		jsonErr(w, 400, "sha is required")
		return
	}

	if err := s.Git.Revert(req.SHA); err != nil {
		// Try reset for relative refs
		if err2 := s.Git.ResetHard(req.SHA); err2 != nil {
			jsonErr(w, 500, fmt.Sprintf("rollback failed: %s", err))
			return
		}
	}

	result, err := s.deploy(r.Context(), nil)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	sha, _ := s.Git.CurrentSHA()
	jsonOK(w, map[string]any{
		"rolled_back_to": sha,
		"deploy":         result,
	})
}

// handlePlan shows what deploy would change without applying.
func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	// Write compose file to temp to diff against
	if err := s.compileAndWrite(cfg); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	changeset, err := s.Backend.Diff(r.Context(), s.Root.ComposePath())
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonOK(w, changeset)
}

// handleStatus returns the current runtime state of all services and agents.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.Backend.Status(r.Context())
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if statuses == nil {
		statuses = []*runtime.ServiceStatus{}
	}
	jsonOK(w, statuses)
}

// handleLogs streams or returns logs for a service.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	follow := r.URL.Query().Get("follow") == "true" || r.URL.Query().Get("follow") == "1"
	lines := 100
	if n := r.URL.Query().Get("lines"); n != "" {
		if parsed, err := strconv.Atoi(n); err == nil {
			lines = parsed
		}
	}

	rc, err := s.Backend.Logs(r.Context(), service, runtime.LogOptions{
		Follow: follow,
		Lines:  lines,
		Since:  r.URL.Query().Get("since"),
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.Copy(w, rc) //nolint:errcheck
}

// handleScale adjusts replica count for a service.
func (s *Server) handleScale(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	var req struct {
		Replicas int `json:"replicas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Replicas < 0 {
		jsonErr(w, 400, "replicas must be a non-negative integer")
		return
	}

	if err := s.Backend.Scale(r.Context(), service, req.Replicas); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonOK(w, map[string]any{"service": service, "replicas": req.Replicas})
}

// handleDown brings the entire workspace stack down.
func (s *Server) handleDown(w http.ResponseWriter, r *http.Request) {
	if err := s.Backend.Down(r.Context()); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "down"})
}

// handleAgentList returns all agents defined in angee.yaml with their status.
func (s *Server) handleAgentList(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	statuses, _ := s.Backend.Status(r.Context())
	statusMap := make(map[string]*runtime.ServiceStatus)
	for _, s := range statuses {
		statusMap[s.Name] = s
	}

	type AgentInfo struct {
		Name      string `json:"name"`
		Lifecycle string `json:"lifecycle"`
		Role      string `json:"role"`
		Status    string `json:"status"`
		Health    string `json:"health"`
	}

	var agents []AgentInfo
	for name, agent := range cfg.Agents {
		status := "stopped"
		health := "unknown"
		if s, ok := statusMap["agent-"+name]; ok {
			status = s.Status
			health = s.Health
		}
		agents = append(agents, AgentInfo{
			Name:      name,
			Lifecycle: agent.Lifecycle,
			Role:      agent.Role,
			Status:    status,
			Health:    health,
		})
	}

	jsonOK(w, agents)
}

// handleAgentStart starts a stopped agent.
func (s *Server) handleAgentStart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	if _, ok := cfg.Agents[name]; !ok {
		jsonErr(w, 404, fmt.Sprintf("agent %q not found in angee.yaml", name))
		return
	}

	if err := s.Root.EnsureAgentDir(name); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	// Recompile and restart just this agent
	if err := s.compileAndWrite(cfg); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	result, err := s.Backend.Apply(r.Context(), s.Root.ComposePath())
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonOK(w, result)
}

// handleAgentStop stops a running agent.
func (s *Server) handleAgentStop(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.Backend.Stop(r.Context(), "agent-"+name); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "stopped", "agent": name})
}

// handleAgentLogs streams logs for a specific agent.
func (s *Server) handleAgentLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// Reuse the logs handler with the agent service name
	follow := r.URL.Query().Get("follow") == "true"

	rc, err := s.Backend.Logs(r.Context(), "agent-"+name, runtime.LogOptions{
		Follow: follow,
		Lines:  200,
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.Copy(w, rc) //nolint:errcheck
}

// handleHistory returns recent git commits.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	n := 20
	if nStr := r.URL.Query().Get("n"); nStr != "" {
		if parsed, err := strconv.Atoi(nStr); err == nil && parsed > 0 {
			n = parsed
		}
	}

	commits, err := s.Git.Log(n)
	if err != nil {
		// No commits yet
		jsonOK(w, []any{})
		return
	}

	type CommitResp struct {
		SHA     string `json:"sha"`
		Message string `json:"message"`
		Author  string `json:"author"`
		Date    string `json:"date"`
	}

	var resp []CommitResp
	for _, c := range commits {
		resp = append(resp, CommitResp{
			SHA:     c.SHA,
			Message: c.Message,
			Author:  c.Author,
			Date:    c.Date,
		})
	}

	jsonOK(w, resp)
}

