// Package git wraps git CLI operations for ANGEE_ROOT management.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Repo wraps git operations for a single repository.
type Repo struct {
	Path string
}

// New creates a Repo for the given path.
func New(path string) *Repo {
	return &Repo{Path: path}
}

func (r *Repo) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Path
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(out.String()), nil
}

// Init initializes a new git repository at r.Path.
func (r *Repo) Init() error {
	if err := os.MkdirAll(r.Path, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	_, err := r.run("init", "-b", "main")
	if err != nil {
		// Try without -b for older git versions
		_, err = r.run("init")
	}
	return err
}

// ConfigureUser sets the local git user for commits.
func (r *Repo) ConfigureUser(name, email string) error {
	if _, err := r.run("config", "user.name", name); err != nil {
		return err
	}
	_, err := r.run("config", "user.email", email)
	return err
}

// Add stages the given paths (or "." for all).
func (r *Repo) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := r.run(args...)
	return err
}

// CommitInfo holds a single git log entry.
type CommitInfo struct {
	SHA     string
	Author  string
	Date    string
	Message string
}

// Commit creates a new commit. Returns the new SHA.
func (r *Repo) Commit(message string) (string, error) {
	_, err := r.run("commit", "-m", message)
	if err != nil {
		return "", err
	}
	return r.CurrentSHA()
}

// CurrentSHA returns the SHA of the current HEAD commit.
func (r *Repo) CurrentSHA() (string, error) {
	return r.run("rev-parse", "HEAD")
}

// CurrentBranch returns the name of the current branch.
func (r *Repo) CurrentBranch() (string, error) {
	return r.run("rev-parse", "--abbrev-ref", "HEAD")
}

// Log returns the last n commit log entries.
func (r *Repo) Log(n int) ([]CommitInfo, error) {
	out, err := r.run(
		"log",
		fmt.Sprintf("-%d", n),
		"--pretty=format:%H|%an|%ad|%s",
		"--date=short",
	)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var commits []CommitInfo
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, CommitInfo{
			SHA:     parts[0],
			Author:  parts[1],
			Date:    parts[2],
			Message: parts[3],
		})
	}
	return commits, nil
}

// Diff returns the unstaged diff of the repository.
func (r *Repo) Diff() (string, error) {
	return r.run("diff")
}

// DiffCached returns the staged diff.
func (r *Repo) DiffCached() (string, error) {
	return r.run("diff", "--cached")
}

// Status returns the short git status output.
func (r *Repo) Status() (string, error) {
	return r.run("status", "--short")
}

// HasChanges returns true if there are uncommitted changes.
func (r *Repo) HasChanges() (bool, error) {
	out, err := r.Status()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// Checkout switches to the given branch, creating it if create is true.
func (r *Repo) Checkout(branch string, create bool) error {
	args := []string{"checkout"}
	if create {
		args = append(args, "-b")
	}
	args = append(args, branch)
	_, err := r.run(args...)
	return err
}

// Revert creates a revert commit for the given SHA.
func (r *Repo) Revert(sha string) error {
	_, err := r.run("revert", "--no-edit", sha)
	return err
}

// ResetHard resets to a given ref (use with care).
func (r *Repo) ResetHard(ref string) error {
	_, err := r.run("reset", "--hard", ref)
	return err
}

// IsRepo returns true if the path is a git repository.
func IsRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// HasInitialCommit returns true if the repo has at least one commit.
func (r *Repo) HasInitialCommit() bool {
	_, err := r.run("rev-parse", "HEAD")
	return err == nil
}
