# Runtime Adapters

> **Status:** v1 ships one adapter (`django-angee`). Rust and Node are designed for, not built for.
> **Source:** `internal/runtime/`. The interface is intentionally small.
> **Upstream spec:** django-angee [R-14](../../django-angee/docs/DECISIONS.md#r-14), [R-15](../../django-angee/docs/DECISIONS.md#r-15), [R-16](../../django-angee/docs/DECISIONS.md#r-16).

A *runtime adapter* tells the `angee` Go CLI how to run a particular kind of consumer project — Django today, Rust and Node later. The CLI itself is runtime-agnostic; everything language-specific is encapsulated behind one interface.

---

## 1. The interface

```go
// internal/projmode/adapter.go

type Process struct {
    Name    string                       // "build" | "runtime" | "frontend" | <process key>
    Cwd     string                       // absolute, resolved by adapter
    Command string                       // resolved binary (e.g. /opt/homebrew/bin/uv)
    Args    []string                     // full argv after Command
    Env     map[string]string            // merged on top of inherited env
}

type Ctx struct {
    ProjectRoot string                   // parent of .angee/
    Manifest    *ProjectManifest         // parsed .angee/project.yaml
    PyProject   *PyProjectAngeeDev       // parsed [tool.angee.dev.*]; may be nil
    Python      PythonResolution         // for python-flavoured runtimes
}

type Adapter interface {
    Name() string                                  // "django-angee" | "rust" | "node"

    // Non-dev dispatch: how to run a one-shot framework subcommand.
    // Used by `angee build`, `angee migrate`, `angee doctor`, `angee fixtures`.
    // Returns full Process so the dispatcher can syscall.Exec with the
    // right working dir + DJANGO_SETTINGS_MODULE + env-var overrides
    // forwarded to the child.
    Dispatch(ctx Ctx, sub string, args []string) (*Process, error)

    // Three long-running children for `angee dev`. Each may be nil.
    // Extras come from pyproject.toml [tool.angee.dev.processes.*]
    // and are not the adapter's concern.
    Watcher(ctx Ctx)    *Process
    DevServer(ctx Ctx)  *Process
    Frontend(ctx Ctx)   *Process

    // FirstCycleMarker is the line the orchestrator waits for on Watcher's
    // stdout before starting DevServer. Locked for django-angee:
    // "angee build --watch: ready (cycle 1)".
    FirstCycleMarker() string
}
```

That's it — five methods. Everything else (process supervision, signal handling, log prefixing, pane-mode TUI) lives in `internal/dev/` and consumes the `*Process` returns.

---

## 2. Worked example: `django-angee`

The concrete v1 implementation in `internal/projmode/django/adapter.go` is small enough to read whole. The relevant choices:

### 2.1 `Dispatch`

Builds an argv that ends in `<projectRoot>/<manage_py> angee <subcmd> <args…>`. Prepends:

- `Command`: resolved `uv` (preferred) or `python` from venv or `python3`.
- `Args`: when using `uv`, prefix with `["run", "--project", uv.project, "python"]`.

```go
func (d *DjangoAngee) Dispatch(ctx Ctx, sub string, args []string) (*Process, error) {
    py := ctx.Python                                      // pre-resolved by caller
    managePy := filepath.Join(ctx.ProjectRoot, ctx.Manifest.Django.ManagePy)

    argv := append([]string{}, py.Args...)                // e.g. ["run", "--project", ".", "python"]
    argv = append(argv, managePy, "angee", sub)
    argv = append(argv, args...)

    return &Process{
        Name:    sub,
        Cwd:     ctx.ProjectRoot,
        Command: py.Cmd,
        Args:    argv,
        Env:     djangoEnv(ctx),
    }, nil
}
```

### 2.2 `Watcher` + `DevServer` + `Frontend`

Same shape as `Dispatch` but with hard-coded subcommands:

```go
func (d *DjangoAngee) Watcher(ctx Ctx) *Process {
    return d.spawn(ctx, "build", []string{"build", "--watch"})
}

func (d *DjangoAngee) DevServer(ctx Ctx) *Process {
    bind := ctx.PyProject.RuntimeDjango.Bind
    if bind == "" { bind = "127.0.0.1:8000" }
    p, _ := d.Dispatch(ctx, "", []string{}) // not used as-is — uses runserver
    p.Args = append(p.Args[:len(p.Args)-2],   // drop trailing "angee" "" 
                    "runserver", bind)
    p.Name = "runtime"
    return p
}

func (d *DjangoAngee) Frontend(ctx Ctx) *Process {
    feCfg := ctx.PyProject.Frontend            // nil-safe in real code
    cwd := filepath.Join(ctx.ProjectRoot, feCfg.Cwd)
    cmd, args := detectPackageManager(cwd, feCfg.Command, feCfg.Args)
    return &Process{Name: "frontend", Cwd: cwd, Command: cmd, Args: args, Env: nil}
}
```

### 2.3 `FirstCycleMarker`

```go
func (d *DjangoAngee) FirstCycleMarker() string {
    return "angee build --watch: ready (cycle 1)"
}
```

This string is locked across the django-angee + angee-go boundary. Don't change without coordinating with `packages/django-angee/angee/management/commands/angee.py`.

### 2.4 Python resolution helper

`internal/projmode/python.go` resolves `uv | venv | python3` once per invocation (cached) and exposes the result via `PythonResolution`:

```go
type PythonResolution struct {
    Cmd        string             // absolute path to executable
    Args       []string           // prefix args (e.g. ["run", "--project", "."])
    VenvDir    string             // when fall-back venv was used
    UsedFallback bool             // true when neither uv nor a venv was found
}
```

Order: **`uv run --project <consumerRoot>`** → **`<consumerRoot>/.venv/bin/python`** → **system `python3`**.

---

## 3. Adding a new adapter

For a new runtime (Rust, Node, or anything else):

1. Create `internal/projmode/<name>/adapter.go`.
2. Implement the five methods above.
3. Define how the consumer declares dev-time config — typically a runtime-native file (Rust: `Cargo.toml [package.metadata.angee.dev]`, Node: `package.json` `"angee.dev"` key). Parse it once at adapter construction; expose via the `Ctx` struct.
4. Register in `internal/projmode/registry.go` keyed on the `runtime:` string from `.angee/project.yaml`.
5. Add detection to `findProjectRoot()` if a different marker file is preferred (default: `.angee/project.yaml` works for all runtimes).

The orchestrator (`internal/dev/orchestrator.go`) needs zero changes — it operates on `*Process` returned from the adapter methods.

---

## 4. What an adapter does NOT do

Crisp boundaries help future adapters stay small.

- **No process supervision.** Spawning, signals, log prefixing, pane TUI — all in `internal/dev/`. Adapters just return `*Process` records.
- **No file watching.** Watchers are runtime-side subprocesses (`manage.py angee build --watch` for Django; `cargo watch` would be the Rust equivalent). The Go orchestrator is "dumb" — it spawns and waits for the marker.
- **No template rendering.** That's `internal/tmpl/` (consumed by `cli/init.go`). Adapters only operate on already-rendered project state.
- **No operator/compose work.** That's `internal/runtime/compose/` (RuntimeBackend) and `internal/operator/`. Project-mode lives in a sibling package (`internal/projmode/`) that shares no internal code with compose-mode.

---

## 5. Cross-references

- Architecture: [`ARCHITECTURE.md` §12](./ARCHITECTURE.md#12-project-mode).
- Templates: [`TEMPLATES.md`](./TEMPLATES.md) — including the `runtime` and `fixtures` keys for `.angee-template.yaml`.
- Upstream contract: django-angee [R-14](../../django-angee/docs/DECISIONS.md#r-14) / [R-15](../../django-angee/docs/DECISIONS.md#r-15) / [R-16](../../django-angee/docs/DECISIONS.md#r-16).
- Decisions-taken addendum on the original brief: django-angee `docs/handoff/angee-go-dev-subcommand.md` §13.
