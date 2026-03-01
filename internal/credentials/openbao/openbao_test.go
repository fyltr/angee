package openbao

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fyltr/angee/internal/config"
)

// newTestServer returns an httptest.Server that simulates OpenBao KV v2.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store := make(map[string]string) // path → value

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Require token
		if r.Header.Get("X-Vault-Token") != "test-token" {
			http.Error(w, `{"errors":["permission denied"]}`, http.StatusForbidden)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/v1/")

		switch {
		// KV v2 read: GET secret/data/...
		case r.Method == http.MethodGet && strings.HasPrefix(path, "secret/data/"):
			val, ok := store[path]
			if !ok {
				http.Error(w, `{"errors":[]}`, http.StatusNotFound)
				return
			}
			resp := map[string]any{
				"data": map[string]any{
					"data": map[string]any{"value": val},
				},
			}
			json.NewEncoder(w).Encode(resp)

		// KV v2 write: POST secret/data/...
		case r.Method == http.MethodPost && strings.HasPrefix(path, "secret/data/"):
			var body struct {
				Data map[string]any `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			val, _ := body.Data["value"].(string)
			store[path] = val
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})

		// KV v2 delete: DELETE secret/metadata/...
		case r.Method == http.MethodDelete && strings.HasPrefix(path, "secret/metadata/"):
			// Convert metadata path → data path for store cleanup
			dataPath := strings.Replace(path, "secret/metadata/", "secret/data/", 1)
			delete(store, dataPath)
			w.WriteHeader(http.StatusNoContent)

		// KV v2 list: LIST secret/metadata/...
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
			resp := map[string]any{
				"data": map[string]any{"keys": keys},
			}
			json.NewEncoder(w).Encode(resp)

		// AppRole login
		case r.Method == http.MethodPost && path == "auth/approle/login":
			var body struct {
				RoleID   string `json:"role_id"`
				SecretID string `json:"secret_id"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.RoleID == "valid-role" && body.SecretID == "valid-secret" {
				json.NewEncoder(w).Encode(map[string]any{
					"auth": map[string]any{"client_token": "approle-token"},
				})
			} else {
				http.Error(w, `{"errors":["invalid credentials"]}`, http.StatusForbidden)
			}

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func newTestBackend(t *testing.T, srv *httptest.Server) *Backend {
	t.Helper()
	return &Backend{
		addr:   srv.URL,
		token:  "test-token",
		prefix: "angee",
		env:    "dev",
	}
}

func TestType(t *testing.T) {
	b := &Backend{}
	if b.Type() != "openbao" {
		t.Errorf("Type() = %q, want %q", b.Type(), "openbao")
	}
}

func TestPathConstruction(t *testing.T) {
	b := &Backend{prefix: "angee", env: "dev"}

	if got := b.dataPath("db-password"); got != "secret/data/angee/dev/db-password" {
		t.Errorf("dataPath = %q", got)
	}
	if got := b.metadataPath("db-password"); got != "secret/metadata/angee/dev/db-password" {
		t.Errorf("metadataPath = %q", got)
	}
	if got := b.listPath(); got != "secret/metadata/angee/dev" {
		t.Errorf("listPath = %q", got)
	}
}

func TestCRUD(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	b := newTestBackend(t, srv)
	ctx := context.Background()

	// Set
	if err := b.Set(ctx, "db-password", "s3cret"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get
	val, err := b.Get(ctx, "db-password")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "s3cret" {
		t.Errorf("Get = %q, want %q", val, "s3cret")
	}

	// List
	keys, err := b.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 || keys[0] != "db-password" {
		t.Errorf("List = %v, want [db-password]", keys)
	}

	// Delete
	if err := b.Delete(ctx, "db-password"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after delete should fail
	_, err = b.Get(ctx, "db-password")
	if err == nil {
		t.Fatal("expected error after delete")
	}

	// List after delete should be empty
	keys, err = b.List(ctx)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("List after delete = %v, want empty", keys)
	}
}

func TestGetNotFound(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	b := newTestBackend(t, srv)

	_, err := b.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestNewTokenAuth(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	t.Setenv("TEST_BAO_TOKEN", "test-token")

	cfg := &config.OpenBaoConfig{
		Address: srv.URL,
		Auth: config.OpenBaoAuthConfig{
			Method:   "token",
			TokenEnv: "TEST_BAO_TOKEN",
		},
	}

	b, err := New(cfg, "dev")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if b.token != "test-token" {
		t.Errorf("token = %q", b.token)
	}
	if b.prefix != "angee" {
		t.Errorf("prefix = %q", b.prefix)
	}
}

func TestNewTokenAuthMissingEnv(t *testing.T) {
	cfg := &config.OpenBaoConfig{
		Address: "http://localhost:8200",
		Auth: config.OpenBaoAuthConfig{
			Method:   "token",
			TokenEnv: "NONEXISTENT_TOKEN_VAR",
		},
	}

	_, err := New(cfg, "dev")
	if err == nil {
		t.Fatal("expected error for missing token env var")
	}
}

func TestNewDefaultPrefix(t *testing.T) {
	t.Setenv("BAO_TOKEN", "tok")
	cfg := &config.OpenBaoConfig{
		Address: "http://localhost:8200",
		Auth:    config.OpenBaoAuthConfig{Method: "token"},
	}
	b, err := New(cfg, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if b.prefix != "angee" {
		t.Errorf("prefix = %q, want angee", b.prefix)
	}
	if b.env != "dev" {
		t.Errorf("env = %q, want dev", b.env)
	}
}

func TestNewMissingAddress(t *testing.T) {
	cfg := &config.OpenBaoConfig{}
	_, err := New(cfg, "dev")
	if err == nil || !strings.Contains(err.Error(), "address is required") {
		t.Errorf("expected address error, got: %v", err)
	}
}

func TestNewUnsupportedAuth(t *testing.T) {
	cfg := &config.OpenBaoConfig{
		Address: "http://localhost:8200",
		Auth:    config.OpenBaoAuthConfig{Method: "kerberos"},
	}
	_, err := New(cfg, "dev")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected unsupported auth error, got: %v", err)
	}
}

func TestExtractKVv2Value(t *testing.T) {
	body := `{"data":{"data":{"value":"hello"}}}`
	val, err := extractKVv2Value([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if val != "hello" {
		t.Errorf("value = %q", val)
	}
}

func TestExtractKVv2ValueMissingField(t *testing.T) {
	body := `{"data":{"data":{"other":"stuff"}}}`
	_, err := extractKVv2Value([]byte(body))
	if err == nil {
		t.Fatal("expected error for missing value field")
	}
}

func TestExtractKVv2Keys(t *testing.T) {
	body := `{"data":{"keys":["a","b","c"]}}`
	keys, err := extractKVv2Keys([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Errorf("keys = %v", keys)
	}
}
