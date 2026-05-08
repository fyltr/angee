package service

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	hookKindPostStart = "post-start"
	hookKindPreStop   = "pre-stop"
	hooksSubdir       = ".angee/hooks"
)

// runWorkspaceHooks executes every executable script under
// <workspacePath>/.angee/hooks/<kind>.d/, sorted by filename.
//
// Templates render these scripts at workspace-create time; the binary
// is intentionally agnostic about their content. Each script receives
// an ANGEE_* env-var bag derived from the workspace's resolved state.
//
// Scripts are run sequentially. The first non-zero exit aborts the
// remaining scripts and is returned to the caller (so a failed
// post-start is loud). Templates that want best-effort semantics can
// trail their commands with `|| true`.
func runWorkspaceHooks(ctx context.Context, workspacePath, name, kind string, allocations map[string]int) error {
	dir := filepath.Join(workspacePath, hooksSubdir, kind+".d")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("hooks: read %s: %w", dir, err)
	}
	scripts := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("hooks: stat %s: %w", entry.Name(), err)
		}
		if !isExecutable(info.Mode()) {
			continue
		}
		scripts = append(scripts, entry.Name())
	}
	sort.Strings(scripts)
	if len(scripts) == 0 {
		return nil
	}
	env := workspaceHookEnv(workspacePath, name, allocations)
	for _, script := range scripts {
		path := filepath.Join(dir, script)
		cmd := exec.CommandContext(ctx, path)
		cmd.Dir = workspacePath
		cmd.Env = env
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hooks: %s/%s: %w", kind, script, err)
		}
	}
	return nil
}

func isExecutable(mode fs.FileMode) bool {
	return mode&0o111 != 0
}

func workspaceHookEnv(workspacePath, name string, allocations map[string]int) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"ANGEE_WORKSPACE_NAME="+name,
		"ANGEE_WORKSPACE_PATH="+workspacePath,
	)
	keys := make([]string, 0, len(allocations))
	for key := range allocations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		envKey := "ANGEE_ALLOC_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		env = append(env, envKey+"="+strconv.Itoa(allocations[key]))
	}
	return env
}
