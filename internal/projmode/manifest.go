package projmode

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ManifestFile is the file the parent-walk searches for. Bare constant so
// callers (cli/project.go) stay decoupled from the type definitions below.
const ManifestFile = ".angee/project.yaml"

// Manifest is the parsed `.angee/project.yaml`. The marker file the angee
// Go CLI walks parents to find on every project-mode invocation.
type Manifest struct {
	// Version is the schema version. v1 is the only one shipping; future
	// breaking changes will bump.
	Version int `yaml:"version"`

	// Runtime is the framework adapter selector — e.g. "django-angee".
	// Maps 1:1 to a concrete impl in `internal/projmode/<name>/`.
	Runtime string `yaml:"runtime"`

	// Django is populated when Runtime == "django-angee". Other adapters
	// add their own siblings (e.g. `Rust *RustManifest`) as v1 ships
	// only the Django adapter.
	Django *DjangoManifest `yaml:"django,omitempty"`
}

// DjangoManifest is the runtime-specific block consumed by the Django
// adapter (`internal/projmode/django/`).
type DjangoManifest struct {
	// ManagePy is the path to manage.py, relative to the project root
	// (parent of .angee/). Required.
	ManagePy string `yaml:"manage_py"`

	// Cwd overrides the working directory for the subprocess. Defaults
	// to the project root. Relative to project root when not absolute.
	Cwd string `yaml:"cwd,omitempty"`

	// Invoker is one of "uv" | "python3" | "poetry". Tells the Python
	// resolver in `python.go` which strategy to prefer.
	Invoker string `yaml:"invoker,omitempty"`

	// UV holds uv-specific knobs.
	UV *DjangoUV `yaml:"uv,omitempty"`

	// Settings is the DJANGO_SETTINGS_MODULE to set in the subprocess
	// environment. When empty the adapter parses it out of manage.py's
	// `os.environ.setdefault(...)` line, or lets manage.py do it itself.
	Settings string `yaml:"settings,omitempty"`
}

// DjangoUV holds uv-specific configuration.
type DjangoUV struct {
	// Project is the value passed to `uv run --project <this>`. Defaults
	// to the project root when empty. Relative paths are resolved against
	// the project root.
	Project string `yaml:"project,omitempty"`
}

// LoadManifest reads and parses `.angee/project.yaml` at the given project
// root. Returns a typed *Manifest plus a sentinel-friendly error wrapping
// any parse / IO failure.
func LoadManifest(projectRoot string) (*Manifest, error) {
	path := filepath.Join(projectRoot, ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if m.Version == 0 {
		// Default schema version is 1 — most legible.
		m.Version = 1
	}
	if m.Runtime == "" {
		return nil, fmt.Errorf(
			"%s: 'runtime' field is required (e.g. runtime: django-angee)",
			path,
		)
	}
	return &m, nil
}

// FindProjectRoot walks parents from start (defaults to cwd) looking for
// the first directory containing `.angee/project.yaml`. Returns "" when no
// project marker is found anywhere up to the filesystem root — analogous
// to `git rev-parse --show-toplevel` returning empty outside a repo.
//
// Honoured override: ANGEE_PROJECT_ROOT env var, when set, short-circuits
// the walk (useful in containers where `.angee/` is mounted somewhere
// non-standard).
func FindProjectRoot(start string) string {
	if v := os.Getenv("ANGEE_PROJECT_ROOT"); v != "" {
		return v
	}
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		start = wd
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
