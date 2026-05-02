package django

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fyltr/angee/internal/projmode"
)

func mkCtx(projectRoot string) projmode.Ctx {
	return projmode.Ctx{
		ProjectRoot: projectRoot,
		Manifest: &projmode.Manifest{
			Runtime: Name,
			Django: &projmode.DjangoManifest{
				ManagePy: "src/manage.py",
				Settings: "config.settings",
			},
		},
		Python: projmode.PythonResolution{
			Cmd:      "/usr/bin/uv",
			Args:     []string{"run", "--project", projectRoot, "python"},
			Strategy: "uv",
		},
	}
}

func TestDispatch_buildArgs(t *testing.T) {
	a := New()
	root := "/tmp/consumer"
	ctx := mkCtx(root)

	p, err := a.Dispatch(ctx, "build", []string{"--check"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Command != "/usr/bin/uv" {
		t.Fatalf("Command = %q", p.Command)
	}
	want := []string{
		"run", "--project", root, "python",
		filepath.Join(root, "src", "manage.py"),
		"angee", "build", "--check",
	}
	if !slicesEqual(p.Args, want) {
		t.Fatalf("Args:\n got: %v\nwant: %v", p.Args, want)
	}
	if p.Cwd != root {
		t.Fatalf("Cwd = %q, want %q", p.Cwd, root)
	}
	if p.Env["DJANGO_SETTINGS_MODULE"] != "config.settings" {
		t.Fatalf("DJANGO_SETTINGS_MODULE not set: %v", p.Env)
	}
}

func TestDispatch_passesThroughExtraArgs(t *testing.T) {
	a := New()
	ctx := mkCtx("/r")
	p, err := a.Dispatch(ctx, "fixtures", []string{"load", "--package", "x"})
	if err != nil {
		t.Fatal(err)
	}
	tail := p.Args[len(p.Args)-5:]
	want := []string{"angee", "fixtures", "load", "--package", "x"}
	if !slicesEqual(tail, want) {
		t.Fatalf("tail: %v, want %v", tail, want)
	}
}

func TestDispatch_missingDjangoBlockIsError(t *testing.T) {
	a := New()
	ctx := mkCtx("/r")
	ctx.Manifest.Django = nil
	if _, err := a.Dispatch(ctx, "build", nil); err == nil {
		t.Fatal("expected error for missing django block")
	}
}

func TestDispatch_absoluteManagePyHonoured(t *testing.T) {
	a := New()
	ctx := mkCtx("/r")
	ctx.Manifest.Django.ManagePy = "/abs/elsewhere/manage.py"
	p, err := a.Dispatch(ctx, "doctor", nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(p.Args, " ")
	if !strings.Contains(joined, "/abs/elsewhere/manage.py") {
		t.Fatalf("absolute manage_py not honoured: %v", p.Args)
	}
}

func TestWatcher_addsWatchFlag(t *testing.T) {
	a := New()
	ctx := mkCtx("/r")
	p := a.Watcher(ctx)
	if p == nil {
		t.Fatal("Watcher returned nil")
	}
	if p.Name != "build" {
		t.Fatalf("Name = %q", p.Name)
	}
	if p.Args[len(p.Args)-1] != "--watch" {
		t.Fatalf("missing --watch in argv: %v", p.Args)
	}
}

func TestDevServer_usesBindFromPyProject(t *testing.T) {
	a := New()
	ctx := mkCtx("/r")
	ctx.PyProject = &projmode.PyProjectAngeeDev{
		Runtime: &projmode.PyProjectRuntime{
			Django: &projmode.PyProjectRuntimeDjango{Bind: "127.0.0.1:8100"},
		},
	}
	p := a.DevServer(ctx)
	if p == nil {
		t.Fatal("DevServer returned nil")
	}
	last := p.Args[len(p.Args)-2:]
	if last[0] != "runserver" || last[1] != "127.0.0.1:8100" {
		t.Fatalf("DevServer argv tail = %v, want [runserver 127.0.0.1:8100]", last)
	}
	if p.Name != "runtime" {
		t.Fatalf("Name = %q", p.Name)
	}
}

func TestDevServer_defaultBind(t *testing.T) {
	a := New()
	ctx := mkCtx("/r")
	p := a.DevServer(ctx)
	if p == nil {
		t.Fatal("DevServer returned nil")
	}
	last := p.Args[len(p.Args)-2:]
	if last[1] != "127.0.0.1:8000" {
		t.Fatalf("default bind = %q, want 127.0.0.1:8000", last[1])
	}
}

func TestFirstCycleMarker_locked(t *testing.T) {
	a := New()
	if a.FirstCycleMarker() != "angee build --watch: ready (cycle 1)" {
		t.Fatal("first-cycle marker text changed — coordinate with django-angee")
	}
}

func slicesEqual[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
