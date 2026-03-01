package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvBackend_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	b := NewEnvBackend(envFile)
	ctx := context.Background()

	// Set
	if err := b.Set(ctx, "db-password", "secret123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := b.Set(ctx, "api-key", "sk-ant-xxx"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get
	val, err := b.Get(ctx, "db-password")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "secret123" {
		t.Errorf("expected 'secret123', got %q", val)
	}

	// List
	names, err := b.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}

	// Delete
	if err := b.Delete(ctx, "db-password"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	names, _ = b.List(ctx)
	if len(names) != 1 {
		t.Errorf("expected 1 name after delete, got %d", len(names))
	}

	// Get deleted → error
	if _, err := b.Get(ctx, "db-password"); err == nil {
		t.Error("expected error for deleted secret")
	}
}

func TestEnvBackend_SecretToEnvKey(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"db-password", "DB_PASSWORD"},
		{"anthropic-api-key", "ANTHROPIC_API_KEY"},
		{"simple", "SIMPLE"},
	}
	for _, tt := range tests {
		got := secretToEnvKey(tt.name)
		if got != tt.want {
			t.Errorf("secretToEnvKey(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestEnvBackend_DollarEscaping(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	b := NewEnvBackend(envFile)
	ctx := context.Background()

	// Value with $ should be escaped in file but returned unescaped
	if err := b.Set(ctx, "test-key", "pa$$word"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Check raw file has $$$$
	data, _ := os.ReadFile(envFile)
	if !contains(string(data), "pa$$$$word") {
		t.Errorf("expected escaped dollars in file, got: %s", data)
	}

	// Get returns unescaped
	val, _ := b.Get(ctx, "test-key")
	if val != "pa$$word" {
		t.Errorf("expected 'pa$$word', got %q", val)
	}
}

func TestEnvBackend_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	b := NewEnvBackend(envFile)
	ctx := context.Background()

	// List on missing file
	names, err := b.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestEnvBackend_Type(t *testing.T) {
	b := NewEnvBackend("/tmp/.env")
	if b.Type() != "env" {
		t.Errorf("expected 'env', got %q", b.Type())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
