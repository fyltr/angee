package copier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	content := `_min_copier_version: "9.0"
_subdirectory: template
_answers_file: .copier-answers.yml
_angee:
  schema: 1
  kind: stack
  name: dev
project_name:
  type: str
  default: demo
`
	if err := os.WriteFile(filepath.Join(dir, "copier.yml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Angee.Kind != "stack" || cfg.Angee.Name != "dev" {
		t.Fatalf("metadata = %#v", cfg.Angee)
	}
	if cfg.Subdirectory != "template" {
		t.Fatalf("subdirectory = %q", cfg.Subdirectory)
	}
}

func TestResolveRejectsWrongKind(t *testing.T) {
	dir := t.TempDir()
	content := `_angee:
  schema: 1
  kind: agent
  name: dev
`
	if err := os.WriteFile(filepath.Join(dir, "copier.yml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Resolve(dir, "stack", "dev")
	if err == nil {
		t.Fatal("expected kind mismatch error")
	}
}
