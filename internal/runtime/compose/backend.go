// Package compose implements RuntimeBackend using docker compose CLI.
package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fyltr/angee-go/internal/runtime"
)

// Backend implements RuntimeBackend by shelling out to `docker compose`.
type Backend struct {
	ComposeFile string // path to the generated docker-compose.yaml
	ProjectName string // docker compose project name
	EnvFile     string // optional: path to root .env file
}

// New creates a DockerCompose backend.
func New(rootPath, projectName string) *Backend {
	return &Backend{
		ComposeFile: filepath.Join(rootPath, "docker-compose.yaml"),
		ProjectName: projectName,
		EnvFile:     filepath.Join(rootPath, ".env"),
	}
}

func (b *Backend) args(extra ...string) []string {
	base := []string{
		"compose",
		"--project-name", b.ProjectName,
		"--file", b.ComposeFile,
	}
	if _, err := os.Stat(b.EnvFile); err == nil {
		base = append(base, "--env-file", b.EnvFile)
	}
	return append(base, extra...)
}

func (b *Backend) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker %s: %w\n%s", strings.Join(args[:2], " "), err, stderr.String())
	}
	return out.Bytes(), nil
}

// Diff shows what would change (docker compose up --dry-run is not universally available,
// so we compare `ps` output against the compose file service list).
func (b *Backend) Diff(ctx context.Context, composeFile string) (*runtime.ChangeSet, error) {
	current, err := b.Status(ctx)
	if err != nil {
		return nil, err
	}

	running := make(map[string]bool)
	for _, s := range current {
		if s.Status == "running" {
			running[s.Name] = true
		}
	}

	// Parse the compose file service names
	out, err := b.run(ctx, b.args("config", "--services")...)
	if err != nil {
		return nil, err
	}

	desired := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			desired[line] = true
		}
	}

	cs := &runtime.ChangeSet{}
	for svc := range desired {
		if running[svc] {
			cs.Update = append(cs.Update, svc)
		} else {
			cs.Add = append(cs.Add, svc)
		}
	}
	for svc := range running {
		if !desired[svc] {
			cs.Remove = append(cs.Remove, svc)
		}
	}
	return cs, nil
}

// Apply runs `docker compose up -d --remove-orphans`.
func (b *Backend) Apply(ctx context.Context, composeFile string) (*runtime.ApplyResult, error) {
	before, _ := b.Status(ctx)

	_, err := b.run(ctx, b.args("up", "-d", "--remove-orphans", "--pull", "missing")...)
	if err != nil {
		return nil, fmt.Errorf("compose up: %w", err)
	}

	after, err := b.Status(ctx)
	if err != nil {
		return nil, err
	}

	beforeMap := make(map[string]bool)
	for _, s := range before {
		beforeMap[s.Name] = true
	}

	result := &runtime.ApplyResult{}
	for _, s := range after {
		if beforeMap[s.Name] {
			result.ServicesUpdated = append(result.ServicesUpdated, s.Name)
		} else {
			result.ServicesStarted = append(result.ServicesStarted, s.Name)
		}
	}

	return result, nil
}

// composePS is the JSON format returned by docker compose ps --format json.
type composePS struct {
	Name    string `json:"Name"`
	Service string `json:"Service"`
	Status  string `json:"Status"`
	Health  string `json:"Health"`
	Image   string `json:"Image"`
	Ports   string `json:"Ports"`
}

// Status returns the current state of all containers in the project.
func (b *Backend) Status(ctx context.Context) ([]*runtime.ServiceStatus, error) {
	out, err := b.run(ctx, b.args("ps", "--format", "json", "--all")...)
	if err != nil {
		// Not an error if the project doesn't exist yet
		return nil, nil
	}

	var results []*runtime.ServiceStatus
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var ps composePS
		if err := json.Unmarshal([]byte(line), &ps); err != nil {
			continue
		}

		svcType := "service"
		if strings.HasPrefix(ps.Service, "agent-") {
			svcType = "agent"
		}

		status := "stopped"
		if strings.Contains(strings.ToLower(ps.Status), "up") ||
			strings.Contains(strings.ToLower(ps.Status), "running") {
			status = "running"
		} else if strings.Contains(strings.ToLower(ps.Status), "exit") {
			status = "stopped"
		} else if strings.Contains(strings.ToLower(ps.Status), "error") {
			status = "error"
		}

		health := "unknown"
		if ps.Health != "" {
			health = strings.ToLower(ps.Health)
		}

		results = append(results, &runtime.ServiceStatus{
			Name:            ps.Service,
			Type:            svcType,
			Status:          status,
			Health:          health,
			ContainerID:     ps.Name,
			Image:           ps.Image,
			ReplicasRunning: 1,
			ReplicasDesired: 1,
			LastUpdated:     time.Now(),
		})
	}

	return results, nil
}

// Logs returns a reader for service logs.
func (b *Backend) Logs(ctx context.Context, service string, opts runtime.LogOptions) (io.ReadCloser, error) {
	args := b.args("logs")
	if opts.Follow {
		args = append(args, "--follow")
	}
	if opts.Lines > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", opts.Lines))
	}
	if opts.Since != "" {
		args = append(args, "--since", opts.Since)
	}
	if service != "" {
		args = append(args, service)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	go func() {
		err := cmd.Run()
		pw.CloseWithError(err)
	}()

	return pr, nil
}

// Scale adjusts the replica count for a service.
func (b *Backend) Scale(ctx context.Context, service string, replicas int) error {
	_, err := b.run(ctx, b.args("up", "-d", "--scale", fmt.Sprintf("%s=%d", service, replicas))...)
	return err
}

// Stop stops the named services without removing them.
func (b *Backend) Stop(ctx context.Context, services ...string) error {
	args := b.args("stop")
	args = append(args, services...)
	_, err := b.run(ctx, args...)
	return err
}

// Down brings the entire stack down.
func (b *Backend) Down(ctx context.Context) error {
	_, err := b.run(ctx, b.args("down")...)
	return err
}
