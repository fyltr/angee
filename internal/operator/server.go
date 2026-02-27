// Package operator implements the HTTP API and MCP server for the angee operator.
package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/fyltr/angee-go/internal/compiler"
	"github.com/fyltr/angee-go/internal/config"
	"github.com/fyltr/angee-go/internal/git"
	"github.com/fyltr/angee-go/internal/root"
	"github.com/fyltr/angee-go/internal/runtime"
	composeruntime "github.com/fyltr/angee-go/internal/runtime/compose"
)

// Server is the angee operator: it owns ANGEE_ROOT and manages the runtime.
type Server struct {
	Root     *root.Root
	Cfg      *config.OperatorConfig
	Backend  runtime.RuntimeBackend
	Compiler *compiler.Compiler
	Git      *git.Repo
	Log      *slog.Logger
}

// New creates an operator Server for the given ANGEE_ROOT path.
func New(angeeRoot string, logger *slog.Logger) (*Server, error) {
	r, err := root.Open(angeeRoot)
	if err != nil {
		return nil, err
	}

	cfg, err := r.LoadOperatorConfig()
	if err != nil {
		return nil, fmt.Errorf("loading operator config: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		Root:     r,
		Cfg:      cfg,
		Git:      git.New(angeeRoot),
		Log:      logger,
		Compiler: compiler.New(angeeRoot, cfg.Docker.Network),
	}

	// Load angee.yaml to get the project name for the compose backend
	angeeCfg, err := r.LoadAngeeConfig()
	if err != nil {
		return nil, fmt.Errorf("loading angee.yaml: %w", err)
	}
	projectName := angeeCfg.Name
	if projectName == "" {
		projectName = "angee"
	}

	// Select runtime backend
	switch cfg.Runtime {
	case "kubernetes":
		return nil, fmt.Errorf("kubernetes backend not yet implemented")
	default:
		s.Backend = composeruntime.New(angeeRoot, projectName)
	}

	return s, nil
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
	mux.HandleFunc("POST /agents/{name}/start", s.handleAgentStart)
	mux.HandleFunc("POST /agents/{name}/stop", s.handleAgentStop)
	mux.HandleFunc("GET /agents/{name}/logs", s.handleAgentLogs)
	mux.HandleFunc("GET /agents", s.handleAgentList)

	// Git history
	mux.HandleFunc("GET /history", s.handleHistory)

	apiKey := s.resolveAPIKey()
	origins := s.Cfg.CORSOrigins

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
	srv := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}

	s.Log.Info("operator started", "addr", addr, "root", s.Root.Path, "runtime", s.Cfg.Runtime)

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// compileAndWrite compiles angee.yaml → docker-compose.yaml.
func (s *Server) compileAndWrite(cfg *config.AngeeConfig) error {
	cf, err := s.Compiler.Compile(cfg)
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}
	return compiler.Write(cf, s.Root.ComposePath())
}

// resolveAPIKey returns the API key from environment or config.
// Environment variable ANGEE_API_KEY takes precedence.
func (s *Server) resolveAPIKey() string {
	if key := os.Getenv("ANGEE_API_KEY"); key != "" {
		return key
	}
	return s.Cfg.APIKey
}

// jsonOK writes a JSON 200 response.
func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// jsonErr writes a JSON error response.
func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

// ── Middleware ────────────────────────────────────────────────────────────────

// authMiddleware checks for a valid Bearer token when an API key is configured.
// The /health endpoint always bypasses auth.
func authMiddleware(next http.Handler, apiKey string, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey == "" || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != apiKey {
			log.Warn("unauthorized request", "path", r.URL.Path, "remote", r.RemoteAddr)
			jsonErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// recoveryMiddleware catches panics, logs the stack trace, and returns 500.
func recoveryMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered", "error", rec, "stack", string(debug.Stack()))
				jsonErr(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware sets CORS headers. It supports trailing-* wildcards in origin
// patterns (e.g. "http://localhost:*" matches any port).
func corsMiddleware(next http.Handler, origins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && matchOrigin(origin, origins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// matchOrigin checks if origin matches any of the allowed patterns.
// A pattern ending in "*" acts as a prefix match.
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
