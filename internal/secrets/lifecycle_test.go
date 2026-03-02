package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/credentials/openbao"
)

// newBaoTestServer simulates OpenBao KV v2 for testing.
func newBaoTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store := make(map[string]string)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1/")

		// Health check — no auth required
		if r.Method == http.MethodGet && path == "sys/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"initialized":true,"sealed":false}`))
			return
		}

		if r.Header.Get("X-Vault-Token") != "test-token" {
			http.Error(w, `{"errors":["permission denied"]}`, http.StatusForbidden)
			return
		}

		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(path, "secret/data/"):
			val, ok := store[path]
			if !ok {
				http.Error(w, `{"errors":[]}`, http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"data": map[string]any{"value": val},
				},
			})

		case r.Method == http.MethodPost && strings.HasPrefix(path, "secret/data/"):
			var body struct {
				Data map[string]any `json:"data"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			val, _ := body.Data["value"].(string)
			store[path] = val
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})

		case r.Method == http.MethodDelete && strings.HasPrefix(path, "secret/metadata/"):
			dataPath := strings.Replace(path, "secret/metadata/", "secret/data/", 1)
			delete(store, dataPath)
			w.WriteHeader(http.StatusNoContent)

		case r.Method == "LIST" && strings.HasPrefix(path, "secret/metadata/"):
			prefix := strings.Replace(path, "secret/metadata/", "secret/data/", 1) + "/"
			var keys []string
			for k := range store {
				if strings.HasPrefix(k, prefix) {
					name := strings.TrimPrefix(k, prefix)
					if !strings.Contains(name, "/") {
						keys = append(keys, name)
					}
				}
			}
			if len(keys) == 0 {
				http.Error(w, `{"errors":[]}`, http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"keys": keys},
			})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func newTestBaoBackend(t *testing.T, srv *httptest.Server) *openbao.Backend {
	t.Helper()
	t.Setenv("TEST_BAO_TOKEN", "test-token")
	cfg := &config.OpenBaoConfig{
		Address: srv.URL,
		Auth: config.OpenBaoAuthConfig{
			Method:   "token",
			TokenEnv: "TEST_BAO_TOKEN",
		},
	}
	b := openbao.TryNew(cfg, "dev")
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
	return b
}

func TestStoreSecretFallbackToEnv(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// No OpenBao configured → falls through to .env
	cfg := &config.AngeeConfig{Name: "test"}
	backend, err := StoreSecret(ctx, cfg, dir, "db-password", "s3cret")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}
	if backend != "env" {
		t.Errorf("expected backend 'env', got %q", backend)
	}

	// Verify .env has it
	data, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if !strings.Contains(string(data), "DB_PASSWORD") {
		t.Errorf("expected DB_PASSWORD in .env, got: %s", data)
	}
}

func TestStoreSecretToOpenBao(t *testing.T) {
	srv := newBaoTestServer(t)
	defer srv.Close()

	dir := t.TempDir()
	ctx := context.Background()

	t.Setenv("TEST_BAO_STORE", "test-token")
	cfg := &config.AngeeConfig{
		Name: "test",
		SecretsBackend: &config.SecretsBackendConfig{
			Type: "openbao",
			OpenBao: &config.OpenBaoConfig{
				Address: srv.URL,
				Auth: config.OpenBaoAuthConfig{
					Method:   "token",
					TokenEnv: "TEST_BAO_STORE",
				},
			},
		},
	}

	backend, err := StoreSecret(ctx, cfg, dir, "api-key", "sk-123")
	if err != nil {
		t.Fatalf("StoreSecret: %v", err)
	}
	if backend != "openbao" {
		t.Errorf("expected backend 'openbao', got %q", backend)
	}
}

func TestSeedOpenBao(t *testing.T) {
	srv := newBaoTestServer(t)
	defer srv.Close()
	bao := newTestBaoBackend(t, srv)
	ctx := context.Background()

	// Write a .env file
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("DB_PASSWORD=s3cret\nAPI_KEY=sk-123\n"), 0600)

	seeded, err := SeedOpenBao(ctx, bao, envPath)
	if err != nil {
		t.Fatalf("SeedOpenBao: %v", err)
	}
	if seeded != 2 {
		t.Errorf("expected 2 seeded, got %d", seeded)
	}

	// Verify secrets are in OpenBao
	val, err := bao.Get(ctx, "db-password")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "s3cret" {
		t.Errorf("expected 's3cret', got %q", val)
	}

	// Seed again → should not overwrite
	seeded2, err := SeedOpenBao(ctx, bao, envPath)
	if err != nil {
		t.Fatalf("SeedOpenBao 2nd: %v", err)
	}
	if seeded2 != 0 {
		t.Errorf("expected 0 seeded on re-run, got %d", seeded2)
	}
}

func TestSeedOpenBaoMissingFile(t *testing.T) {
	srv := newBaoTestServer(t)
	defer srv.Close()
	bao := newTestBaoBackend(t, srv)
	ctx := context.Background()

	seeded, err := SeedOpenBao(ctx, bao, "/nonexistent/.env")
	if err != nil {
		t.Fatalf("SeedOpenBao: %v", err)
	}
	if seeded != 0 {
		t.Errorf("expected 0 seeded, got %d", seeded)
	}
}

func TestPullToEnvFile(t *testing.T) {
	srv := newBaoTestServer(t)
	defer srv.Close()
	bao := newTestBaoBackend(t, srv)
	ctx := context.Background()

	// Pre-populate OpenBao
	bao.Set(ctx, "db-password", "s3cret")
	bao.Set(ctx, "api-key", "sk-123")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	count, err := PullToEnvFile(ctx, bao, envPath)
	if err != nil {
		t.Fatalf("PullToEnvFile: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	data, _ := os.ReadFile(envPath)
	content := string(data)
	if !strings.Contains(content, "AUTO-GENERATED by angee up") {
		t.Error("expected auto-generated header")
	}
	if !strings.Contains(content, "DB_PASSWORD=s3cret") {
		t.Errorf("expected DB_PASSWORD in .env, got: %s", content)
	}
	if !strings.Contains(content, "API_KEY=sk-123") {
		t.Errorf("expected API_KEY in .env, got: %s", content)
	}
}

func TestPullToEnvFileEmpty(t *testing.T) {
	srv := newBaoTestServer(t)
	defer srv.Close()
	bao := newTestBaoBackend(t, srv)
	ctx := context.Background()

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	count, err := PullToEnvFile(ctx, bao, envPath)
	if err != nil {
		t.Fatalf("PullToEnvFile: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestPullToEnvFileDollarEscaping(t *testing.T) {
	srv := newBaoTestServer(t)
	defer srv.Close()
	bao := newTestBaoBackend(t, srv)
	ctx := context.Background()

	bao.Set(ctx, "test-key", "pa$$word")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	_, err := PullToEnvFile(ctx, bao, envPath)
	if err != nil {
		t.Fatalf("PullToEnvFile: %v", err)
	}

	data, _ := os.ReadFile(envPath)
	// $ should be escaped as $$ in the .env file
	if !strings.Contains(string(data), "pa$$$$word") {
		t.Errorf("expected escaped dollars, got: %s", data)
	}
}

func TestEnvKeyToSecret(t *testing.T) {
	tests := []struct{ in, want string }{
		{"DB_PASSWORD", "db-password"},
		{"ANTHROPIC_API_KEY", "anthropic-api-key"},
		{"SIMPLE", "simple"},
	}
	for _, tt := range tests {
		got := envKeyToSecret(tt.in)
		if got != tt.want {
			t.Errorf("envKeyToSecret(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSecretToEnvKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"db-password", "DB_PASSWORD"},
		{"anthropic-api-key", "ANTHROPIC_API_KEY"},
	}
	for _, tt := range tests {
		got := secretToEnvKey(tt.in)
		if got != tt.want {
			t.Errorf("secretToEnvKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
