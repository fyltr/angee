package projmode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPyProjectAngeeDev_parsesDjangoBlocks(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "pyproject.toml"), []byte(`
[project]
name = "consumer"

[tool.angee.dev.runtime.django]
manage = "src/manage.py"
bind   = "127.0.0.1:8100"
settings = "config.settings"

[tool.angee.dev.frontend]
cwd = "ui/react/web"

[tool.angee.dev.processes.celery]
cwd     = "src"
command = "celery"
args    = ["-A", "config", "worker", "-l", "info"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	dev, err := LoadPyProjectAngeeDev(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if dev == nil {
		t.Fatal("expected non-nil dev block")
	}
	if dev.Runtime == nil || dev.Runtime.Django == nil ||
		dev.Runtime.Django.Bind != "127.0.0.1:8100" {
		t.Fatalf("Django.Bind = %+v", dev.Runtime)
	}
	if dev.Frontend == nil || dev.Frontend.Cwd != "ui/react/web" {
		t.Fatalf("Frontend = %+v", dev.Frontend)
	}
	celery, ok := dev.Processes["celery"]
	if !ok || celery.Command != "celery" || len(celery.Args) != 5 {
		t.Fatalf("Processes[celery] = %+v", celery)
	}
}

func TestLoadPyProjectAngeeDev_missingFileReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	dev, err := LoadPyProjectAngeeDev(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if dev != nil {
		t.Fatalf("expected nil dev block, got %+v", dev)
	}
}

func TestLoadPyProjectAngeeDev_reservedNameRejected(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "pyproject.toml"), []byte(`
[tool.angee.dev.processes.runtime]
command = "echo"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPyProjectAngeeDev(tmp); err == nil {
		t.Fatal("expected error for reserved process name 'runtime'")
	}
}

func TestLoadPyProjectAngeeDev_missingCommandRejected(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "pyproject.toml"), []byte(`
[tool.angee.dev.processes.celery]
cwd = "src"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPyProjectAngeeDev(tmp); err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestValidProcessName(t *testing.T) {
	cases := map[string]bool{
		"celery":      true,
		"my-worker":   true,
		"a":           true,
		"a1":          true,
		"":            false,
		"1celery":     false,
		"-leading":    false,
		"Upper":       false,
		"with_under":  false,
		"with space":  false,
		"weird!chars": false,
	}
	for in, want := range cases {
		if got := validProcessName(in); got != want {
			t.Errorf("validProcessName(%q) = %v, want %v", in, got, want)
		}
	}
}
