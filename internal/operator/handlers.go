package operator

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/fyltr/angee/api"
	"github.com/fyltr/angee/internal/runtime"
)

// Every handler: decode request → call Platform method → encode response.

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.Platform.HealthCheck())
}

func (s *Server) handleConfigGet(w http.ResponseWriter, _ *http.Request) {
	result, err := s.Platform.ConfigGet()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	var req api.ConfigSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.Platform.ConfigSet(r.Context(), req.Content, req.Message, req.Deploy)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	var req api.DeployRequest
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	result, err := s.Platform.Deploy(r.Context(), req.Message)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	var req api.RollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.Platform.Rollback(r.Context(), req.SHA)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	result, err := s.Platform.Plan(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.Platform.Status(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, statuses)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	svc := r.PathValue("service")
	opts := parseLogOptions(r, 100)
	rc, err := s.Platform.ServiceLogs(r.Context(), svc, opts)
	if err != nil {
		writeError(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.Copy(w, rc) //nolint:errcheck
}

func (s *Server) handleScale(w http.ResponseWriter, r *http.Request) {
	svc := r.PathValue("service")
	var req api.ScaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.Platform.Scale(r.Context(), svc, req.Replicas)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleDown(w http.ResponseWriter, r *http.Request) {
	if err := s.Platform.Down(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, api.DownResponse{Status: "down"})
}

// ── Agents ──────────────────────────────────────────────────────────────────

func (s *Server) handleAgentList(w http.ResponseWriter, r *http.Request) {
	result, err := s.Platform.AgentList(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleAgentStart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	result, err := s.Platform.AgentStart(r.Context(), name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleAgentStop(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.Platform.AgentStop(r.Context(), name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, api.AgentActionResponse{Status: "stopped", Agent: name})
}

func (s *Server) handleAgentLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	opts := parseLogOptions(r, 200)
	rc, err := s.Platform.ServiceLogs(r.Context(), "agent-"+name, opts)
	if err != nil {
		writeError(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.Copy(w, rc) //nolint:errcheck
}

// ── Connectors ──────────────────────────────────────────────────────────────

func (s *Server) handleConnectorList(w http.ResponseWriter, r *http.Request) {
	tags := r.URL.Query()["tags"]
	result, err := s.Platform.ConnectorList(tags)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleConnectorGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	result, err := s.Platform.ConnectorGet(name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleConnectorCreate(w http.ResponseWriter, r *http.Request) {
	var req api.ConnectorCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.Platform.ConnectorCreate(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleConnectorUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req api.ConnectorUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.Platform.ConnectorUpdate(r.Context(), name, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleConnectorDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.Platform.ConnectorDelete(r.Context(), name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// ── History ─────────────────────────────────────────────────────────────────

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	n := 20
	if nStr := r.URL.Query().Get("n"); nStr != "" {
		if parsed, err := strconv.Atoi(nStr); err == nil && parsed > 0 {
			n = parsed
		}
	}
	result, err := s.Platform.History(n)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, result)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

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
