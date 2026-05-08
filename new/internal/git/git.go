package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	Bin string
}

func New() Client {
	return Client{Bin: "git"}
}

func (c Client) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	bin := c.Bin
	if bin == "" {
		bin = "git"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return out, nil
}

func (c Client) Clone(ctx context.Context, repo, dest string, args ...string) error {
	cmdArgs := append([]string{"clone"}, args...)
	cmdArgs = append(cmdArgs, repo, dest)
	_, err := c.Run(ctx, "", cmdArgs...)
	return err
}

func (c Client) CloneRef(ctx context.Context, repo, dest, ref string) error {
	args := []string{}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	return c.Clone(ctx, repo, dest, args...)
}

func (c Client) Fetch(ctx context.Context, dir string) error {
	_, err := c.Run(ctx, dir, "fetch", "--all", "--prune")
	return err
}

func (c Client) WorktreeAdd(ctx context.Context, repoDir, dest, ref string) error {
	args := []string{"worktree", "add", dest}
	if ref != "" {
		args = append(args, ref)
	}
	_, err := c.Run(ctx, repoDir, args...)
	return err
}

func (c Client) WorktreeAddBranch(ctx context.Context, repoDir, dest, branch, ref string) error {
	args := []string{"worktree", "add"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, dest)
	if ref != "" {
		args = append(args, ref)
	}
	_, err := c.Run(ctx, repoDir, args...)
	return err
}

func (c Client) CurrentRef(ctx context.Context, dir string) (string, error) {
	out, err := c.Run(ctx, dir, "branch", "--show-current")
	if err == nil && strings.TrimSpace(string(out)) != "" {
		return strings.TrimSpace(string(out)), nil
	}
	out, err = c.Run(ctx, dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (c Client) Dirty(ctx context.Context, dir string) (bool, error) {
	out, err := c.Run(ctx, dir, "status", "--short")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (c Client) Pull(ctx context.Context, dir string) error {
	_, err := c.Run(ctx, dir, "pull", "--ff-only")
	return err
}

func (c Client) Push(ctx context.Context, dir string, ref string) error {
	args := []string{"push"}
	if ref != "" {
		args = append(args, "origin", ref)
	}
	_, err := c.Run(ctx, dir, args...)
	return err
}
