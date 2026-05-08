package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := NewRoot(&stdout, &stderr)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "angee version " + Version
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestInitDevReportsTemplateAndRoot(t *testing.T) {
	root := t.TempDir()
	writeStackTemplate(t, root)
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	cmd := NewRootWithIO(strings.NewReader("\n"), &stdout, &stderr)
	cmd.SetArgs([]string{"init", "--dev"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stderr.String(); !strings.Contains(got, "ANGEE_ROOT [.angee]:") {
		t.Fatalf("prompt = %q, want ANGEE_ROOT default prompt", got)
	}
	want := "stack template dev initialized as .angee"
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("init output = %q, want %q", got, want)
	}
}

func TestInitDevRefusesNonEmptyRoot(t *testing.T) {
	root := t.TempDir()
	writeStackTemplate(t, root)
	writeExistingStackRoot(t, root)
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	cmd := NewRoot(&stdout, &stderr)
	cmd.SetArgs([]string{"init", "--dev", "--yes"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error is nil")
	}
	want := "stack template dev already exists as .angee; use --force to overwrite or `angee stack update` to update"
	if got := err.Error(); got != want {
		t.Fatalf("init error = %q, want %q", got, want)
	}
}

func TestInitDevForceAllowsNonEmptyRoot(t *testing.T) {
	root := t.TempDir()
	writeStackTemplate(t, root)
	writeExistingStackRoot(t, root)
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	cmd := NewRoot(&stdout, &stderr)
	cmd.SetArgs([]string{"init", "--dev", "--force", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "stack template dev initialized as .angee"
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("init output = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, ".angee", "angee.yaml")); err != nil {
		t.Fatalf("Stat(angee.yaml) error = %v", err)
	}
}

func TestInitStackTemplateInitializesNamedRoot(t *testing.T) {
	root := t.TempDir()
	templateRoot := writeStackTemplate(t, root)
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	cmd := NewRoot(&stdout, &stderr)
	cmd.SetArgs([]string{"init", "stack", "--template", templateRoot, "angee-notes", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "angee-notes", "angee.yaml")); err != nil {
		t.Fatalf("Stat(angee-notes/angee.yaml) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "angee-notes", ".angee", "angee.yaml")); !os.IsNotExist(err) {
		t.Fatalf("unexpected nested .angee manifest err = %v", err)
	}
}

func writeStackTemplate(t *testing.T, root string) string {
	t.Helper()
	templateRoot := filepath.Join(root, ".templates", "stacks", "dev")
	manifestDir := filepath.Join(templateRoot, "template", "{{ ANGEE_ROOT }}")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(template) error = %v", err)
	}
	copierYAML := `_subdirectory: template
_templates_suffix: .jinja
_angee:
  kind: stack
  name: dev
ANGEE_ROOT:
  default: .angee
`
	if err := os.WriteFile(filepath.Join(templateRoot, "copier.yml"), []byte(copierYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(copier.yml) error = %v", err)
	}
	manifestYAML := `version: 1
kind: stack
name: test
`
	if err := os.WriteFile(filepath.Join(manifestDir, "angee.yaml.jinja"), []byte(manifestYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(angee.yaml.jinja) error = %v", err)
	}
	return templateRoot
}

func writeExistingStackRoot(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".angee"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.angee) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".angee", "existing"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}
}
