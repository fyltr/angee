package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fyltr/angee/api"
	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/copier"
	"github.com/fyltr/angee/internal/provision"
	"github.com/fyltr/angee/internal/root"
	"github.com/fyltr/angee/internal/state"
)

func (p *Platform) StackInit(ctx context.Context, req api.StackInitRequest) (*api.ProvisionResponse, error) {
	if req.Name == "" {
		return nil, BadRequest("stack name is required")
	}
	if !req.Yes {
		return nil, BadRequest("non-interactive stack init requires --yes")
	}
	if req.Root != "" && !samePath(req.Root, p.Root.Path) {
		return nil, BadRequest(fmt.Sprintf("operator is serving ANGEE_ROOT %s, not %s", p.Root.Path, req.Root))
	}

	worktree := req.Path
	if worktree == "" {
		worktree = filepath.Dir(p.Root.Path)
	}
	worktree = expandPath(worktree)
	if err := os.MkdirAll(worktree, 0755); err != nil {
		return nil, fmt.Errorf("creating stack worktree: %w", err)
	}

	manifestPath := filepath.Join(p.Root.Path, root.AngeeYAML)
	if !req.Force {
		if _, err := os.Stat(manifestPath); err == nil {
			return nil, Conflict(fmt.Sprintf("ANGEE_ROOT already has %s", root.AngeeYAML))
		}
	}

	tmpl, err := copier.Resolve(req.Template, "stack", req.Name)
	if err != nil {
		return nil, err
	}
	r, err := root.Initialize(p.Root.Path)
	if err != nil {
		return nil, err
	}
	p.Root = r
	p.Git = r.Git
	if err := r.WriteGitignore(); err != nil {
		return nil, err
	}
	p.Cfg.TemplateSource = templateSource(req.Template, tmpl)
	if err := r.WriteOperatorConfig(p.Cfg); err != nil {
		return nil, err
	}

	data := map[string]string{}
	for k, v := range req.Set {
		data[k] = v
	}
	if data["project_name"] == "" {
		data["project_name"] = deriveStackName(worktree)
	}
	if err := copier.Copy(ctx, tmpl, worktree, data, req.Force); err != nil {
		return nil, err
	}

	cfg, err := config.Load(manifestPath)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}

	store := state.New(p.Root.Path)
	if _, err := provision.ResolvePortLeases(store, cfg.PortLeases, req.Ports, "stack:"+req.Name); err != nil {
		return nil, err
	}
	secrets, err := provision.ResolveSecrets(store, cfg.Secrets, req.Secrets)
	if err != nil {
		return nil, err
	}
	if err := r.WriteEnvFile(formatEnv(secrets)); err != nil {
		return nil, err
	}

	if err := r.InitialCommit(); err != nil {
		p.Log.Warn("initial commit skipped", "err", err)
	}

	return &api.ProvisionResponse{
		Status:   "ok",
		Message:  fmt.Sprintf("Initialized stack %s", req.Name),
		Root:     p.Root.Path,
		Manifest: manifestPath,
		Changed:  []string{manifestPath, filepath.Join(p.Root.Path, state.DirName)},
	}, nil
}

func (p *Platform) StackUpdate(ctx context.Context, req api.StackUpdateRequest) (*api.ProvisionResponse, error) {
	if !req.Yes {
		return nil, BadRequest("non-interactive stack update requires --yes")
	}
	if req.Root != "" && !samePath(req.Root, p.Root.Path) {
		return nil, BadRequest(fmt.Sprintf("operator is serving ANGEE_ROOT %s, not %s", p.Root.Path, req.Root))
	}
	if _, err := os.Stat(p.Root.AngeeYAMLPath()); err != nil {
		return nil, fmt.Errorf("stack is not initialized: %w", err)
	}
	worktree := filepath.Dir(p.Root.Path)
	data := map[string]string{}
	for k, v := range req.Set {
		data[k] = v
	}
	if err := copier.Update(ctx, worktree, data); err != nil {
		return nil, err
	}
	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	store := state.New(p.Root.Path)
	if _, err := provision.ResolvePortLeases(store, cfg.PortLeases, req.Ports, "stack:update"); err != nil {
		return nil, err
	}
	secrets, err := provision.ResolveSecrets(store, cfg.Secrets, req.Secrets)
	if err != nil {
		return nil, err
	}
	if err := p.Root.WriteEnvFile(formatEnv(secrets)); err != nil {
		return nil, err
	}
	if _, err := p.Root.CommitConfig("angee: update stack template"); err != nil {
		p.Log.Warn("stack update commit skipped", "err", err)
	}
	return &api.ProvisionResponse{
		Status:   "ok",
		Message:  "Updated stack",
		Root:     p.Root.Path,
		Manifest: p.Root.AngeeYAMLPath(),
		Changed:  []string{p.Root.AngeeYAMLPath(), filepath.Join(p.Root.Path, state.DirName)},
	}, nil
}

func (p *Platform) WorkspaceInit(ctx context.Context, req api.WorkspaceInitRequest) (*api.ProvisionResponse, error) {
	if err := validateResourceName("workspace", req.Name); err != nil {
		return nil, err
	}
	if !req.Yes {
		return nil, BadRequest("non-interactive workspace init requires --yes")
	}
	if err := p.requireRequestRoot(req.Root); err != nil {
		return nil, err
	}

	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	stackCfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	if err := stackCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	templateRef, err := workspaceTemplateRef(stackCfg, req.Template)
	if err != nil {
		return nil, err
	}
	tmpl, err := copier.Resolve(templateRef, "workspace", "")
	if err != nil {
		return nil, err
	}

	workspaceDir := p.workspaceDir(req.Name)
	if _, err := os.Stat(workspaceDir); err == nil {
		return nil, Conflict(fmt.Sprintf("workspace %q already exists", req.Name))
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(workspaceDir), 0755); err != nil {
		return nil, fmt.Errorf("creating workspaces directory: %w", err)
	}

	data := map[string]string{
		"workspace_name": req.Name,
		"source_ref":     workspaceSourceRef(req.Name, req.Branch),
	}
	if err := copier.Copy(ctx, tmpl, workspaceDir, data, false); err != nil {
		return nil, err
	}

	workspaceCfg, manifestPath, err := loadWorkspaceConfig(workspaceDir)
	if err != nil {
		return nil, err
	}
	if err := applyWorkspaceSourceRefs(workspaceCfg, req.Name, req.Branch, req.Overrides); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := workspaceCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := config.Write(workspaceCfg, manifestPath); err != nil {
		return nil, err
	}
	if err := p.resolveWorkspaceState(req.Name, workspaceCfg, req.Ports, req.Secrets); err != nil {
		return nil, err
	}

	registerWorkspace(stackCfg, req.Name, templateSource(templateRef, tmpl), req.Branch, workspaceCfg)
	if err := stackCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := config.Write(stackCfg, p.Root.AngeeYAMLPath()); err != nil {
		return nil, err
	}
	if err := p.commitProvision("angee: initialize workspace "+req.Name, root.AngeeYAML, filepath.Join(root.WorkspacesDir, req.Name)); err != nil {
		p.Log.Warn("workspace init commit skipped", "err", err)
	}

	return &api.ProvisionResponse{
		Status:   "ok",
		Message:  fmt.Sprintf("Initialized workspace %s", req.Name),
		Root:     p.Root.Path,
		Manifest: manifestPath,
		Changed:  []string{manifestPath, p.Root.AngeeYAMLPath()},
	}, nil
}

func (p *Platform) WorkspaceUpdate(ctx context.Context, req api.WorkspaceUpdateRequest) (*api.ProvisionResponse, error) {
	if err := validateResourceName("workspace", req.Name); err != nil {
		return nil, err
	}
	if !req.Yes {
		return nil, BadRequest("non-interactive workspace update requires --yes")
	}
	if err := p.requireRequestRoot(req.Root); err != nil {
		return nil, err
	}

	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	stackCfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	spec, err := workspaceSpec(stackCfg, req.Name)
	if err != nil {
		return nil, err
	}
	workspaceDir := p.workspaceDirFromSpec(req.Name, spec)
	if _, err := os.Stat(workspaceDir); err != nil {
		if os.IsNotExist(err) {
			return nil, NotFound(fmt.Sprintf("workspace %q is not initialized", req.Name))
		}
		return nil, err
	}
	if err := copier.Update(ctx, workspaceDir, nil, req.Ref); err != nil {
		return nil, err
	}

	workspaceCfg, manifestPath, err := loadWorkspaceConfig(workspaceDir)
	if err != nil {
		return nil, err
	}
	if err := applyWorkspaceSourceRefs(workspaceCfg, req.Name, "", req.Overrides); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := workspaceCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := config.Write(workspaceCfg, manifestPath); err != nil {
		return nil, err
	}
	if err := p.resolveWorkspaceState(req.Name, workspaceCfg, req.Ports, req.Secrets); err != nil {
		return nil, err
	}

	registerWorkspace(stackCfg, req.Name, spec.Template, spec.Branch, workspaceCfg)
	if err := stackCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := config.Write(stackCfg, p.Root.AngeeYAMLPath()); err != nil {
		return nil, err
	}
	if err := p.commitProvision("angee: update workspace "+req.Name, root.AngeeYAML, filepath.Join(root.WorkspacesDir, req.Name)); err != nil {
		p.Log.Warn("workspace update commit skipped", "err", err)
	}

	return &api.ProvisionResponse{
		Status:   "ok",
		Message:  fmt.Sprintf("Updated workspace %s", req.Name),
		Root:     p.Root.Path,
		Manifest: manifestPath,
		Changed:  []string{manifestPath, p.Root.AngeeYAMLPath()},
	}, nil
}

func (p *Platform) WorkspaceList(ctx context.Context, req api.WorkspaceListRequest) (*api.ProvisionResponse, error) {
	if err := p.requireRequestRoot(req.Root); err != nil {
		return nil, err
	}
	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	names := workspaceNames(cfg)
	message := "No workspaces"
	if len(names) > 0 {
		message = "Workspaces: " + strings.Join(names, ", ")
	}
	return &api.ProvisionResponse{Status: "ok", Message: message, Root: p.Root.Path, Changed: names}, nil
}

func (p *Platform) WorkspaceDev(ctx context.Context, req api.WorkspaceDevRequest) (*api.ProvisionResponse, error) {
	return nil, NotImplemented("workspace dev reconciliation is not implemented yet")
}

func (p *Platform) AgentInit(ctx context.Context, req api.AgentInitRequest) (*api.ProvisionResponse, error) {
	if err := validateResourceName("agent", req.Name); err != nil {
		return nil, err
	}
	if !req.Yes {
		return nil, BadRequest("non-interactive agent init requires --yes")
	}
	if err := p.requireRequestRoot(req.Root); err != nil {
		return nil, err
	}

	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	stackCfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	if err := stackCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	templateRef, err := agentTemplateRef(stackCfg, req.Template)
	if err != nil {
		return nil, err
	}
	tmpl, err := copier.Resolve(templateRef, "agent", "")
	if err != nil {
		return nil, err
	}

	agentDir := p.agentDir(req.Name)
	if _, err := os.Stat(agentDir); err == nil {
		return nil, Conflict(fmt.Sprintf("agent %q already exists", req.Name))
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(agentDir), 0755); err != nil {
		return nil, fmt.Errorf("creating agents directory: %w", err)
	}

	data := map[string]string{
		"agent_name": req.Name,
		"source_ref": workspaceSourceRef(req.Name, req.Branch),
	}
	if err := copier.Copy(ctx, tmpl, agentDir, data, false); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755); err != nil {
		return nil, err
	}

	agentCfg, manifestPath, err := loadAgentConfig(agentDir)
	if err != nil {
		return nil, err
	}
	if err := applyWorkspaceSourceRefs(agentCfg, req.Name, req.Branch, req.Overrides); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := agentCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := config.Write(agentCfg, manifestPath); err != nil {
		return nil, err
	}

	if err := registerAgent(stackCfg, req.Name, templateSource(templateRef, tmpl), agentCfg); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := stackCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := p.resolveStackState(stackCfg, req.Ports, req.Secrets); err != nil {
		return nil, err
	}
	if err := p.writeAgentEnv(req.Name, agentCfg); err != nil {
		return nil, err
	}
	if err := config.Write(stackCfg, p.Root.AngeeYAMLPath()); err != nil {
		return nil, err
	}
	if err := p.commitProvision("angee: initialize agent "+req.Name, root.AngeeYAML, filepath.Join(root.AgentsDir, req.Name)); err != nil {
		p.Log.Warn("agent init commit skipped", "err", err)
	}

	return &api.ProvisionResponse{
		Status:   "ok",
		Message:  fmt.Sprintf("Initialized agent %s", req.Name),
		Root:     p.Root.Path,
		Manifest: manifestPath,
		Changed:  []string{manifestPath, p.Root.AngeeYAMLPath()},
	}, nil
}

func (p *Platform) AgentUpdate(ctx context.Context, req api.AgentUpdateRequest) (*api.ProvisionResponse, error) {
	if err := validateResourceName("agent", req.Name); err != nil {
		return nil, err
	}
	if !req.Yes {
		return nil, BadRequest("non-interactive agent update requires --yes")
	}
	if err := p.requireRequestRoot(req.Root); err != nil {
		return nil, err
	}

	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	stackCfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	spec, err := agentSpec(stackCfg, req.Name)
	if err != nil {
		return nil, err
	}
	agentDir := p.agentDir(req.Name)
	if _, err := os.Stat(agentDir); err != nil {
		if os.IsNotExist(err) {
			return nil, NotFound(fmt.Sprintf("agent %q is not initialized", req.Name))
		}
		return nil, err
	}
	if req.Template != "" && req.Template != spec.Template {
		return nil, BadRequest("agent update --template switching is not implemented; update the recorded template first")
	}
	if err := copier.Update(ctx, agentDir, nil, req.Ref); err != nil {
		return nil, err
	}

	agentCfg, manifestPath, err := loadAgentConfig(agentDir)
	if err != nil {
		return nil, err
	}
	if err := applyWorkspaceSourceRefs(agentCfg, req.Name, "", req.Overrides); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := agentCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := config.Write(agentCfg, manifestPath); err != nil {
		return nil, err
	}
	if err := registerAgent(stackCfg, req.Name, spec.Template, agentCfg); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := stackCfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	if err := p.resolveStackState(stackCfg, req.Ports, req.Secrets); err != nil {
		return nil, err
	}
	if err := p.writeAgentEnv(req.Name, agentCfg); err != nil {
		return nil, err
	}
	if err := config.Write(stackCfg, p.Root.AngeeYAMLPath()); err != nil {
		return nil, err
	}
	if err := p.commitProvision("angee: update agent "+req.Name, root.AngeeYAML, filepath.Join(root.AgentsDir, req.Name)); err != nil {
		p.Log.Warn("agent update commit skipped", "err", err)
	}

	return &api.ProvisionResponse{
		Status:   "ok",
		Message:  fmt.Sprintf("Updated agent %s", req.Name),
		Root:     p.Root.Path,
		Manifest: manifestPath,
		Changed:  []string{manifestPath, p.Root.AngeeYAMLPath()},
	}, nil
}

func (p *Platform) AgentRestart(ctx context.Context, req api.AgentActionRequest) (*api.ProvisionResponse, error) {
	if err := validateResourceName("agent", req.Name); err != nil {
		return nil, err
	}
	if err := p.requireRequestRoot(req.Root); err != nil {
		return nil, err
	}
	if err := p.AgentStop(ctx, req.Name); err != nil {
		return nil, err
	}
	if _, err := p.AgentStart(ctx, req.Name); err != nil {
		return nil, err
	}
	return &api.ProvisionResponse{Status: "ok", Message: fmt.Sprintf("Restarted agent %s", req.Name), Root: p.Root.Path}, nil
}

func (p *Platform) AgentDestroy(ctx context.Context, req api.AgentActionRequest) (*api.ProvisionResponse, error) {
	return nil, NotImplemented("agent destroy is not implemented yet")
}

func (p *Platform) AgentChat(ctx context.Context, req api.AgentChatRequest) (*api.ProvisionResponse, error) {
	return nil, NotImplemented("agent chat is not implemented yet")
}

func (p *Platform) AgentAsk(ctx context.Context, req api.AgentAskRequest) (*api.ProvisionResponse, error) {
	return nil, NotImplemented("agent ask is not implemented yet")
}

func (p *Platform) Reconcile(ctx context.Context, req api.ReconcileRequest) (*api.ProvisionResponse, error) {
	if req.Root != "" && !samePath(req.Root, p.Root.Path) {
		return nil, BadRequest(fmt.Sprintf("operator is serving ANGEE_ROOT %s, not %s", p.Root.Path, req.Root))
	}
	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}
	store := state.New(p.Root.Path)
	if _, err := provision.ResolvePortLeases(store, cfg.PortLeases, nil, "reconcile:"+req.Mode); err != nil {
		return nil, err
	}
	secrets, err := provision.ResolveSecrets(store, cfg.Secrets, nil)
	if err != nil {
		return nil, err
	}
	if err := p.Root.WriteEnvFile(formatEnv(secrets)); err != nil {
		return nil, err
	}
	result, err := p.Deploy(ctx, "")
	if err != nil {
		return nil, err
	}
	changed := append([]string{}, result.ServicesStarted...)
	changed = append(changed, result.ServicesUpdated...)
	changed = append(changed, result.ServicesRemoved...)
	return &api.ProvisionResponse{
		Status:   "ok",
		Message:  "Reconciled stack",
		Root:     p.Root.Path,
		Manifest: p.Root.AngeeYAMLPath(),
		Changed:  changed,
	}, nil
}

func (p *Platform) requireRequestRoot(requestRoot string) error {
	if requestRoot != "" && !samePath(requestRoot, p.Root.Path) {
		return BadRequest(fmt.Sprintf("operator is serving ANGEE_ROOT %s, not %s", p.Root.Path, requestRoot))
	}
	return nil
}

func validateResourceName(kind, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return BadRequest(kind + " name is required")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return BadRequest(fmt.Sprintf("%s name %q must not contain path separators", kind, name))
	}
	return nil
}

func workspaceTemplateRef(cfg *config.AngeeConfig, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	if cfg.Workspaces != nil && cfg.Workspaces.DefaultTemplate != "" {
		return cfg.Workspaces.DefaultTemplate, nil
	}
	return "", BadRequest("workspace template is required or workspaces.default_template must be set")
}

func agentTemplateRef(cfg *config.AngeeConfig, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	if cfg.Agents != nil && cfg.Agents.DefaultTemplate != "" {
		return cfg.Agents.DefaultTemplate, nil
	}
	return "", BadRequest("agent template is required or agents.default_template must be set")
}

func (p *Platform) workspaceDir(name string) string {
	return filepath.Join(p.Root.Path, root.WorkspacesDir, name)
}

func (p *Platform) agentDir(name string) string {
	return filepath.Join(p.Root.Path, root.AgentsDir, name)
}

func (p *Platform) workspaceDirFromSpec(name string, spec config.WorkspaceSpec) string {
	if spec.Path == "" {
		return p.workspaceDir(name)
	}
	if filepath.IsAbs(spec.Path) {
		return spec.Path
	}
	return filepath.Join(p.Root.Path, filepath.FromSlash(spec.Path))
}

func loadWorkspaceConfig(workspaceDir string) (*config.AngeeConfig, string, error) {
	for _, name := range []string{"workspace.yaml", root.AngeeYAML} {
		path := filepath.Join(workspaceDir, name)
		if _, err := os.Stat(path); err == nil {
			cfg, err := config.Load(path)
			return cfg, path, err
		} else if !os.IsNotExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("workspace manifest not found in %s", workspaceDir)
}

func loadAgentConfig(agentDir string) (*config.AngeeConfig, string, error) {
	for _, name := range []string{"agent.yaml", root.AngeeYAML} {
		path := filepath.Join(agentDir, name)
		if _, err := os.Stat(path); err == nil {
			cfg, err := config.Load(path)
			return cfg, path, err
		} else if !os.IsNotExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("agent manifest not found in %s", agentDir)
}

func workspaceSourceRef(workspaceName, branch string) string {
	if branch != "" {
		return branch
	}
	return workspaceName
}

func applyWorkspaceSourceRefs(cfg *config.AngeeConfig, workspaceName, branch string, overrides map[string]string) error {
	if len(overrides) > 0 && len(cfg.Sources) == 0 {
		return fmt.Errorf("workspace has no sources to override")
	}
	defaultRef := workspaceSourceRef(workspaceName, branch)
	for name, source := range cfg.Sources {
		if source.Ref == "" || source.Ref == "same-name" {
			source.Ref = defaultRef
			cfg.Sources[name] = source
		}
	}
	for name, ref := range overrides {
		source, ok := cfg.Sources[name]
		if !ok {
			return fmt.Errorf("source override %q does not match a declared source", name)
		}
		source.Ref = ref
		cfg.Sources[name] = source
	}
	return nil
}

func registerWorkspace(stackCfg *config.AngeeConfig, name, templateRef, branch string, workspaceCfg *config.AngeeConfig) {
	if stackCfg.Workspaces == nil {
		stackCfg.Workspaces = &config.WorkspaceRegistrySpec{}
	}
	if stackCfg.Workspaces.Prefix == "" {
		stackCfg.Workspaces.Prefix = root.WorkspacesDir
	}
	if stackCfg.Workspaces.Items == nil {
		stackCfg.Workspaces.Items = map[string]config.WorkspaceSpec{}
	}
	stackCfg.Workspaces.Items[name] = config.WorkspaceSpec{
		Template: templateRef,
		Branch:   branch,
		Path:     filepath.ToSlash(filepath.Join(root.WorkspacesDir, name)),
		Sources:  workspaceSourceRefs(workspaceCfg),
	}
}

func workspaceSpec(cfg *config.AngeeConfig, name string) (config.WorkspaceSpec, error) {
	if cfg.Workspaces == nil || cfg.Workspaces.Items == nil {
		return config.WorkspaceSpec{}, NotFound(fmt.Sprintf("workspace %q is not registered", name))
	}
	spec, ok := cfg.Workspaces.Items[name]
	if !ok {
		return config.WorkspaceSpec{}, NotFound(fmt.Sprintf("workspace %q is not registered", name))
	}
	return spec, nil
}

func agentSpec(cfg *config.AngeeConfig, name string) (config.AgentSpec, error) {
	if cfg.Agents == nil || cfg.Agents.Items == nil {
		return config.AgentSpec{}, NotFound(fmt.Sprintf("agent %q is not registered", name))
	}
	spec, ok := cfg.Agents.Items[name]
	if !ok {
		return config.AgentSpec{}, NotFound(fmt.Sprintf("agent %q is not registered", name))
	}
	return spec, nil
}

func workspaceNames(cfg *config.AngeeConfig) []string {
	if cfg.Workspaces == nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Workspaces.Items))
	for name := range cfg.Workspaces.Items {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func workspaceSourceRefs(cfg *config.AngeeConfig) map[string]string {
	if len(cfg.Sources) == 0 {
		return nil
	}
	out := make(map[string]string, len(cfg.Sources))
	for name, source := range cfg.Sources {
		if source.Ref != "" {
			out[name] = source.Ref
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p *Platform) resolveWorkspaceState(name string, cfg *config.AngeeConfig, ports map[string]int, secrets map[string]string) error {
	scope := workspaceScope(name)
	store := state.New(p.Root.Path)
	if _, err := provision.ResolvePortLeases(store, scopePortLeases(scope, cfg.PortLeases), scopeIntMap(scope, ports), "workspace:"+name); err != nil {
		return err
	}
	resolvedSecrets, err := provision.ResolveSecrets(store, scopeSecrets(scope, cfg.Secrets), scopeStringMap(scope, secrets))
	if err != nil {
		return err
	}
	workspaceSecrets := unscopedSecrets(scope, cfg.Secrets, resolvedSecrets)
	return os.WriteFile(filepath.Join(p.workspaceDir(name), ".env"), []byte(formatEnv(workspaceSecrets)), 0600)
}

func registerAgent(stackCfg *config.AngeeConfig, name, templateRef string, agentCfg *config.AngeeConfig) error {
	agent, err := agentSpecFromManifest(name, templateRef, agentCfg)
	if err != nil {
		return err
	}
	mergeSecrets(&stackCfg.Secrets, agentCfg.Secrets)
	mergePortLeases(&stackCfg.PortLeases, agentCfg.PortLeases)
	mergeMCPServers(&stackCfg.MCPServers, agentCfg.MCPServers)
	mergeVolumes(&stackCfg.Volumes, agentCfg.Volumes)

	if stackCfg.Agents == nil {
		stackCfg.Agents = &config.AgentRegistrySpec{}
	}
	if stackCfg.Agents.Prefix == "" {
		stackCfg.Agents.Prefix = root.AgentsDir
	}
	if stackCfg.Agents.Items == nil {
		stackCfg.Agents.Items = map[string]config.AgentSpec{}
	}
	stackCfg.Agents.Items[name] = agent
	return nil
}

func agentSpecFromManifest(name, templateRef string, agentCfg *config.AngeeConfig) (config.AgentSpec, error) {
	svc, err := agentService(agentCfg, name)
	if err != nil {
		return config.AgentSpec{}, err
	}
	lifecycle := svc.Lifecycle
	if lifecycle == "" {
		lifecycle = config.LifecycleAgent
	}
	return config.AgentSpec{
		Image:      svc.Image,
		Command:    append([]string{}, svc.Command...),
		Template:   templateRef,
		Lifecycle:  lifecycle,
		MCPServers: mcpServerNames(agentCfg),
		Env:        copyStringMap(svc.Env),
		Resources:  svc.Resources,
	}, nil
}

func agentService(agentCfg *config.AngeeConfig, name string) (config.ServiceSpec, error) {
	if svc, ok := agentCfg.Services[name]; ok {
		return svc, nil
	}
	if len(agentCfg.Services) == 1 {
		for _, svc := range agentCfg.Services {
			return svc, nil
		}
	}
	return config.ServiceSpec{}, fmt.Errorf("agent manifest must declare service %q", name)
}

func mcpServerNames(agentCfg *config.AngeeConfig) []string {
	names := make([]string, 0, len(agentCfg.MCPServers))
	for name := range agentCfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mergeSecrets(dst *map[string]config.SecretSpec, src map[string]config.SecretSpec) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]config.SecretSpec{}
	}
	for name, spec := range src {
		(*dst)[name] = spec
	}
}

func mergePortLeases(dst *map[string]config.PortLeaseSpec, src map[string]config.PortLeaseSpec) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]config.PortLeaseSpec{}
	}
	for name, spec := range src {
		(*dst)[name] = spec
	}
}

func mergeMCPServers(dst *map[string]config.MCPServerSpec, src map[string]config.MCPServerSpec) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]config.MCPServerSpec{}
	}
	for name, spec := range src {
		(*dst)[name] = spec
	}
}

func mergeVolumes(dst *map[string]config.VolumeSpec, src map[string]config.VolumeSpec) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]config.VolumeSpec{}
	}
	for name, spec := range src {
		(*dst)[name] = spec
	}
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func (p *Platform) resolveStackState(cfg *config.AngeeConfig, ports map[string]int, secrets map[string]string) error {
	store := state.New(p.Root.Path)
	if _, err := provision.ResolvePortLeases(store, cfg.PortLeases, ports, "stack"); err != nil {
		return err
	}
	resolvedSecrets, err := provision.ResolveSecrets(store, cfg.Secrets, secrets)
	if err != nil {
		return err
	}
	return p.Root.WriteEnvFile(formatEnv(resolvedSecrets))
}

func (p *Platform) writeAgentEnv(name string, agentCfg *config.AngeeConfig) error {
	if err := os.MkdirAll(p.agentDir(name), 0755); err != nil {
		return err
	}
	secrets, err := state.New(p.Root.Path).LoadSecrets()
	if err != nil {
		return err
	}
	agentSecrets := make(map[string]state.Secret, len(agentCfg.Secrets))
	for secretName := range agentCfg.Secrets {
		secret, ok := secrets[secretName]
		if !ok {
			continue
		}
		agentSecrets[secretName] = secret
	}
	return os.WriteFile(filepath.Join(p.agentDir(name), ".env"), []byte(formatEnv(agentSecrets)), 0600)
}

func workspaceScope(name string) string {
	return "workspaces/" + name
}

func scopeName(scope, name string) string {
	if scope == "" {
		return name
	}
	return scope + "/" + name
}

func scopePortLeases(scope string, declared map[string]config.PortLeaseSpec) map[string]config.PortLeaseSpec {
	if len(declared) == 0 {
		return nil
	}
	out := make(map[string]config.PortLeaseSpec, len(declared))
	for name, spec := range declared {
		out[scopeName(scope, name)] = spec
	}
	return out
}

func scopeSecrets(scope string, declared map[string]config.SecretSpec) map[string]config.SecretSpec {
	if len(declared) == 0 {
		return nil
	}
	out := make(map[string]config.SecretSpec, len(declared))
	for name, spec := range declared {
		out[scopeName(scope, name)] = spec
	}
	return out
}

func scopeIntMap(scope string, values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]int, len(values))
	for name, value := range values {
		out[scopeName(scope, name)] = value
	}
	return out
}

func scopeStringMap(scope string, values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		out[scopeName(scope, name)] = value
	}
	return out
}

func unscopedSecrets(scope string, declared map[string]config.SecretSpec, resolved map[string]state.Secret) map[string]state.Secret {
	if len(declared) == 0 {
		return nil
	}
	out := make(map[string]state.Secret, len(declared))
	for name := range declared {
		secret, ok := resolved[scopeName(scope, name)]
		if !ok {
			continue
		}
		secret.Name = name
		out[name] = secret
	}
	return out
}

func (p *Platform) commitProvision(message string, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	if err := p.Root.Git.Add(paths...); err != nil {
		return err
	}
	changed, err := p.Root.Git.HasChanges()
	if err != nil || !changed {
		return err
	}
	_, err = p.Root.Git.Commit(message)
	return err
}

func templateSource(requested string, tmpl *copier.Template) string {
	if requested != "" {
		return requested
	}
	return tmpl.Config.Angee.Kind + "s/" + tmpl.Config.Angee.Name
}

func deriveStackName(worktree string) string {
	base := filepath.Base(filepath.Clean(worktree))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "angee"
	}
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.ReplaceAll(base, "_", "-")
	return base
}

func expandPath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func samePath(a, b string) bool {
	absA := expandPath(a)
	absB := expandPath(b)
	return filepath.Clean(absA) == filepath.Clean(absB)
}

func formatEnv(secrets map[string]state.Secret) string {
	keys := make([]string, 0, len(secrets))
	for key := range secrets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(secretEnvName(key))
		b.WriteByte('=')
		b.WriteString(shellQuote(secrets[key].Value))
		b.WriteByte('\n')
	}
	return b.String()
}

func secretEnvName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z') && !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' && r != '.' && r != '/' && r != ':'
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
