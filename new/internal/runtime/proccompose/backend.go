package proccompose

import (
	"context"
	"fmt"
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

func (b Backend) Build(context.Context, runtime.Target) error {
	return nil
}

func (b Backend) Up(ctx context.Context, target runtime.Target) error {
	args := b.baseArgs(target.Root)
	args = append(args, "up", "-d")
	args = append(args, target.Services...)
	_, err := b.run(ctx, target.Root, args...)
	return err
}

func (b Backend) Down(ctx context.Context, root string) error {
	args := b.baseArgs(root)
	args = append(args, "down")
	_, err := b.run(ctx, root, args...)
	return err
}

func (b Backend) Start(ctx context.Context, target runtime.Target) error {
	args := b.baseArgs(target.Root)
	args = append(args, "process", "start")
	args = append(args, target.Services...)
	_, err := b.run(ctx, target.Root, args...)
	return err
}

func (b Backend) Stop(ctx context.Context, target runtime.Target) error {
	args := b.baseArgs(target.Root)
	args = append(args, "process", "stop")
	args = append(args, target.Services...)
	_, err := b.run(ctx, target.Root, args...)
	return err
}

func (b Backend) Restart(ctx context.Context, target runtime.Target) error {
	args := b.baseArgs(target.Root)
	args = append(args, "process", "restart")
	args = append(args, target.Services...)
	_, err := b.run(ctx, target.Root, args...)
	return err
}

func (b Backend) Logs(ctx context.Context, req runtime.LogsRequest) (<-chan string, error) {
	args := b.baseArgs(req.Root)
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

func (b Backend) Status(context.Context, string) ([]runtime.ServiceStatus, error) {
	return nil, nil
}

func (b Backend) run(ctx context.Context, root string, args ...string) ([]byte, error) {
	if b.Runner == nil {
		b.Runner = ExecRunner{}
	}
	return b.Runner.Run(ctx, root, "process-compose", args...)
}

func (b Backend) baseArgs(root string) []string {
	return []string{"-f", filepath.Join(root, "process-compose.yaml")}
}
