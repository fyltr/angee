package compose

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fyltr/angee/internal/runtime"
)

type Runner interface {
	Run(ctx context.Context, dir string, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

type Backend struct {
	Runner Runner
}

func NewBackend() Backend {
	return Backend{Runner: ExecRunner{}}
}

func (b Backend) Build(ctx context.Context, target runtime.Target) error {
	args := b.baseArgs(target.Root, target.EnvFile)
	args = append(args, "build")
	args = append(args, target.Services...)
	_, err := b.run(ctx, target.Root, args...)
	return err
}

func (b Backend) Up(ctx context.Context, target runtime.Target) error {
	args := b.baseArgs(target.Root, target.EnvFile)
	args = append(args, "up", "-d")
	if target.Build {
		args = append(args, "--build")
	}
	args = append(args, target.Services...)
	_, err := b.run(ctx, target.Root, args...)
	return err
}

func (b Backend) UpForeground(ctx context.Context, target runtime.Target, stdout io.Writer, stderr io.Writer) error {
	return b.Up(ctx, target)
}

func (b Backend) Down(ctx context.Context, root string) error {
	args := b.baseArgs(root, "")
	args = append(args, "down")
	_, err := b.run(ctx, root, args...)
	return err
}

func (b Backend) Start(ctx context.Context, target runtime.Target) error {
	args := b.baseArgs(target.Root, target.EnvFile)
	args = append(args, "start")
	args = append(args, target.Services...)
	_, err := b.run(ctx, target.Root, args...)
	return err
}

func (b Backend) Stop(ctx context.Context, target runtime.Target) error {
	args := b.baseArgs(target.Root, target.EnvFile)
	args = append(args, "stop")
	args = append(args, target.Services...)
	_, err := b.run(ctx, target.Root, args...)
	return err
}

func (b Backend) Restart(ctx context.Context, target runtime.Target) error {
	args := b.baseArgs(target.Root, target.EnvFile)
	args = append(args, "restart")
	args = append(args, target.Services...)
	_, err := b.run(ctx, target.Root, args...)
	return err
}

func (b Backend) Logs(ctx context.Context, req runtime.LogsRequest) (<-chan string, error) {
	args := b.baseArgs(req.Root, req.EnvFile)
	args = append(args, "logs")
	if req.Follow {
		args = append(args, "--follow")
	}
	args = append(args, req.Services...)
	out, err := b.run(ctx, req.Root, args...)
	if err != nil {
		return nil, err
	}
	ch := make(chan string, 1)
	ch <- string(out)
	close(ch)
	return ch, nil
}

func (b Backend) Status(ctx context.Context, root string) ([]runtime.ServiceStatus, error) {
	args := b.baseArgs(root, "")
	args = append(args, "ps", "--format", "json")
	out, err := b.run(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	return parsePS(out), nil
}

func (b Backend) run(ctx context.Context, root string, args ...string) ([]byte, error) {
	if b.Runner == nil {
		b.Runner = ExecRunner{}
	}
	return b.Runner.Run(ctx, root, "docker", args...)
}

func (b Backend) baseArgs(root, envFile string) []string {
	args := []string{"compose", "-f", filepath.Join(root, "docker-compose.yaml")}
	if envFile != "" {
		args = append(args, "--env-file", envFile)
	}
	return args
}

func parsePS(data []byte) []runtime.ServiceStatus {
	var statuses []runtime.ServiceStatus
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var one struct {
			Service string `json:"Service"`
			Name    string `json:"Name"`
			State   string `json:"State"`
		}
		if err := json.Unmarshal([]byte(line), &one); err != nil {
			continue
		}
		name := one.Service
		if name == "" {
			name = one.Name
		}
		if name == "" {
			continue
		}
		statuses = append(statuses, runtime.ServiceStatus{Name: name, Runtime: "container", State: one.State})
	}
	return statuses
}

var ErrNoServices = errors.New("no container services selected")
