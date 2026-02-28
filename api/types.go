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

// ScaleRequest is sent to POST /scale/{service}.
type ScaleRequest struct {
	Replicas int `json:"replicas"`
}

// ScaleResponse is returned by POST /scale/{service}.
type ScaleResponse struct {
	Service  string `json:"service"`
	Replicas int    `json:"replicas"`
}

// ServiceStatus describes the current state of a service or agent.
type ServiceStatus struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Status          string `json:"status"`
	Health          string `json:"health"`
	ReplicasRunning int    `json:"replicas_running"`
	ReplicasDesired int    `json:"replicas_desired"`
	Domains         []string `json:"domains,omitempty"`
}

// AgentInfo describes an agent with its runtime status.
type AgentInfo struct {
	Name      string `json:"name"`
	Lifecycle string `json:"lifecycle"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Health    string `json:"health"`
}

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

// DownResponse is returned by POST /down.
type DownResponse struct {
	Status string `json:"status"`
}

// CommitInfo holds a single git log entry.
type CommitInfo struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

// AgentActionResponse is returned by agent start/stop operations.
type AgentActionResponse struct {
	Status string `json:"status,omitempty"`
	Agent  string `json:"agent,omitempty"`
}

// ConfigGetResponse wraps the config for JSON serialization from MCP.
type ConfigGetResponse struct {
	Config any `json:"config"`
}

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
