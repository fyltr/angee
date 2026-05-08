package runtime

import "context"

type Target struct {
	Root     string
	Services []string
	Build    bool
	EnvFile  string
}

type LogsRequest struct {
	Root     string
	Services []string
	Follow   bool
	EnvFile  string
}

type ServiceStatus struct {
	Name    string `json:"name"`
	Runtime string `json:"runtime"`
	State   string `json:"state"`
}

type Backend interface {
	Build(ctx context.Context, target Target) error
	Up(ctx context.Context, target Target) error
	Down(ctx context.Context, root string) error
	Start(ctx context.Context, target Target) error
	Stop(ctx context.Context, target Target) error
	Restart(ctx context.Context, target Target) error
	Logs(ctx context.Context, req LogsRequest) (<-chan string, error)
	Status(ctx context.Context, root string) ([]ServiceStatus, error)
}
