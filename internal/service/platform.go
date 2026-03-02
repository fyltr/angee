package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/fyltr/angee/api"
	"github.com/fyltr/angee/internal/compiler"
	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/git"
	"github.com/fyltr/angee/internal/root"
	"github.com/fyltr/angee/internal/runtime"
	composeruntime "github.com/fyltr/angee/internal/runtime/compose"
)

// Platform is the business logic core. Both HTTP handlers and MCP tools
// call methods on this struct. It owns all dependencies and never knows
// about HTTP or JSON-RPC.
type Platform struct {
	Root     *root.Root
	Cfg      *config.OperatorConfig
	Backend  runtime.RuntimeBackend
	Compiler *compiler.Compiler
	Git      *git.Repo
	Health   *HealthChecker
	Log      *slog.Logger

	// healthCancel stops current health-probe goroutines for restart.
	healthCancel context.CancelFunc
}

// NewPlatform creates a Platform for the given ANGEE_ROOT path.
func NewPlatform(angeeRoot string, logger *slog.Logger) (*Platform, error) {
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

	comp := compiler.New(angeeRoot, cfg.Docker.Network)

	p := &Platform{
		Root:     r,
		Cfg:      cfg,
		Git:      git.New(angeeRoot),
		Log:      logger,
		Compiler: comp,
		Health:   newHealthChecker(logger),
	}

	comp.APIKey = p.resolveAPIKey()

	angeeCfg, err := r.LoadAngeeConfig()
	if err != nil {
		return nil, fmt.Errorf("loading angee.yaml: %w", err)
	}
	projectName := angeeCfg.Name
	if projectName == "" {
		projectName = "angee"
	}

	switch cfg.Runtime {
	case "kubernetes":
		return nil, fmt.Errorf("kubernetes backend not yet implemented")
	default:
		p.Backend = composeruntime.New(angeeRoot, projectName)
	}

	return p, nil
}

// resolveAPIKey returns the API key from environment or config.
func (p *Platform) resolveAPIKey() string {
	if key := os.Getenv("ANGEE_API_KEY"); key != "" {
		return key
	}
	return p.Cfg.APIKey
}

// APIKey returns the resolved API key.
func (p *Platform) APIKey() string {
	return p.resolveAPIKey()
}

// ── Platform operations ─────────────────────────────────────────────────────

// HealthCheck returns operator liveness.
func (p *Platform) HealthCheck() *api.HealthResponse {
	return &api.HealthResponse{Status: "ok", Root: p.Root.Path, Runtime: p.Cfg.Runtime}
}

// Deploy compiles angee.yaml and applies it to the runtime.
func (p *Platform) Deploy(ctx context.Context, message string) (*api.ApplyResult, error) {
	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := p.prepareAndCompile(cfg); err != nil {
		return nil, fmt.Errorf("preparing: %w", err)
	}

	result, err := p.Backend.Apply(ctx, p.Root.ComposePath())
	if err != nil {
		return nil, fmt.Errorf("applying: %w", err)
	}

	p.Log.Info("deployed",
		"started", len(result.ServicesStarted),
		"updated", len(result.ServicesUpdated),
		"removed", len(result.ServicesRemoved),
	)
	p.RestartHealthProbes(ctx)

	return toAPIResult(result), nil
}

// Plan shows what deploy would change without applying.
func (p *Platform) Plan(ctx context.Context) (*api.ChangeSet, error) {
	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	if err := p.compileAndWrite(cfg); err != nil {
		return nil, err
	}
	cs, err := p.Backend.Diff(ctx, p.Root.ComposePath())
	if err != nil {
		return nil, err
	}
	return &api.ChangeSet{Add: cs.Add, Update: cs.Update, Remove: cs.Remove}, nil
}

// Rollback reverts to a previous git commit and redeploys.
func (p *Platform) Rollback(ctx context.Context, sha string) (*api.RollbackResponse, error) {
	if sha == "" {
		return nil, BadRequest("sha is required")
	}
	if err := p.Git.Revert(sha); err != nil {
		if err2 := p.Git.ResetHard(sha); err2 != nil {
			return nil, fmt.Errorf("rollback failed: %w", err)
		}
	}
	result, err := p.Deploy(ctx, "")
	if err != nil {
		return nil, err
	}
	newSHA, _ := p.Git.CurrentSHA()
	return &api.RollbackResponse{RolledBackTo: newSHA, Deploy: result}, nil
}

// Status returns the current runtime state of all services and agents.
func (p *Platform) Status(ctx context.Context) ([]*runtime.ServiceStatus, error) {
	statuses, err := p.Backend.Status(ctx)
	if err != nil {
		return nil, err
	}
	if statuses == nil {
		statuses = []*runtime.ServiceStatus{}
	}
	// Overlay operator-side health probe results.
	for _, st := range statuses {
		if h := p.Health.Status(st.Name); h != "" {
			st.Health = h
		}
	}
	return statuses, nil
}

// Scale adjusts the replica count for a service.
func (p *Platform) Scale(ctx context.Context, service string, replicas int) (*api.ScaleResponse, error) {
	if service == "" {
		return nil, BadRequest("service is required")
	}
	if err := p.Backend.Scale(ctx, service, replicas); err != nil {
		return nil, err
	}
	return &api.ScaleResponse{Service: service, Replicas: replicas}, nil
}

// Down brings the entire stack down.
func (p *Platform) Down(ctx context.Context) error {
	return p.Backend.Down(ctx)
}

// ── Private helpers ─────────────────────────────────────────────────────────

func (p *Platform) loadConfig() (*config.AngeeConfig, error) {
	return p.Root.LoadAngeeConfig()
}

func (p *Platform) prepareAndCompile(cfg *config.AngeeConfig) error {
	for agentName := range cfg.Agents {
		if err := p.Root.EnsureAgentDir(agentName); err != nil {
			return fmt.Errorf("agent dir for %s: %w", agentName, err)
		}
	}
	for agentName, agent := range cfg.Agents {
		if err := compiler.RenderAgentFiles(p.Root.Path, p.Root.AgentDir(agentName), agent, cfg.MCPServers); err != nil {
			return fmt.Errorf("agent files for %s: %w", agentName, err)
		}
	}
	return p.compileAndWrite(cfg)
}

func (p *Platform) compileAndWrite(cfg *config.AngeeConfig) error {
	cf, err := p.Compiler.Compile(cfg)
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}
	return compiler.Write(cf, p.Root.ComposePath())
}

func (p *Platform) writeAndCommit(cfg *config.AngeeConfig, message string) error {
	if err := config.Write(cfg, p.Root.AngeeYAMLPath()); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return BadRequest(err.Error())
	}
	_, err := p.Root.CommitConfig(message)
	return err
}

// RestartHealthProbes (re)starts health-check goroutines for the current config.
func (p *Platform) RestartHealthProbes(parentCtx context.Context) {
	if p.healthCancel != nil {
		p.healthCancel()
	}
	cfg, err := p.Root.LoadAngeeConfig()
	if err != nil {
		p.Log.Warn("skipping health probes: cannot load angee.yaml", "error", err)
		return
	}
	probes := p.Health.Reload(cfg)
	if len(probes) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(parentCtx)
	p.healthCancel = cancel
	p.Health.Run(ctx, probes)
	p.Log.Info("health probes started", "count", len(probes))
}

// agentServiceName returns the docker-compose service name for an agent.
func agentServiceName(name string) string {
	return compiler.AgentServicePrefix + name
}

// buildStatusMap returns a map of service name → runtime status.
func (p *Platform) buildStatusMap(ctx context.Context) map[string]*runtime.ServiceStatus {
	statuses, _ := p.Backend.Status(ctx)
	m := make(map[string]*runtime.ServiceStatus, len(statuses))
	for _, st := range statuses {
		m[st.Name] = st
	}
	return m
}

// toAPIResult converts a runtime.ApplyResult to an api.ApplyResult.
func toAPIResult(r *runtime.ApplyResult) *api.ApplyResult {
	if r == nil {
		return nil
	}
	return &api.ApplyResult{
		ServicesStarted: r.ServicesStarted,
		ServicesUpdated: r.ServicesUpdated,
		ServicesRemoved: r.ServicesRemoved,
	}
}
