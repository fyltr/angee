// Package git wraps git CLI operations for ANGEE_ROOT management.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// refRE constrains user-supplied git refs (SHAs, branches, tags) to
// characters that can never be misinterpreted as a flag. Without this,
// a value like `--upload-pack=evil` flowing from an HTTP request into
// `git revert <ref>` would smuggle a flag into git's argv.
//
// The git-check-ref-format rules are stricter than this; we accept the
// subset that's universally safe and reject everything else with a clear
// error message rather than handing argv to git and hoping.
var refRE = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// validateRef returns an error if ref contains anything outside refRE,
// is empty, or starts with `-` (defence in depth).
func validateRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("empty ref")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("ref must not start with '-': %q", ref)
	}
	if !refRE.MatchString(ref) {
		return fmt.Errorf("ref %q contains characters outside %s", ref, refRE.String())
	}
	return nil
}

// Repo wraps git operations for a single repository.
type Repo struct {
	Path string
}

// New creates a Repo for the given path.
func New(path string) *Repo {
	return &Repo{Path: path}
}

// run executes a git subcommand with the given context. Callers without a
// caller-bound context can pass context.Background() — methods that flow
// from HTTP handlers (Revert, ResetHard, Log) accept ctx so a client
// disconnect cancels the underlying git process instead of leaking it.
func (r *Repo) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Path
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// Init initializes a new git repository at r.Path.
func (r *Repo) Init() error {
	if err := os.MkdirAll(r.Path, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	_, err := r.run(context.Background(), "init", "-b", "main")
	if err != nil {
		// Try without -b for older git versions
		_, err = r.run(context.Background(), "init")
	}
	return err
}

// ConfigureUser sets the local git user for commits.
func (r *Repo) ConfigureUser(name, email string) error {
	if _, err := r.run(context.Background(), "config", "user.name", name); err != nil {
		return err
	}
	_, err := r.run(context.Background(), "config", "user.email", email)
	return err
}

// Add stages the given paths (or "." for all).
func (r *Repo) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := r.run(context.Background(), args...)
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
	_, err := r.run(context.Background(), "commit", "-m", message)
	if err != nil {
		return "", err
	}
	return r.CurrentSHA()
}

// CurrentSHA returns the SHA of the current HEAD commit.
func (r *Repo) CurrentSHA() (string, error) {
	return r.run(context.Background(), "rev-parse", "HEAD")
}

// Log returns the last n commit log entries.
func (r *Repo) Log(n int) ([]CommitInfo, error) {
	return r.LogCtx(context.Background(), n)
}

// LogCtx returns the last n commit log entries; ctx cancellation kills git.
func (r *Repo) LogCtx(ctx context.Context, n int) ([]CommitInfo, error) {
	out, err := r.run(ctx,
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

// Status returns the short git status output.
func (r *Repo) Status() (string, error) {
	return r.run(context.Background(), "status", "--short")
}

// HasChanges returns true if there are uncommitted changes.
func (r *Repo) HasChanges() (bool, error) {
	out, err := r.Status()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// RevertCtx creates a revert commit; ctx cancellation kills git.
// SHA is validated against refRE so it can't smuggle a git flag.
func (r *Repo) RevertCtx(ctx context.Context, sha string) error {
	if err := validateRef(sha); err != nil {
		return err
	}
	_, err := r.run(ctx, "revert", "--no-edit", "--", sha)
	return err
}

// ResetHardCtx resets to ref; ctx cancellation kills git.
// Ref is validated against refRE so it can't smuggle a git flag.
func (r *Repo) ResetHardCtx(ctx context.Context, ref string) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	// `git reset --hard` doesn't accept the `--` positional separator
	// (it's not a pathspec command), so we rely on validateRef alone.
	_, err := r.run(ctx, "reset", "--hard", ref)
	return err
}

// CloneCtx is Clone with a context for cancellation.
func CloneCtx(ctx context.Context, url, dest, branch string) error {
	args := []string{
		"-c", "protocol.https.allow=always",
		"-c", "protocol.file.allow=user",
		"-c", "protocol.http.allow=user",
		"-c", "protocol.git.allow=never",
		"-c", "protocol.ssh.allow=user",
		"clone",
	}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--", url, dest)

	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w: %s", url, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// SyncCtx fetches updates for an existing clone, optionally checks out ref,
// and fast-forwards the current branch when attached to one.
func (r *Repo) SyncCtx(ctx context.Context, ref string) error {
	if _, err := r.run(ctx, "fetch", "--all", "--tags", "--prune"); err != nil {
		return err
	}
	if ref != "" && ref != "current" {
		if err := validateRef(ref); err != nil {
			return err
		}
		if _, err := r.run(ctx, "checkout", ref); err != nil {
			return err
		}
	}
	branch, err := r.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	if branch == "HEAD" {
		return nil
	}
	_, err = r.run(ctx, "pull", "--ff-only")
	return err
}

// IsRepo returns true only when path is the repository root. Git normally
// searches parent directories, but ANGEE_ROOT can live inside an ignored app
// repository; parent repositories must not be treated as the ANGEE_ROOT repo.
func IsRepo(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	repoRoot, err := filepath.Abs(strings.TrimSpace(string(out)))
	if err != nil {
		return false
	}
	return canonicalPath(repoRoot) == canonicalPath(abs)
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}
