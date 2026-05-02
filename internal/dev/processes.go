package dev

import (
	"path/filepath"
	"sort"

	"github.com/fyltr/angee/internal/projmode"
)

// extrasFromPyProject converts the parsed `[tool.angee.dev.processes.*]`
// blocks into projmode.Process records the orchestrator can spawn.
//
// Order is lexicographic by name (deterministic — matches the start
// order documented in ARCHITECTURE.md §12.6 and the handoff §7.1).
//
// Cwd is resolved against projectRoot (relative paths join, absolute
// paths pass through, empty defaults to projectRoot).
func extrasFromPyProject(
	dev *projmode.PyProjectAngeeDev,
	projectRoot string,
) []*projmode.Process {
	if dev == nil || len(dev.Processes) == 0 {
		return nil
	}
	names := make([]string, 0, len(dev.Processes))
	for n := range dev.Processes {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]*projmode.Process, 0, len(names))
	for _, name := range names {
		p := dev.Processes[name]
		cwd := p.Cwd
		if cwd == "" {
			cwd = projectRoot
		} else if !filepath.IsAbs(cwd) {
			cwd = filepath.Join(projectRoot, cwd)
		}
		out = append(out, &projmode.Process{
			Name:    name,
			Cwd:     cwd,
			Command: p.Command,
			Args:    p.Args,
			Env:     p.Env,
		})
	}
	return out
}
