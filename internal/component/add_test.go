package component

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasComponentFile(t *testing.T) {
	dir := t.TempDir()

	// No component file
	if hasComponentFile(dir) {
		t.Error("expected false for empty dir")
	}

	// angee-component.yaml
	os.WriteFile(filepath.Join(dir, "angee-component.yaml"), []byte("name: test"), 0644)
	if !hasComponentFile(dir) {
		t.Error("expected true for dir with angee-component.yaml")
	}

	// Clean and test component.yaml
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, "component.yaml"), []byte("name: test"), 0644)
	if !hasComponentFile(dir2) {
		t.Error("expected true for dir with component.yaml")
	}
}

func TestResolveComponentFile(t *testing.T) {
	// Prefer angee-component.yaml over component.yaml
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "angee-component.yaml"), []byte("name: test"), 0644)
	os.WriteFile(filepath.Join(dir, "component.yaml"), []byte("name: test-alt"), 0644)

	got := ResolveComponentFile(dir)
	want := filepath.Join(dir, "angee-component.yaml")
	if got != want {
		t.Errorf("ResolveComponentFile() = %q, want %q", got, want)
	}

	// Falls back to component.yaml
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, "component.yaml"), []byte("name: test"), 0644)

	got2 := ResolveComponentFile(dir2)
	want2 := filepath.Join(dir2, "component.yaml")
	if got2 != want2 {
		t.Errorf("ResolveComponentFile() = %q, want %q", got2, want2)
	}
}

func TestFetchComponentLocalDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "component.yaml"), []byte("name: test\nservices:\n  test:\n    image: test:latest\n"), 0644)

	got, cleanup, err := FetchComponent(dir)
	if err != nil {
		t.Fatalf("FetchComponent() error: %v", err)
	}
	if cleanup != "" {
		t.Error("expected no cleanup dir for local path")
	}
	if got != dir {
		t.Errorf("FetchComponent() = %q, want %q", got, dir)
	}
}

func TestFetchComponentLocalDirWithAngeeComponent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "angee-component.yaml"), []byte("name: test\nservices:\n  test:\n    image: test:latest\n"), 0644)

	got, cleanup, err := FetchComponent(dir)
	if err != nil {
		t.Fatalf("FetchComponent() error: %v", err)
	}
	if cleanup != "" {
		t.Error("expected no cleanup dir for local path")
	}
	if got != dir {
		t.Errorf("FetchComponent() = %q, want %q", got, dir)
	}
}

func TestResolveEmbeddedComponent(t *testing.T) {
	// Create a fake embedded components dir
	dir := t.TempDir()
	compDir := filepath.Join(dir, "postgres")
	os.MkdirAll(compDir, 0755)
	os.WriteFile(filepath.Join(compDir, "component.yaml"), []byte("name: postgres"), 0644)

	// Set env var
	os.Setenv("ANGEE_COMPONENTS_PATH", dir)
	defer os.Unsetenv("ANGEE_COMPONENTS_PATH")

	got, ok := resolveEmbeddedComponent("postgres")
	if !ok {
		t.Fatal("expected to find embedded component")
	}
	if got != compDir {
		t.Errorf("resolveEmbeddedComponent() = %q, want %q", got, compDir)
	}

	// Non-existent component
	_, ok2 := resolveEmbeddedComponent("nonexistent")
	if ok2 {
		t.Error("expected false for nonexistent component")
	}
}
