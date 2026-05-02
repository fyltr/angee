package projmode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindProjectRoot_walksUpToMarker(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "consumer")
	deep := filepath.Join(root, "ui", "react", "web")
	if err := os.MkdirAll(filepath.Join(root, ".angee"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".angee", "project.yaml"),
		[]byte("version: 1\nruntime: django-angee\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	got := FindProjectRoot(deep)
	want, _ := filepath.Abs(root)
	if got != want {
		t.Fatalf("FindProjectRoot(%q) = %q, want %q", deep, got, want)
	}
}

func TestFindProjectRoot_returnsEmptyWhenNoMarker(t *testing.T) {
	tmp := t.TempDir()
	if got := FindProjectRoot(tmp); got != "" {
		t.Fatalf("FindProjectRoot(%q) = %q, want empty", tmp, got)
	}
}

func TestFindProjectRoot_envOverride(t *testing.T) {
	t.Setenv("ANGEE_PROJECT_ROOT", "/somewhere/pinned")
	got := FindProjectRoot(t.TempDir())
	if got != "/somewhere/pinned" {
		t.Fatalf("env override not honoured: got %q", got)
	}
}

func TestLoadManifest_minimalDjango(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".angee"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `version: 1
runtime: django-angee
django:
  manage_py: src/manage.py
  invoker: uv
  uv:
    project: .
  settings: config.settings
`
	if err := os.WriteFile(
		filepath.Join(tmp, ".angee", "project.yaml"),
		[]byte(body), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if m.Runtime != "django-angee" {
		t.Fatalf("Runtime = %q", m.Runtime)
	}
	if m.Django == nil || m.Django.ManagePy != "src/manage.py" {
		t.Fatalf("Django.ManagePy = %+v", m.Django)
	}
	if m.Django.UV == nil || m.Django.UV.Project != "." {
		t.Fatalf("Django.UV = %+v", m.Django.UV)
	}
	if m.Django.Settings != "config.settings" {
		t.Fatalf("Django.Settings = %q", m.Django.Settings)
	}
}

func TestLoadManifest_missingRuntimeIsError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".angee"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(tmp, ".angee", "project.yaml"),
		[]byte("version: 1\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(tmp); err == nil {
		t.Fatal("expected error for missing runtime")
	}
}
