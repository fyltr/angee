// Package projmode is the runtime-adapter layer for project-mode invocations
// (when a `.angee/project.yaml` runtime manifest is found by parent-walk).
//
// Compose-mode (operator state in `~/.angee/`) is handled by sibling packages
// `internal/runtime/`, `internal/operator/`, etc. — projmode shares no code
// with those by design. The two modes coexist in a single binary; only the
// presence of `.angee/project.yaml` (vs `.angee/angee.yaml`) decides which
// applies on a given invocation.
//
// One concrete Adapter ships in v1: `internal/projmode/django/`. The
// interface is small enough that future Rust / Node adapters can plug in
// without touching the orchestrator (`internal/dev/`).
//
// See the design contract:
//   - docs/ARCHITECTURE.md §12 — Project Mode
//   - docs/RUNTIMES.md         — adapter-author guide
//   - django-angee R-15 / R-16
package projmode

// Process is everything `internal/dev/orchestrator` or the dispatcher needs
// to spawn one child: the resolved binary, its argv, working directory, and
// any environment overlay merged on top of the inherited env.
//
// Adapters return *Process records; they do NOT spawn subprocesses
// themselves. That keeps process supervision (signals, log prefixing, the
// pane TUI) entirely in `internal/dev/`.
type Process struct {
	// Name is one of "build" | "runtime" | "frontend" | a process key from
	// pyproject.toml [tool.angee.dev.processes.*]. Used as the prefix in
	// line-mode output and as the tab label in pane-mode TUI.
	Name string

	// Cwd is the absolute working directory for the child process. When
	// empty, the orchestrator inherits the orchestrator's cwd (typically
	// the consumer project root).
	Cwd string

	// Command is the resolved absolute path of the binary to exec
	// (e.g. /opt/homebrew/bin/uv). Adapters resolve this themselves so the
	// dispatcher does no further PATH lookup.
	Command string

	// Args is the argv slice AFTER Command (so Args[0] is NOT the binary
	// name; it's the first argument). Mirrors os/exec.Cmd.Args[1:].
	Args []string

	// Env is the per-process environment OVERLAY. The orchestrator merges
	// this on top of the inherited environment (the caller of `angee dev`
	// or `angee build`); keys here win on collision. nil means "inherit".
	Env map[string]string
}

// Ctx is what every Adapter method receives. Built once by the dispatcher
// at the start of each invocation; cheap to pass by value.
type Ctx struct {
	// ProjectRoot is the absolute path of the consumer project root —
	// the parent of the `.angee/` directory that contained `project.yaml`.
	ProjectRoot string

	// Manifest is the parsed `.angee/project.yaml`.
	Manifest *Manifest

	// PyProject is the parsed `[tool.angee.dev.*]` block from the
	// consumer's pyproject.toml. May be nil when the consumer hasn't
	// written that block yet (the orchestrator falls back to defaults).
	PyProject *PyProjectAngeeDev

	// Python is the pre-resolved Python invocation: the binary plus any
	// prefix args (e.g. `["run", "--project", ".", "python"]` for `uv`).
	// Only populated for python-flavoured runtimes.
	Python PythonResolution
}

// Adapter is the contract every framework runtime implements. Five methods,
// no more. Process supervision and template rendering are NOT the adapter's
// concern — see the package doc for the boundary rules.
type Adapter interface {
	// Name returns the runtime identifier as it appears in
	// `.angee/project.yaml`'s `runtime:` field — e.g. "django-angee".
	Name() string

	// Dispatch builds the *Process for a one-shot framework subcommand.
	// Used by `angee build`, `angee migrate`, `angee doctor`,
	// `angee fixtures`. The dispatcher syscall.Execs the returned Process
	// (stdio is shared with the parent so the user sees the framework's
	// output verbatim and signals + exit codes pass through cleanly).
	//
	// `sub` is the framework subcommand (e.g. "build"); `args` is what
	// followed it on the command line.
	Dispatch(ctx Ctx, sub string, args []string) (*Process, error)

	// Watcher returns the long-running build-watcher Process. For
	// django-angee: `uv run python src/manage.py angee build --watch`.
	// May return nil when the runtime has no watcher concept.
	Watcher(ctx Ctx) *Process

	// DevServer returns the long-running dev server Process. For
	// django-angee: `uv run python src/manage.py runserver <bind>`.
	DevServer(ctx Ctx) *Process

	// Frontend returns the long-running frontend dev server Process.
	// May return nil when the consumer has no frontend (the orchestrator
	// then runs without one — backend-only consumers are common).
	Frontend(ctx Ctx) *Process

	// FirstCycleMarker is the line the orchestrator scans for on
	// Watcher's stdout before starting DevServer (so runserver doesn't
	// import a stale runtime/). Locked for django-angee:
	//   "angee build --watch: ready (cycle 1)"
	// New adapters should pick a similarly-explicit marker so log noise
	// can't accidentally trigger.
	FirstCycleMarker() string
}
