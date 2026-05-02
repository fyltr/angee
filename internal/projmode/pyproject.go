package projmode

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// PyProjectAngeeDev mirrors the `[tool.angee.dev.*]` blocks the consumer
// can put in pyproject.toml to configure `angee dev`. All fields are
// optional; the orchestrator falls back to detection / defaults when
// blocks are absent.
//
// Example consumer pyproject.toml:
//
//	[tool.angee.dev.runtime.django]
//	manage = "src/manage.py"
//	bind   = "127.0.0.1:8100"
//
//	[tool.angee.dev.frontend]
//	cwd  = "ui/react/web"
//
//	[tool.angee.dev.processes.celery]
//	cwd     = "src"
//	command = "celery"
//	args    = ["-A", "config", "worker", "-l", "info"]
type PyProjectAngeeDev struct {
	// Runtime holds per-runtime knobs. Each adapter reads its own field;
	// other adapters' fields stay nil.
	Runtime *PyProjectRuntime `toml:"runtime,omitempty"`

	// Frontend overrides for the frontend adapter (default: auto-detect).
	Frontend *PyProjectFrontend `toml:"frontend,omitempty"`

	// Processes is the long tail of extra dev-time children — Celery,
	// MailHog, MinIO, docker compose for Postgres/Redis/agents, etc.
	// Same shape across every runtime; the orchestrator doesn't introspect
	// them. Names match `^[a-z][a-z0-9-]*$`. Reserved names ("build",
	// "runtime", "frontend") are an error and surfaced at load time.
	Processes map[string]PyProjectProcess `toml:"processes,omitempty"`
}

// PyProjectRuntime is the runtime-adapter knobs.
type PyProjectRuntime struct {
	Django *PyProjectRuntimeDjango `toml:"django,omitempty"`
}

// PyProjectRuntimeDjango is consumed by the Django adapter.
type PyProjectRuntimeDjango struct {
	Manage   string `toml:"manage,omitempty"`
	Bind     string `toml:"bind,omitempty"`
	Settings string `toml:"settings,omitempty"`
}

// PyProjectFrontend overrides frontend auto-detection.
type PyProjectFrontend struct {
	Adapter string   `toml:"adapter,omitempty"` // "vite" | future
	Cwd     string   `toml:"cwd,omitempty"`     // relative to project root
	Command string   `toml:"command,omitempty"` // "pnpm" | "npm" | "yarn"
	Args    []string `toml:"args,omitempty"`    // default: ["dev"]
}

// PyProjectProcess is one extra dev-time child.
type PyProjectProcess struct {
	Cwd     string            `toml:"cwd,omitempty"`
	Command string            `toml:"command"`
	Args    []string          `toml:"args,omitempty"`
	Env     map[string]string `toml:"env,omitempty"`
}

// pyprojectShape is the minimal TOML structure we unmarshal — only the
// `[tool.angee.dev]` subtree is interesting; everything else (project,
// dependencies, build-system, …) is ignored by the parser.
type pyprojectShape struct {
	Tool struct {
		Angee struct {
			Dev *PyProjectAngeeDev `toml:"dev,omitempty"`
		} `toml:"angee,omitempty"`
	} `toml:"tool,omitempty"`
}

// LoadPyProjectAngeeDev reads `pyproject.toml` at the given project root
// and returns the parsed `[tool.angee.dev.*]` blocks. Returns (nil, nil)
// when pyproject.toml exists but has no `[tool.angee.dev]` section —
// callers treat that as "use defaults". Returns (nil, err) on IO / parse
// failures.
func LoadPyProjectAngeeDev(projectRoot string) (*PyProjectAngeeDev, error) {
	path := filepath.Join(projectRoot, "pyproject.toml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var shape pyprojectShape
	if err := toml.Unmarshal(data, &shape); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	dev := shape.Tool.Angee.Dev
	if dev == nil {
		return nil, nil
	}
	if err := validateProcesses(dev.Processes); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return dev, nil
}

// validateProcesses enforces the rules from RUNTIMES.md §3:
//   - names match `^[a-z][a-z0-9-]*$`
//   - reserved names ("build", "runtime", "frontend") are forbidden
//   - command field is required
func validateProcesses(procs map[string]PyProjectProcess) error {
	reserved := map[string]struct{}{"build": {}, "runtime": {}, "frontend": {}}
	for name, p := range procs {
		if _, bad := reserved[name]; bad {
			return fmt.Errorf(
				"[tool.angee.dev.processes.%s]: name is reserved "+
					"(build/runtime/frontend are owned by the adapter)",
				name,
			)
		}
		if !validProcessName(name) {
			return fmt.Errorf(
				"[tool.angee.dev.processes.%s]: name must match "+
					"^[a-z][a-z0-9-]*$",
				name,
			)
		}
		if p.Command == "" {
			return fmt.Errorf(
				"[tool.angee.dev.processes.%s]: 'command' field is required",
				name,
			)
		}
	}
	return nil
}

// validProcessName checks `^[a-z][a-z0-9-]*$` without a regex dep.
func validProcessName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			continue
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		case r == '-':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
