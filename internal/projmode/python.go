package projmode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// PythonResolution is the strategy chosen for invoking Python in the
// consumer project. Prepended to the manage.py argv when `Adapter.Dispatch`
// or `Adapter.Watcher`/`DevServer` build their *Process records.
type PythonResolution struct {
	// Cmd is the absolute path of the binary to exec. e.g.
	// "/opt/homebrew/bin/uv" or "/path/to/.venv/bin/python".
	Cmd string

	// Args is the prefix that goes before manage.py in the final argv.
	// For uv: ["run", "--project", "<projectRoot>", "python"]
	// For venv: [] (Cmd already IS python)
	// For system python3: [] (Cmd is python3)
	Args []string

	// Strategy reports which path of the resolution order won. "uv" |
	// "venv" | "system". Surfaced for diagnostics; not consumed by callers.
	Strategy string

	// VenvDir is set when Strategy == "venv". The resolved venv directory.
	VenvDir string
}

// ResolvePython picks an invocation strategy for running Python in the
// consumer project. Order, per RUNTIMES.md §2.5:
//
//  1. `uv run --project <projectRoot> python …` — preferred when uv is on
//     PATH (handles transitive sync).
//  2. `<projectRoot>/.venv/bin/python …` — fall-back when a venv exists.
//  3. system `python3` — last resort.
//
// The uvProject argument is the value to pass to `uv --project`. Defaults
// to projectRoot when empty. Relative paths are resolved against
// projectRoot.
func ResolvePython(projectRoot, uvProject string) (PythonResolution, error) {
	// 1) uv if available
	if uv, err := exec.LookPath("uv"); err == nil {
		project := uvProject
		if project == "" {
			project = projectRoot
		} else if !filepath.IsAbs(project) {
			project = filepath.Join(projectRoot, project)
		}
		return PythonResolution{
			Cmd:      uv,
			Args:     []string{"run", "--project", project, "python"},
			Strategy: "uv",
		}, nil
	}

	// 2) venv at <projectRoot>/.venv/bin/python
	venv := filepath.Join(projectRoot, ".venv")
	venvPython := filepath.Join(venv, "bin", "python")
	if info, err := os.Stat(venvPython); err == nil && !info.IsDir() {
		return PythonResolution{
			Cmd:      venvPython,
			Strategy: "venv",
			VenvDir:  venv,
		}, nil
	}

	// 3) system python3
	if py3, err := exec.LookPath("python3"); err == nil {
		return PythonResolution{
			Cmd:      py3,
			Strategy: "system",
		}, nil
	}

	return PythonResolution{}, fmt.Errorf(
		"no Python interpreter found: uv not on PATH, no .venv/ at %s, "+
			"and python3 not on PATH",
		projectRoot,
	)
}
