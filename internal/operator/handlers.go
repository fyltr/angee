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
	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/runtime"
)

// handleHealth returns operator liveness.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, api.HealthResponse{Status: "ok", Root: s.Root.Path, Runtime: s.Cfg.Runtime})
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
	var req api.ConfigSetRequest
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
	cfg, err := s.Root.LoadAngeeConfig()
	if err != nil {
		jsonErr(w, 400, fmt.Sprintf("invalid angee.yaml: %s", err))
		return
	}

	// Structural validation
	if err := cfg.Validate(); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	// Commit
	sha, err := s.Root.CommitConfig(req.Message)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	resp := api.ConfigSetResponse{SHA: sha, Message: req.Message}

	if req.Deploy {
		result, err := s.deploy(r.Context(), nil)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		resp.Deploy = applyResultToAPI(result)
	}

	jsonOK(w, resp)
}

// handleDeploy compiles angee.yaml and applies it to the runtime.
func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	var req api.DeployRequest
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck

	result, err := s.deploy(r.Context(), nil)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonOK(w, applyResultToAPI(result))
}

// prepareAndCompile ensures agent directories exist, renders agent config
// file templates, and compiles angee.yaml → docker-compose.yaml. This is the
// shared sequence used by deploy, agent start, and config set.
func (s *Server) prepareAndCompile(cfg *config.AngeeConfig) error {
	for agentName := range cfg.Agents {
		if err := s.Root.EnsureAgentDir(agentName); err != nil {
			return fmt.Errorf("agent dir for %s: %w", agentName, err)
		}
	}
	for agentName, agent := range cfg.Agents {
		if err := compiler.RenderAgentFiles(s.Root.Path, s.Root.AgentDir(agentName), agent, cfg.MCPServers); err != nil {
			return fmt.Errorf("agent files for %s: %w", agentName, err)
		}
	}
	return s.compileAndWrite(cfg)
}

// deploy is the internal deploy implementation.
func (s *Server) deploy(ctx context.Context, _ *config.AngeeConfig) (*runtime.ApplyResult, error) {
	angeeConfig, err := s.Root.LoadAngeeConfig()
	if err != nil {
		return nil, err
	}

	// Validate config before deploying
	if err := angeeConfig.Validate(); err != nil {
		return nil, err
	}

	if err := s.prepareAndCompile(angeeConfig); err != nil {
		return nil, fmt.Errorf("preparing: %w", err)
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
	var req api.RollbackRequest
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
	jsonOK(w, api.RollbackResponse{
		RolledBackTo: sha,
		Deploy:       applyResultToAPI(result),
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

	jsonOK(w, api.ChangeSet{
		Add:    changeset.Add,
		Update: changeset.Update,
		Remove: changeset.Remove,
	})
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

// parseLogOptions extracts follow, lines, and since from query parameters.
func parseLogOptions(r *http.Request, defaultLines int) runtime.LogOptions {
	follow := r.URL.Query().Get("follow") == "true" || r.URL.Query().Get("follow") == "1"
	lines := defaultLines
	if n := r.URL.Query().Get("lines"); n != "" {
		if parsed, err := strconv.Atoi(n); err == nil {
			lines = parsed
		}
	}
	return runtime.LogOptions{
		Follow: follow,
		Lines:  lines,
		Since:  r.URL.Query().Get("since"),
	}
}

// streamLogs writes an io.ReadCloser to the HTTP response as text/plain.
func streamLogs(w http.ResponseWriter, rc io.ReadCloser) {
	defer rc.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.Copy(w, rc) //nolint:errcheck
}

// handleLogs streams or returns logs for a service.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	opts := parseLogOptions(r, 100)

	rc, err := s.Backend.Logs(r.Context(), service, opts)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	streamLogs(w, rc)
}

// handleScale adjusts replica count for a service.
func (s *Server) handleScale(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	var req api.ScaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Replicas < 0 {
		jsonErr(w, 400, "replicas must be a non-negative integer")
		return
	}

	if err := s.Backend.Scale(r.Context(), service, req.Replicas); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonOK(w, api.ScaleResponse{Service: service, Replicas: req.Replicas})
}

// handleDown brings the entire workspace stack down.
func (s *Server) handleDown(w http.ResponseWriter, r *http.Request) {
	if err := s.Backend.Down(r.Context()); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, api.DownResponse{Status: "down"})
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

	if err := s.prepareAndCompile(cfg); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	result, err := s.Backend.Apply(r.Context(), s.Root.ComposePath())
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonOK(w, applyResultToAPI(result))
}

// handleAgentStop stops a running agent.
func (s *Server) handleAgentStop(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.Backend.Stop(r.Context(), compiler.AgentServicePrefix+name); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, api.AgentActionResponse{Status: "stopped", Agent: name})
}

// handleAgentLogs streams logs for a specific agent.
func (s *Server) handleAgentLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	opts := parseLogOptions(r, 200)

	rc, err := s.Backend.Logs(r.Context(), compiler.AgentServicePrefix+name, opts)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	streamLogs(w, rc)
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

	var resp []api.CommitInfo
	for _, c := range commits {
		resp = append(resp, api.CommitInfo{
			SHA:     c.SHA,
			Message: c.Message,
			Author:  c.Author,
			Date:    c.Date,
		})
	}

	jsonOK(w, resp)
}

// applyResultToAPI converts a runtime.ApplyResult to an api.ApplyResult.
func applyResultToAPI(r *runtime.ApplyResult) *api.ApplyResult {
	if r == nil {
		return nil
	}
	return &api.ApplyResult{
		ServicesStarted: r.ServicesStarted,
		ServicesUpdated: r.ServicesUpdated,
		ServicesRemoved: r.ServicesRemoved,
	}
}
