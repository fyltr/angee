// Package api defines request/response types shared between the CLI and operator.
package api

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthResponse is returned by GET /health.
type HealthResponse struct {
	Status  string `json:"status"`
	Root    string `json:"root"`
	Runtime string `json:"runtime"`
}

// ProvisionResponse is returned by stack, workspace, agent, and reconcile
// provisioning endpoints.
type ProvisionResponse struct {
	Status   string   `json:"status"`
	Message  string   `json:"message,omitempty"`
	Root     string   `json:"root,omitempty"`
	Manifest string   `json:"manifest,omitempty"`
	Changed  []string `json:"changed,omitempty"`
}

// ── Provisioning ────────────────────────────────────────────────────────────

type StackInitRequest struct {
	Name     string            `json:"name"`
	Path     string            `json:"path,omitempty"`
	Root     string            `json:"root,omitempty"`
	Template string            `json:"template,omitempty"`
	Set      map[string]string `json:"set,omitempty"`
	Secrets  map[string]string `json:"secrets,omitempty"`
	Ports    map[string]int    `json:"ports,omitempty"`
	Force    bool              `json:"force,omitempty"`
	Yes      bool              `json:"yes,omitempty"`
}

type StackUpdateRequest struct {
	Root    string            `json:"root,omitempty"`
	Set     map[string]string `json:"set,omitempty"`
	Secrets map[string]string `json:"secrets,omitempty"`
	Ports   map[string]int    `json:"ports,omitempty"`
	Yes     bool              `json:"yes,omitempty"`
}

type WorkspaceInitRequest struct {
	Name           string            `json:"name"`
	Root           string            `json:"root,omitempty"`
	Template       string            `json:"template,omitempty"`
	Branch         string            `json:"branch,omitempty"`
	Overrides      map[string]string `json:"overrides,omitempty"`
	Secrets        map[string]string `json:"secrets,omitempty"`
	Ports          map[string]int    `json:"ports,omitempty"`
	CreateBranches bool              `json:"create_branches,omitempty"`
	Start          bool              `json:"start,omitempty"`
	Yes            bool              `json:"yes,omitempty"`
}

type WorkspaceUpdateRequest struct {
	Name      string            `json:"name"`
	Root      string            `json:"root,omitempty"`
	Ref       string            `json:"ref,omitempty"`
	Overrides map[string]string `json:"overrides,omitempty"`
	Secrets   map[string]string `json:"secrets,omitempty"`
	Ports     map[string]int    `json:"ports,omitempty"`
	Sync      bool              `json:"sync,omitempty"`
	Restart   bool              `json:"restart,omitempty"`
	Yes       bool              `json:"yes,omitempty"`
}

type WorkspaceListRequest struct {
	Root string `json:"root,omitempty"`
}

type WorkspaceDevRequest struct {
	Root string `json:"root,omitempty"`
	Name string `json:"name"`
}

type AgentInitRequest struct {
	Name              string            `json:"name"`
	Root              string            `json:"root,omitempty"`
	Template          string            `json:"template,omitempty"`
	WorkspaceTemplate string            `json:"workspace_template,omitempty"`
	Branch            string            `json:"branch,omitempty"`
	Overrides         map[string]string `json:"overrides,omitempty"`
	Secrets           map[string]string `json:"secrets,omitempty"`
	Ports             map[string]int    `json:"ports,omitempty"`
	CreateBranches    bool              `json:"create_branches,omitempty"`
	Start             bool              `json:"start,omitempty"`
	Yes               bool              `json:"yes,omitempty"`
}

type AgentUpdateRequest struct {
	Name              string            `json:"name"`
	Root              string            `json:"root,omitempty"`
	Template          string            `json:"template,omitempty"`
	WorkspaceTemplate string            `json:"workspace_template,omitempty"`
	Ref               string            `json:"ref,omitempty"`
	Overrides         map[string]string `json:"overrides,omitempty"`
	Secrets           map[string]string `json:"secrets,omitempty"`
	Ports             map[string]int    `json:"ports,omitempty"`
	Restart           bool              `json:"restart,omitempty"`
	Yes               bool              `json:"yes,omitempty"`
}

type AgentActionRequest struct {
	Root string `json:"root,omitempty"`
	Name string `json:"name"`
}

type AgentChatRequest struct {
	Root string `json:"root,omitempty"`
	Name string `json:"name"`
}

type AgentAskRequest struct {
	Root    string `json:"root,omitempty"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

type ReconcileRequest struct {
	Root   string   `json:"root,omitempty"`
	Mode   string   `json:"mode,omitempty"`
	Only   []string `json:"only,omitempty"`
	Except []string `json:"except,omitempty"`
	Follow bool     `json:"follow,omitempty"`
}

// ── Deploy ──────────────────────────────────────────────────────────────────

// DeployRequest is sent to POST /deploy.
type DeployRequest struct {
	Message string `json:"message,omitempty"`
}

// ApplyResult is the outcome of a deploy or agent start.
type ApplyResult struct {
	ServicesStarted []string `json:"services_started"`
	ServicesUpdated []string `json:"services_updated"`
	ServicesRemoved []string `json:"services_removed"`
}

// RollbackRequest is sent to POST /rollback.
type RollbackRequest struct {
	SHA string `json:"sha"`
}

// RollbackResponse is returned by POST /rollback.
type RollbackResponse struct {
	RolledBackTo string       `json:"rolled_back_to"`
	Deploy       *ApplyResult `json:"deploy,omitempty"`
}

// ChangeSet describes the diff between desired and current state (GET /plan).
type ChangeSet struct {
	Add    []string `json:"add"`
	Update []string `json:"update"`
	Remove []string `json:"remove"`
}

// ── Services ────────────────────────────────────────────────────────────────

// ScaleRequest is sent to POST /scale/{service}.
type ScaleRequest struct {
	Service  string `json:"service"`
	Replicas int    `json:"replicas"`
}

// ScaleResponse is returned by POST /scale/{service}.
type ScaleResponse struct {
	Service  string `json:"service"`
	Replicas int    `json:"replicas"`
}

// ServiceStatus describes the current state of a service or agent.
type ServiceStatus struct {
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Status          string   `json:"status"`
	Health          string   `json:"health"`
	ReplicasRunning int      `json:"replicas_running"`
	ReplicasDesired int      `json:"replicas_desired"`
	Domains         []string `json:"domains,omitempty"`
}

// DownResponse is returned by POST /down.
type DownResponse struct {
	Status string `json:"status"`
}

// ── Agents ──────────────────────────────────────────────────────────────────

// AgentInfo describes an agent with its runtime status.
type AgentInfo struct {
	Name      string `json:"name"`
	Lifecycle string `json:"lifecycle"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Health    string `json:"health"`
}

// AgentActionResponse is returned by agent start/stop operations.
type AgentActionResponse struct {
	Status string `json:"status,omitempty"`
	Agent  string `json:"agent,omitempty"`
}

// ── Config ──────────────────────────────────────────────────────────────────

// ConfigSetRequest is sent to POST /config.
type ConfigSetRequest struct {
	Content string `json:"content"`
	Message string `json:"message,omitempty"`
	Deploy  bool   `json:"deploy,omitempty"`
}

// ConfigSetResponse is returned by POST /config.
type ConfigSetResponse struct {
	SHA     string       `json:"sha"`
	Message string       `json:"message"`
	Deploy  *ApplyResult `json:"deploy,omitempty"`
}

// ConfigGetResponse wraps the config for JSON serialization from MCP.
type ConfigGetResponse struct {
	Config any `json:"config"`
}

// ── Credentials ─────────────────────────────────────────────────────────────

// CredentialSetRequest is sent to POST /credentials.
type CredentialSetRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ── History ─────────────────────────────────────────────────────────────────

// CommitInfo holds a single git log entry.
type CommitInfo struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

// ── Chat ────────────────────────────────────────────────────────────────────

// ChatRequest is sent to POST /agents/{name}/chat.
type ChatRequest struct {
	Message string `json:"message"`
	Agent   string `json:"agent,omitempty"`
}

// ChatResponse is returned by POST /agents/{name}/chat.
type ChatResponse struct {
	Response string `json:"response,omitempty"`
	Message  string `json:"message,omitempty"`
}
