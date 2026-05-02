// Package dev implements the `angee dev` orchestrator: parallel process
// supervision for the dev loop (build watcher + runtime dev server +
// frontend + any [tool.angee.dev.processes.*] extras).
//
// Per ARCHITECTURE.md §12.6 / RUNTIMES.md, the orchestrator is
// runtime-agnostic — it consumes *projmode.Process records returned by
// adapters and supervises them. No file watching of its own (django-angee
// owns `manage.py angee build --watch`); no template rendering (`internal/
// tmpl/`); no operator/compose work (`internal/runtime/compose/`).
package dev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fyltr/angee/internal/projmode"
)

// Plan is the resolved set of children to spawn. Built by `BuildPlan`
// from the adapter + manifest + pyproject; consumed by `Run`.
type Plan struct {
	Watcher  *projmode.Process // build --watch (long-running); may be nil
	Runtime  *projmode.Process // runserver (long-running); may be nil
	Frontend *projmode.Process // pnpm dev (long-running); may be nil
	Extras   []*projmode.Process // [tool.angee.dev.processes.*]
	Marker   string              // adapter.FirstCycleMarker()
}

// PlanOptions are the user-facing CLI flags that filter Plan.
type PlanOptions struct {
	Only       []string // --only=watcher,runtime,...
	Except     []string // --except=frontend,celery
	NoWatch    bool     // --no-watch
	NoFrontend bool     // --no-frontend
}

// BuildPlan composes a runnable Plan from an adapter + the dev context.
// Apply user filters via ApplyFilters before passing to Run.
func BuildPlan(adapter projmode.Adapter, ctx projmode.Ctx) Plan {
	plan := Plan{
		Watcher:  adapter.Watcher(ctx),
		Runtime:  adapter.DevServer(ctx),
		Frontend: adapter.Frontend(ctx),
		Marker:   adapter.FirstCycleMarker(),
	}
	if ctx.PyProject != nil {
		plan.Extras = extrasFromPyProject(ctx.PyProject, ctx.ProjectRoot)
	}
	return plan
}

// ApplyFilters drops children excluded by --only / --except / --no-watch
// / --no-frontend. Returns an error when --only references a name not in
// the plan (typo protection per the handoff §8).
func (p *Plan) ApplyFilters(opts PlanOptions) error {
	if opts.NoWatch {
		opts.Except = appendUnique(opts.Except, "build")
	}
	if opts.NoFrontend {
		opts.Except = appendUnique(opts.Except, "frontend")
	}

	all := p.allNames()
	if len(opts.Only) > 0 && len(opts.Except) > 0 {
		return errors.New("--only and --except are mutually exclusive")
	}
	for _, name := range opts.Only {
		if !contains(all, name) {
			return fmt.Errorf(
				"--only %q: unknown child (have: %s)",
				name, strings.Join(all, ", "),
			)
		}
	}
	for _, name := range opts.Except {
		if !contains(all, name) {
			return fmt.Errorf(
				"--except %q: unknown child (have: %s)",
				name, strings.Join(all, ", "),
			)
		}
	}

	keep := func(name string) bool {
		if len(opts.Only) > 0 {
			return contains(opts.Only, name)
		}
		return !contains(opts.Except, name)
	}
	if p.Watcher != nil && !keep("build") {
		p.Watcher = nil
	}
	if p.Runtime != nil && !keep("runtime") {
		p.Runtime = nil
	}
	if p.Frontend != nil && !keep("frontend") {
		p.Frontend = nil
	}
	pruned := make([]*projmode.Process, 0, len(p.Extras))
	for _, e := range p.Extras {
		if keep(e.Name) {
			pruned = append(pruned, e)
		}
	}
	p.Extras = pruned
	return nil
}

func (p Plan) allNames() []string {
	out := make([]string, 0, 4+len(p.Extras))
	if p.Watcher != nil {
		out = append(out, "build")
	}
	if p.Runtime != nil {
		out = append(out, "runtime")
	}
	if p.Frontend != nil {
		out = append(out, "frontend")
	}
	for _, e := range p.Extras {
		out = append(out, e.Name)
	}
	return out
}

// running wraps an *exec.Cmd we've started. The orchestrator owns the
// lifecycle: start → wait → terminate. PID + Cmd kept around for signal
// delivery to the process group.
type running struct {
	name string
	cmd  *exec.Cmd
	exit chan error // sends the Wait() error exactly once, then closes
}

// Run starts every child in the plan, waits for the watcher's first-cycle
// marker before starting the rest, and supervises until SIGINT/SIGTERM
// or any child exits. Returns nil on clean shutdown; the first non-zero
// child error otherwise.
//
// Behaviour locked by ARCHITECTURE.md §12.6:
//   - Watcher first; wait up to 60 s for first-cycle marker.
//   - On marker: start Runtime + Frontend + Extras (lex order, deterministic).
//   - SIGINT to orchestrator → SIGTERM to each child group → 10 s grace
//     → SIGKILL stragglers.
//   - Any child non-zero exit triggers orderly shutdown of remaining.
//
// Marker-wait timeout is generous because the first build can be slow
// (Python + Django boot, full pipeline). After the marker fires once,
// the rest of the loop is steady-state.
func Run(ctx context.Context, plan Plan, sink Sink) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var children []*running
	defer terminateAll(&children, sink)

	// 1. Watcher first. Tee its stdout through the first-cycle scanner so
	//    we can hold Runtime until the marker fires.
	if plan.Watcher != nil {
		w, scanner, err := startWatcher(plan.Watcher, sink, plan.Marker)
		if err != nil {
			return fmt.Errorf("starting watcher: %w", err)
		}
		children = append(children, w)
		sink.SystemLine("started [build] (waiting for: %q)", plan.Marker)

		// Wait for the marker (or watcher death) before continuing. This
		// is the only sequential step; everything after starts in parallel.
		const markerTimeout = 60 * time.Second
		if !scanner.Wait(ctx, markerTimeout) {
			return fmt.Errorf(
				"watcher never emitted first-cycle marker %q within %s "+
					"(check stderr for the build error)",
				plan.Marker, markerTimeout,
			)
		}
		sink.SystemLine("first-cycle marker received — starting siblings")
	}

	// 2. Runtime + Frontend + Extras in parallel.
	startSet := []*projmode.Process{}
	if plan.Runtime != nil {
		startSet = append(startSet, plan.Runtime)
	}
	if plan.Frontend != nil {
		startSet = append(startSet, plan.Frontend)
	}
	startSet = append(startSet, plan.Extras...)
	for _, p := range startSet {
		r, err := startOne(p, sink)
		if err != nil {
			return fmt.Errorf("starting %s: %w", p.Name, err)
		}
		children = append(children, r)
		sink.SystemLine("started [%s] pid=%d", r.name, r.cmd.Process.Pid)
	}

	// 3. Supervise: wait for ANY child exit OR a signal.
	sigC := make(chan os.Signal, 2)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigC)

	exitC := merge(children)

	select {
	case sig := <-sigC:
		sink.SystemLine("received %s — shutting down", sig)
		return nil
	case ev := <-exitC:
		if ev.err != nil {
			sink.SystemLine("[%s] exited: %v", ev.name, ev.err)
			return ev.err
		}
		sink.SystemLine("[%s] exited cleanly — shutting down", ev.name)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type childExit struct {
	name string
	err  error
}

// merge fans every child's exit channel into a single receive. Returns a
// channel that emits exactly one childExit (the first one); subsequent
// exits are observed during terminateAll's shutdown waits.
func merge(rs []*running) <-chan childExit {
	out := make(chan childExit, 1)
	var once sync.Once
	for _, r := range rs {
		go func(r *running) {
			err := <-r.exit
			once.Do(func() {
				out <- childExit{name: r.name, err: err}
			})
		}(r)
	}
	return out
}

// startWatcher spawns the watcher and tees its stdout through the
// first-cycle scanner. Stderr goes straight to the sink.
func startWatcher(
	p *projmode.Process,
	sink Sink,
	marker string,
) (*running, *firstCycleScanner, error) {
	cmd := buildCmd(p)
	stdoutR, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = sink.Writer(p.Name)

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	tee := sink.Writer(p.Name)
	scanner := newFirstCycleScanner(stdoutR, tee, marker)
	go scanner.run(context.Background())

	r := &running{name: p.Name, cmd: cmd, exit: make(chan error, 1)}
	go func() {
		r.exit <- cmd.Wait()
		close(r.exit)
	}()
	return r, scanner, nil
}

// startOne spawns a non-watcher child with stdout/stderr piped through
// the sink.
func startOne(p *projmode.Process, sink Sink) (*running, error) {
	cmd := buildCmd(p)
	cmd.Stdout = sink.Writer(p.Name)
	cmd.Stderr = sink.Writer(p.Name)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := &running{name: p.Name, cmd: cmd, exit: make(chan error, 1)}
	go func() {
		r.exit <- cmd.Wait()
		close(r.exit)
	}()
	return r, nil
}

// buildCmd applies the *Process record to an *exec.Cmd, putting the
// child in its own process group so SIGTERM can be delivered to the
// whole group on shutdown.
func buildCmd(p *projmode.Process) *exec.Cmd {
	cmd := exec.Command(p.Command, p.Args...)
	if p.Cwd != "" {
		cmd.Dir = p.Cwd
	}
	cmd.Env = mergeEnv(os.Environ(), p.Env)
	setSession(cmd) // POSIX setsid / Windows CREATE_NEW_PROCESS_GROUP
	return cmd
}

// terminateAll signals SIGTERM to every still-running child's process
// group, waits up to 10 s for clean exits, then SIGKILLs stragglers.
// Called from Run's deferred cleanup.
func terminateAll(rsp *[]*running, sink Sink) {
	rs := *rsp
	if len(rs) == 0 {
		return
	}
	for _, r := range rs {
		signalGroup(r, syscall.SIGTERM)
	}
	deadline := time.After(10 * time.Second)
	pending := make(map[string]*running, len(rs))
	for _, r := range rs {
		pending[r.name] = r
	}
	for len(pending) > 0 {
		select {
		case <-deadline:
			for name, r := range pending {
				sink.SystemLine("[%s] still running after 10s — SIGKILL", name)
				signalGroup(r, syscall.SIGKILL)
				<-r.exit
			}
			return
		default:
		}
		for name, r := range pending {
			select {
			case <-r.exit:
				delete(pending, name)
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
}

// mergeEnv overlays per-process env on top of inherited environment.
// Mirrors cli/project.go:mergeEnv but kept here so the dev package has
// no dep on cli/.
func mergeEnv(base []string, overlay map[string]string) []string {
	if len(overlay) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(overlay))
	seen := make(map[string]bool, len(overlay))
	for _, kv := range base {
		eq := strings.Index(kv, "=")
		if eq >= 0 {
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

// helpers — kept inline; they're trivial and used only here.

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func appendUnique(xs []string, s string) []string {
	if contains(xs, s) {
		return xs
	}
	return append(xs, s)
}

// piped is exported as no-op for tests that need to disable the orchestrator's
// realio.Discard usage; not actually used in production. Keeping the import
// graph honest.
var _ = io.Discard
