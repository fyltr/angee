// Package git provides a hybrid git client.
//
// Read-only queries (status, refs, config, ahead/behind) are implemented with
// go-git so they avoid spawning a process per call. Write and network
// operations (clone, fetch, pull, push, merge, rebase, worktree add) shell out
// to the git CLI so they inherit the user's credential helpers, SSH config,
// and submodule handling, and so worktree creation performs comparably with
// upstream git on large repos (go-git lacks parallel checkout — see
// go-git/go-git#1956).
package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Client struct {
	Bin string
}

func New() Client {
	return Client{Bin: "git"}
}

// Run executes an arbitrary git command. Kept as an escape hatch for callers
// that need git operations not exposed by the typed API (e.g. checkout in
// templates.go). Prefer adding a typed method over using Run.
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

// open returns the go-git repository at dir. Discovers the repo via parent
// walk so worktrees and bare-checked-out directories work.
func openRepo(dir string) (*gogit.Repository, error) {
	repo, err := gogit.PlainOpenWithOptions(dir, &gogit.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	if err != nil {
		return nil, fmt.Errorf("open git repo at %s: %w", dir, err)
	}
	return repo, nil
}

// --- Network / write operations: shell out to git CLI for auth correctness ---

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

func (c Client) Merge(ctx context.Context, dir, ref string) error {
	_, err := c.Run(ctx, dir, "merge", "--no-edit", ref)
	return err
}

func (c Client) Rebase(ctx context.Context, dir, ref string) error {
	_, err := c.Run(ctx, dir, "rebase", ref)
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

func (c Client) Pull(ctx context.Context, dir string) error {
	_, err := c.Run(ctx, dir, "pull", "--ff-only")
	return err
}

func (c Client) Push(ctx context.Context, dir string, ref string) error {
	args := []string{"push"}
	if ref != "" {
		remote, err := c.PushRemote(ctx, dir)
		if err != nil {
			return err
		}
		args = append(args, remote, ref)
	}
	_, err := c.Run(ctx, dir, args...)
	return err
}

func (c Client) PushSetUpstream(ctx context.Context, dir string, ref string) error {
	if ref == "" {
		return c.Push(ctx, dir, ref)
	}
	remote, err := c.PushRemote(ctx, dir)
	if err != nil {
		return err
	}
	_, err = c.Run(ctx, dir, "push", "-u", remote, ref)
	return err
}

// --- Read-only queries: go-git ---

func (c Client) RefExists(ctx context.Context, dir, ref string) bool {
	if strings.TrimSpace(ref) == "" {
		return false
	}
	repo, err := openRepo(dir)
	if err != nil {
		return false
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil || hash == nil {
		return false
	}
	if _, err := repo.CommitObject(*hash); err != nil {
		return false
	}
	return true
}

func (c Client) SyncBaseRef(ctx context.Context, dir, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("base ref is empty")
	}
	remotes, err := c.Remotes(ctx, dir)
	if err != nil {
		return "", err
	}
	candidates := []string{}
	if strings.HasPrefix(ref, "refs/") || remoteQualifiedRef(ref, remotes) {
		candidates = append(candidates, ref)
	} else {
		for _, remote := range remotes {
			if remote == "origin" {
				candidates = append([]string{remote + "/" + ref}, candidates...)
			} else {
				candidates = append(candidates, remote+"/"+ref)
			}
		}
		candidates = append(candidates, ref)
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if c.RefExists(ctx, dir, candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("base ref %q was not found after fetch", ref)
}

func remoteQualifiedRef(ref string, remotes []string) bool {
	for _, remote := range remotes {
		if strings.HasPrefix(ref, remote+"/") {
			return true
		}
	}
	return false
}

func (c Client) CurrentRef(ctx context.Context, dir string) (string, error) {
	repo, err := openRepo(dir)
	if err != nil {
		return "", err
	}
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("git head at %s: %w", dir, err)
	}
	if head.Name().IsBranch() {
		return head.Name().Short(), nil
	}
	return shortHash(head.Hash().String()), nil
}

func (c Client) CurrentBranch(ctx context.Context, dir string) (string, bool, error) {
	repo, err := openRepo(dir)
	if err != nil {
		return "", false, err
	}
	head, err := repo.Head()
	if err != nil {
		// Detached or no commits yet: not an error for this query.
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("git head at %s: %w", dir, err)
	}
	if !head.Name().IsBranch() {
		return "", false, nil
	}
	return head.Name().Short(), true, nil
}

func (c Client) Upstream(ctx context.Context, dir string) (string, bool, error) {
	branch, ok, err := c.CurrentBranch(ctx, dir)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	repo, err := openRepo(dir)
	if err != nil {
		return "", false, err
	}
	cfg, err := repo.Config()
	if err != nil {
		return "", false, fmt.Errorf("git config at %s: %w", dir, err)
	}
	br, ok := cfg.Branches[branch]
	if !ok || br.Remote == "" || br.Merge == "" {
		return "", false, nil
	}
	mergeShort := strings.TrimPrefix(string(br.Merge), "refs/heads/")
	return br.Remote + "/" + mergeShort, true, nil
}

func (c Client) AheadCount(ctx context.Context, dir, base string) (int, error) {
	ahead, _, err := c.AheadBehind(ctx, dir, base)
	return ahead, err
}

func (c Client) AheadBehind(ctx context.Context, dir, base string) (ahead int, behind int, err error) {
	repo, err := openRepo(dir)
	if err != nil {
		return 0, 0, err
	}
	headRef, err := repo.Head()
	if err != nil {
		return 0, 0, fmt.Errorf("git head at %s: %w", dir, err)
	}
	baseHash, err := repo.ResolveRevision(plumbing.Revision(base))
	if err != nil || baseHash == nil {
		return 0, 0, fmt.Errorf("resolve base %q at %s: %w", base, dir, err)
	}
	headHash := headRef.Hash()
	baseCommit, err := repo.CommitObject(*baseHash)
	if err != nil {
		return 0, 0, fmt.Errorf("base commit %s: %w", baseHash.String(), err)
	}
	headCommit, err := repo.CommitObject(headHash)
	if err != nil {
		return 0, 0, fmt.Errorf("head commit %s: %w", headHash.String(), err)
	}
	mergeBases, err := headCommit.MergeBase(baseCommit)
	if err != nil {
		return 0, 0, fmt.Errorf("merge-base of %s and %s: %w", headHash.String(), baseHash.String(), err)
	}
	if len(mergeBases) == 0 {
		return 0, 0, fmt.Errorf("no merge base between %s and %s", base, headHash.String())
	}
	mb := mergeBases[0].Hash
	ahead, err = countAncestors(repo, headHash, mb)
	if err != nil {
		return 0, 0, err
	}
	behind, err = countAncestors(repo, *baseHash, mb)
	if err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// countAncestors returns the number of commits reachable from start up to
// (but not including) stop. Mirrors `git rev-list --count stop..start`.
func countAncestors(repo *gogit.Repository, start, stop plumbing.Hash) (int, error) {
	if start == stop {
		return 0, nil
	}
	iter, err := repo.Log(&gogit.LogOptions{From: start})
	if err != nil {
		return 0, fmt.Errorf("git log from %s: %w", start.String(), err)
	}
	defer iter.Close()
	count := 0
	stopErr := errors.New("stop")
	if err := iter.ForEach(func(c *object.Commit) error {
		if c.Hash == stop {
			return stopErr
		}
		count++
		return nil
	}); err != nil && !errors.Is(err, stopErr) {
		return 0, fmt.Errorf("walk commits from %s: %w", start.String(), err)
	}
	return count, nil
}

func (c Client) Config(ctx context.Context, dir, key string) (string, bool, error) {
	repo, err := openRepo(dir)
	if err != nil {
		return "", false, nil
	}
	cfg, err := repo.Config()
	if err != nil {
		return "", false, nil
	}
	section, subsection, name, ok := splitConfigKey(key)
	if !ok {
		return "", false, nil
	}
	var raw string
	if subsection == "" {
		raw = cfg.Raw.Section(section).Option(name)
	} else {
		raw = cfg.Raw.Section(section).Subsection(subsection).Option(name)
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false, nil
	}
	return value, true, nil
}

// splitConfigKey parses git-style "section.subsection.key" or "section.key"
// into its components. Subsection may itself contain dots; everything between
// the first and last dot is treated as the subsection.
func splitConfigKey(key string) (section, subsection, name string, ok bool) {
	first := strings.Index(key, ".")
	last := strings.LastIndex(key, ".")
	if first < 0 || first == len(key)-1 {
		return "", "", "", false
	}
	if first == last {
		return key[:first], "", key[first+1:], true
	}
	return key[:first], key[first+1 : last], key[last+1:], true
}

func (c Client) Remotes(ctx context.Context, dir string) ([]string, error) {
	repo, err := openRepo(dir)
	if err != nil {
		return nil, err
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return nil, fmt.Errorf("list remotes at %s: %w", dir, err)
	}
	names := make([]string, 0, len(remotes))
	for _, r := range remotes {
		names = append(names, r.Config().Name)
	}
	return names, nil
}

func (c Client) PushRemote(ctx context.Context, dir string) (string, error) {
	branch, hasBranch, err := c.CurrentBranch(ctx, dir)
	if err != nil {
		return "", err
	}
	if hasBranch {
		if remote, ok, err := c.Config(ctx, dir, "branch."+branch+".pushRemote"); err != nil {
			return "", err
		} else if ok {
			return remote, nil
		}
	}
	if remote, ok, err := c.Config(ctx, dir, "remote.pushDefault"); err != nil {
		return "", err
	} else if ok {
		return remote, nil
	}
	if hasBranch {
		if remote, ok, err := c.Config(ctx, dir, "branch."+branch+".remote"); err != nil {
			return "", err
		} else if ok {
			return remote, nil
		}
	}
	remotes, err := c.Remotes(ctx, dir)
	if err != nil {
		return "", err
	}
	for _, remote := range remotes {
		if remote == "origin" {
			return remote, nil
		}
	}
	if len(remotes) == 1 {
		return remotes[0], nil
	}
	if len(remotes) == 0 {
		return "", fmt.Errorf("no git remotes configured")
	}
	if hasBranch {
		return "", fmt.Errorf("multiple git remotes configured; set branch.%s.pushRemote or remote.pushDefault", branch)
	}
	return "", fmt.Errorf("multiple git remotes configured; set remote.pushDefault")
}

func (c Client) Dirty(ctx context.Context, dir string) (bool, error) {
	repo, err := openRepo(dir)
	if err != nil {
		return false, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("worktree at %s: %w", dir, err)
	}
	st, err := wt.Status()
	if err != nil {
		return false, fmt.Errorf("status at %s: %w", dir, err)
	}
	return !st.IsClean(), nil
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}
