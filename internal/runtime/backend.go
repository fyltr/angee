// Package runtime defines the RuntimeBackend interface and related types.
package runtime

import (
	"context"
	"io"
	"time"
)

// ServiceStatus describes the current state of a single service or agent.
type ServiceStatus struct {
	Name            string    `json:"name"`
	Type            string    `json:"type"`   // service | agent
	Status          string    `json:"status"` // running | stopped | error | starting
	Health          string    `json:"health"` // healthy | unhealthy | unknown
	ContainerID     string    `json:"container_id,omitempty"`
	Image           string    `json:"image,omitempty"`
	ReplicasRunning int       `json:"replicas_running"`
	ReplicasDesired int       `json:"replicas_desired"`
	Domains         []string  `json:"domains,omitempty"`
	LastUpdated     time.Time `json:"last_updated"`
}

// ChangeSet describes the diff between desired and current state.
type ChangeSet struct {
	Add    []string `json:"add"`
	Update []string `json:"update"`
	Remove []string `json:"remove"`
}

// ApplyResult is returned after a successful Apply.
type ApplyResult struct {
	ServicesStarted []string `json:"services_started"`
	ServicesUpdated []string `json:"services_updated"`
	ServicesRemoved []string `json:"services_removed"`
}

// LogOptions controls log output.
type LogOptions struct {
	Follow bool
	Lines  int
	Since  string
}

// RuntimeBackend is the interface that all runtime implementations must satisfy.
// The operator uses this interface exclusively — it never calls Docker or K8s APIs directly.
type RuntimeBackend interface {
	// Diff returns what would change if Apply were called.
	Diff(ctx context.Context, composeFile string) (*ChangeSet, error)

	// Apply starts/updates/removes services to match the compiled compose file.
	Apply(ctx context.Context, composeFile string) (*ApplyResult, error)

	// Pull pulls the latest images for all services in the compose file.
	Pull(ctx context.Context) error

	// Status returns the current state of all services.
	Status(ctx context.Context) ([]*ServiceStatus, error)

	// Logs streams logs for a named service.
	Logs(ctx context.Context, service string, opts LogOptions) (io.ReadCloser, error)

	// Scale adjusts the replica count for a service.
	Scale(ctx context.Context, service string, replicas int) error

	// Stop gracefully stops the named services (or all if empty).
	Stop(ctx context.Context, services ...string) error

	// Down brings the entire stack down.
	Down(ctx context.Context) error
}
