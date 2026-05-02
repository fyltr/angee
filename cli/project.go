package cli

import (
	"fmt"
	"os"
	"syscall"

	"github.com/fyltr/angee/internal/projmode"
	"github.com/fyltr/angee/internal/projmode/django"
)

// dispatchToRuntime is the project-mode hot path for `angee build`,
// `angee migrate`, `angee doctor`, `angee fixtures`. It loads the runtime
// manifest, resolves Python, picks the adapter, and exec's the framework
// subcommand. Signals + exit codes pass through to the parent shell so
// the user sees the framework's output verbatim.
//
// Returns an error only on setup failures (no manifest, unknown adapter,
// no Python). On a successful syscall.Exec this function does not return
// — control transfers to the framework.
func dispatchToRuntime(sub string, args []string) error {
	projectRoot := projmode.FindProjectRoot("")
	if projectRoot == "" {
		return fmt.Errorf(
			"angee %s: no .angee/project.yaml found in this or any "+
				"parent directory.\n"+
				"  In compose-mode, this command isn't available; "+
				"see `angee --help` for compose-mode commands.",
			sub,
		)
	}
	manifest, err := projmode.LoadManifest(projectRoot)
	if err != nil {
		return fmt.Errorf("angee %s: %w", sub, err)
	}
	adapter, err := pickAdapter(manifest.Runtime)
	if err != nil {
		return fmt.Errorf("angee %s: %w", sub, err)
	}

	uvProject := ""
	if manifest.Django != nil && manifest.Django.UV != nil {
		uvProject = manifest.Django.UV.Project
	}
	py, err := projmode.ResolvePython(projectRoot, uvProject)
	if err != nil {
		return fmt.Errorf("angee %s: %w", sub, err)
	}

	pyProject, err := projmode.LoadPyProjectAngeeDev(projectRoot)
	if err != nil {
		return fmt.Errorf("angee %s: %w", sub, err)
	}

	ctx := projmode.Ctx{
		ProjectRoot: projectRoot,
		Manifest:    manifest,
		PyProject:   pyProject,
		Python:      py,
	}
	proc, err := adapter.Dispatch(ctx, sub, args)
	if err != nil {
		return fmt.Errorf("angee %s: %w", sub, err)
	}

	// Hand off to the framework. After this, we're gone — no Go cleanup
	// runs, signals + stdio + exit code pass through cleanly.
	if proc.Cwd != "" {
		if err := os.Chdir(proc.Cwd); err != nil {
			return fmt.Errorf("angee %s: chdir %s: %w", sub, proc.Cwd, err)
		}
	}
	argv := append([]string{proc.Command}, proc.Args...)
	env := mergeEnv(proc.Env)
	return syscall.Exec(proc.Command, argv, env)
}

// pickAdapter selects a concrete projmode.Adapter by `runtime:` field.
// Adding a new adapter means adding a case here (and a new package).
func pickAdapter(name string) (projmode.Adapter, error) {
	switch name {
	case django.Name:
		return django.New(), nil
	default:
		return nil, fmt.Errorf(
			"unknown runtime %q (supported: %s)",
			name, django.Name,
		)
	}
}

// mergeEnv overlays the per-process env map onto the inherited environment.
// Returns a fresh `KEY=VALUE` slice suitable for syscall.Exec's envv arg.
// nil overlay means "inherit" → return os.Environ() unchanged.
func mergeEnv(overlay map[string]string) []string {
	if len(overlay) == 0 {
		return os.Environ()
	}
	base := os.Environ()
	out := make([]string, 0, len(base)+len(overlay))
	seen := make(map[string]bool, len(overlay))
	for _, kv := range base {
		// Only override when the overlay has the key.
		if eq := indexEq(kv); eq >= 0 {
			key := kv[:eq]
			if v, ok := overlay[key]; ok {
				out = append(out, key+"="+v)
				seen[key] = true
				continue
			}
		}
		out = append(out, kv)
	}
	for k, v := range overlay {
		if !seen[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// indexEq returns the index of the first '=' in s, or -1 if absent.
// Tiny helper — strings.Index would be fine, but this keeps the import
// surface tight.
func indexEq(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return i
		}
	}
	return -1
}
