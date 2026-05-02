// Package django is the runtime adapter for `django-angee` consumers.
//
// One of these per language framework. The interface (`projmode.Adapter`)
// is small enough that implementing it is mechanical; the real work is
// the resolution policy in `Build*` and the env-var handling.
//
// See:
//   - ../adapter.go         — the interface this implements
//   - ../../docs/RUNTIMES.md — adapter-author guide
package django

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/fyltr/angee/internal/projmode"
)

// Name is the runtime identifier matching `runtime: django-angee` in
// `.angee/project.yaml`.
const Name = "django-angee"

// Adapter implements projmode.Adapter for Django + django-angee consumers.
type Adapter struct{}

// New returns a freshly constructed adapter. Stateless; safe to share.
func New() *Adapter { return &Adapter{} }

// Name returns the runtime identifier.
func (*Adapter) Name() string { return Name }

// FirstCycleMarker is locked across the django-angee + angee-go boundary.
// `packages/django-angee/angee/management/commands/angee.py` emits this
// exact string on its first successful watch cycle. Don't change without
// coordinating both sides.
func (*Adapter) FirstCycleMarker() string {
	return "angee build --watch: ready (cycle 1)"
}

// Dispatch builds the *Process for `angee {build,migrate,doctor,fixtures}`
// invocations. Result: `<python> [<py-args>] <manage.py> angee <sub> <args…>`.
func (a *Adapter) Dispatch(
	ctx projmode.Ctx,
	sub string,
	args []string,
) (*projmode.Process, error) {
	dj := ctx.Manifest.Django
	if dj == nil {
		return nil, fmt.Errorf(
			"manifest is missing the `django:` block (required for " +
				"runtime: django-angee)",
		)
	}
	if dj.ManagePy == "" {
		return nil, fmt.Errorf("manifest.django.manage_py is required")
	}
	managePy := absPath(ctx.ProjectRoot, dj.ManagePy)
	cwd := absPath(ctx.ProjectRoot, dj.Cwd)
	if cwd == "" {
		cwd = ctx.ProjectRoot
	}

	argv := append([]string{}, ctx.Python.Args...)
	argv = append(argv, managePy, "angee", sub)
	argv = append(argv, args...)

	return &projmode.Process{
		Name:    sub,
		Cwd:     cwd,
		Command: ctx.Python.Cmd,
		Args:    argv,
		Env:     djangoEnv(ctx),
	}, nil
}

// Watcher returns the long-running `angee build --watch` process.
func (a *Adapter) Watcher(ctx projmode.Ctx) *projmode.Process {
	p, err := a.Dispatch(ctx, "build", []string{"--watch"})
	if err != nil {
		return nil
	}
	p.Name = "build"
	return p
}

// DevServer returns the long-running `runserver` process. The bind address
// comes from `[tool.angee.dev.runtime.django] bind` (default 127.0.0.1:8000).
//
// We do NOT funnel runserver through `manage.py angee`: it's the
// stock-Django subcommand. So the argv is built inline rather than via
// Dispatch (which always inserts `angee` between manage.py and the
// subcommand name).
func (a *Adapter) DevServer(ctx projmode.Ctx) *projmode.Process {
	dj := ctx.Manifest.Django
	if dj == nil || dj.ManagePy == "" {
		return nil
	}
	bind := "127.0.0.1:8000"
	if ctx.PyProject != nil &&
		ctx.PyProject.Runtime != nil &&
		ctx.PyProject.Runtime.Django != nil &&
		ctx.PyProject.Runtime.Django.Bind != "" {
		bind = ctx.PyProject.Runtime.Django.Bind
	}
	managePy := absPath(ctx.ProjectRoot, dj.ManagePy)
	cwd := absPath(ctx.ProjectRoot, dj.Cwd)
	if cwd == "" {
		cwd = ctx.ProjectRoot
	}
	argv := append([]string{}, ctx.Python.Args...)
	argv = append(argv, managePy, "runserver", bind)
	return &projmode.Process{
		Name:    "runtime",
		Cwd:     cwd,
		Command: ctx.Python.Cmd,
		Args:    argv,
		Env:     djangoEnv(ctx),
	}
}

// Frontend returns the long-running frontend dev process, or nil if no
// frontend is configured / detected. Resolution:
//
//  1. Explicit override via `[tool.angee.dev.frontend]`.
//  2. Auto-detect: a `package.json` with a `dev` script under one of
//     `ui/react/web/`, `frontend/`, `web/`, the project root.
//  3. If neither matches: nil (no frontend; orchestrator just runs
//     watcher + runserver + extras).
//
// Package manager: `pnpm` if a pnpm-lock.yaml is present in the chosen
// cwd, else `npm` if `package-lock.json`, else `yarn` if `yarn.lock`,
// else first of pnpm/npm/yarn on PATH. Args default to ["dev"].
func (a *Adapter) Frontend(ctx projmode.Ctx) *projmode.Process {
	cwd, ok := resolveFrontendCwd(ctx)
	if !ok {
		return nil
	}
	cmd, args := resolveFrontendCommand(ctx, cwd)
	if cmd == "" {
		return nil
	}
	return &projmode.Process{
		Name:    "frontend",
		Cwd:     cwd,
		Command: cmd,
		Args:    args,
	}
}

// resolveFrontendCwd applies the override → auto-detect → not-found
// resolution order described on Frontend. Returns "", false when no
// frontend is configured.
func resolveFrontendCwd(ctx projmode.Ctx) (string, bool) {
	if ctx.PyProject != nil && ctx.PyProject.Frontend != nil &&
		ctx.PyProject.Frontend.Cwd != "" {
		dir := absPath(ctx.ProjectRoot, ctx.PyProject.Frontend.Cwd)
		if hasPackageJSON(dir) {
			return dir, true
		}
	}
	for _, sub := range []string{
		"ui/react/web", "frontend", "web", ".",
	} {
		dir := absPath(ctx.ProjectRoot, sub)
		if hasPackageJSON(dir) {
			return dir, true
		}
	}
	return "", false
}

// resolveFrontendCommand picks pnpm / npm / yarn based on lock-file
// presence (preferred) or PATH lookup (fallback). Returns "", nil when
// none is available.
func resolveFrontendCommand(
	ctx projmode.Ctx,
	cwd string,
) (string, []string) {
	args := []string{"dev"}
	if ctx.PyProject != nil && ctx.PyProject.Frontend != nil {
		fe := ctx.PyProject.Frontend
		if len(fe.Args) > 0 {
			args = fe.Args
		}
		if fe.Command != "" {
			if path, err := exec.LookPath(fe.Command); err == nil {
				return path, args
			}
		}
	}
	// Lock-file based detection (which package manager the project uses).
	if hasFile(cwd, "pnpm-lock.yaml") {
		if pnpm, err := exec.LookPath("pnpm"); err == nil {
			return pnpm, args
		}
	}
	if hasFile(cwd, "package-lock.json") {
		if npm, err := exec.LookPath("npm"); err == nil {
			return npm, args
		}
	}
	if hasFile(cwd, "yarn.lock") {
		if yarn, err := exec.LookPath("yarn"); err == nil {
			return yarn, args
		}
	}
	// Fall back to first available manager on PATH.
	for _, name := range []string{"pnpm", "npm", "yarn"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, args
		}
	}
	return "", nil
}

// djangoEnv builds the env overlay for any Django child.
//
// PYTHONUNBUFFERED=1 is critical: when Python's stdout is a pipe (not a
// TTY) it block-buffers by default, which delays the build watcher's
// first-cycle marker by up to a buffer-flush worth of output. The
// orchestrator scans line-by-line and assumes prompt delivery.
//
// DJANGO_SETTINGS_MODULE is set from the manifest when declared.
func djangoEnv(ctx projmode.Ctx) map[string]string {
	env := map[string]string{
		"PYTHONUNBUFFERED": "1",
	}
	if ctx.Manifest != nil && ctx.Manifest.Django != nil &&
		ctx.Manifest.Django.Settings != "" {
		env["DJANGO_SETTINGS_MODULE"] = ctx.Manifest.Django.Settings
	}
	return env
}

// absPath joins p onto base when p is relative; returns p unchanged when
// absolute. "" → "" so callers can distinguish "not set".
func absPath(base, p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

func hasPackageJSON(dir string) bool { return hasFile(dir, "package.json") }

func hasFile(dir, name string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !info.IsDir()
}
