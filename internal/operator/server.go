// Package operator implements the HTTP API and MCP server for the angee operator.
// All business logic lives in internal/service — this package is a thin adapter.
package operator

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/fyltr/angee/api"
	"github.com/fyltr/angee/internal/service"
)

// Server is the angee operator HTTP server.
// It delegates all business logic to Platform.
type Server struct {
	Platform *service.Platform
	Log      *slog.Logger
}

// New creates an operator Server for the given ANGEE_ROOT path.
func New(angeeRoot string, logger *slog.Logger) (*Server, error) {
	plat, err := service.NewPlatform(angeeRoot, logger)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = plat.Log
	}
	return &Server{Platform: plat, Log: logger}, nil
}

// Handler returns the HTTP handler for the operator API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /health", s.handleHealth)

	// Config
	mux.HandleFunc("GET /config", s.handleConfigGet)
	mux.HandleFunc("POST /config", s.handleConfigSet)

	// Deployment lifecycle
	mux.HandleFunc("POST /deploy", s.handleDeploy)
	mux.HandleFunc("POST /rollback", s.handleRollback)
	mux.HandleFunc("GET /plan", s.handlePlan)

	// Runtime status
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /logs/{service}", s.handleLogs)
	mux.HandleFunc("GET /logs", s.handleLogs)
	mux.HandleFunc("POST /scale/{service}", s.handleScale)
	mux.HandleFunc("POST /down", s.handleDown)

	// Agent operations
	mux.HandleFunc("GET /agents", s.handleAgentList)
	mux.HandleFunc("POST /agents/{name}/start", s.handleAgentStart)
	mux.HandleFunc("POST /agents/{name}/stop", s.handleAgentStop)
	mux.HandleFunc("GET /agents/{name}/logs", s.handleAgentLogs)

	// Connectors
	mux.HandleFunc("GET /connectors", s.handleConnectorList)
	mux.HandleFunc("POST /connectors", s.handleConnectorCreate)
	mux.HandleFunc("GET /connectors/{name}", s.handleConnectorGet)
	mux.HandleFunc("PATCH /connectors/{name}", s.handleConnectorUpdate)
	mux.HandleFunc("DELETE /connectors/{name}", s.handleConnectorDelete)

	// Git history
	mux.HandleFunc("GET /history", s.handleHistory)

	// MCP endpoint
	mux.HandleFunc("POST /mcp", s.handleMCP)

	// OpenAPI
	mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)

	apiKey := s.Platform.APIKey()
	origins := s.Platform.Cfg.CORSOrigins

	return recoveryMiddleware(
		corsMiddleware(
			authMiddleware(
				loggingMiddleware(mux, s.Log),
				apiKey, s.Log,
			),
			origins,
		),
		s.Log,
	)
}

// Start runs the operator HTTP server.
func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.Handler()}
	s.Log.Info("operator started", "addr", addr, "root", s.Platform.Root.Path, "runtime", s.Platform.Cfg.Runtime)
	s.Platform.RestartHealthProbes(ctx)

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ── Response helpers ────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, err error) {
	code := service.ErrorStatus(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(api.ErrorResponse{Error: err.Error()}) //nolint:errcheck
}

// ── Middleware ───────────────────────────────────────────────────────────────

func authMiddleware(next http.Handler, apiKey string, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey == "" || r.URL.Path == "/health" || r.URL.Path == "/openapi.json" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != apiKey {
			log.Warn("unauthorized request", "path", r.URL.Path, "remote", r.RemoteAddr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(api.ErrorResponse{Error: "unauthorized"}) //nolint:errcheck
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoveryMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered", "error", rec, "stack", string(debug.Stack()))
				writeError(w, &service.ServiceError{Status: 500, Message: "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler, origins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && matchOrigin(origin, origins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func matchOrigin(origin string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(origin, strings.TrimSuffix(p, "*")) {
				return true
			}
		} else if origin == p {
			return true
		}
	}
	return false
}

func loggingMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
