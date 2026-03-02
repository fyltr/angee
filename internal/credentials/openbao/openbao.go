// Package openbao implements the credentials.Backend interface using
// OpenBao's KV v2 secrets engine.
package openbao

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fyltr/angee/internal/config"
)

// Backend stores secrets in OpenBao KV v2.
type Backend struct {
	addr   string // OpenBao API address (e.g. "http://openbao:8200")
	token  string // auth token
	prefix string // KV path prefix (default "angee")
	env    string // environment segment (dev/staging/prod)
}

// New creates an OpenBao backend from the angee.yaml secrets_backend config.
// It authenticates immediately (token or approle) and returns a ready backend.
func New(obCfg *config.OpenBaoConfig, environment string) (*Backend, error) {
	if obCfg.Address == "" {
		return nil, fmt.Errorf("openbao address is required")
	}

	prefix := obCfg.Prefix
	if prefix == "" {
		prefix = "angee"
	}
	if environment == "" {
		environment = "dev"
	}

	b := &Backend{
		addr:   strings.TrimRight(obCfg.Address, "/"),
		prefix: prefix,
		env:    environment,
	}

	// Authenticate
	switch obCfg.Auth.Method {
	case "token", "":
		envVar := obCfg.Auth.TokenEnv
		if envVar == "" {
			envVar = "BAO_TOKEN"
		}
		b.token = os.Getenv(envVar)
		if b.token == "" {
			return nil, fmt.Errorf("openbao token not found in env var %s", envVar)
		}

	case "approle":
		roleIDEnv := obCfg.Auth.RoleIDEnv
		if roleIDEnv == "" {
			roleIDEnv = "BAO_ROLE_ID"
		}
		secretIDEnv := obCfg.Auth.SecretIDEnv
		if secretIDEnv == "" {
			secretIDEnv = "BAO_SECRET_ID"
		}
		roleID := os.Getenv(roleIDEnv)
		secretID := os.Getenv(secretIDEnv)
		if roleID == "" || secretID == "" {
			return nil, fmt.Errorf("openbao approle requires %s and %s env vars", roleIDEnv, secretIDEnv)
		}
		token, err := b.approleLogin(context.Background(), roleID, secretID)
		if err != nil {
			return nil, fmt.Errorf("openbao approle login: %w", err)
		}
		b.token = token

	default:
		return nil, fmt.Errorf("unsupported openbao auth method: %q", obCfg.Auth.Method)
	}

	return b, nil
}

// TryNew creates an OpenBao backend if reachable and configured, otherwise returns nil.
// Used during init/add when OpenBao may not be running yet.
func TryNew(obCfg *config.OpenBaoConfig, environment string) *Backend {
	b := tryBuild(obCfg, environment)
	if b == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if !b.IsReachable(ctx) {
		return nil
	}

	return b
}

// NewFromEnv creates an OpenBao backend from config without checking reachability.
// Returns nil if the config is missing or the token env var is unset.
// Used when the caller will handle readiness checks separately (e.g. WaitReady).
func NewFromEnv(obCfg *config.OpenBaoConfig, environment string) *Backend {
	return tryBuild(obCfg, environment)
}

// tryBuild creates a Backend from config by reading the token from env vars.
// Returns nil if config is missing or the token is unset.
func tryBuild(obCfg *config.OpenBaoConfig, environment string) *Backend {
	if obCfg == nil || obCfg.Address == "" {
		return nil
	}

	prefix := obCfg.Prefix
	if prefix == "" {
		prefix = "angee"
	}
	if environment == "" {
		environment = "dev"
	}

	envVar := obCfg.Auth.TokenEnv
	if envVar == "" {
		envVar = "BAO_TOKEN"
	}
	token := os.Getenv(envVar)
	if token == "" {
		return nil
	}

	return &Backend{
		addr:   strings.TrimRight(obCfg.Address, "/"),
		token:  token,
		prefix: prefix,
		env:    environment,
	}
}

// IsReachable returns true if OpenBao responds to a health check (HTTP 200 or 429).
func (b *Backend) IsReachable(ctx context.Context) bool {
	url := b.addr + "/v1/sys/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200 || resp.StatusCode == 429
}

// WaitReady polls IsReachable every 500ms until the context expires.
func (b *Backend) WaitReady(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Check immediately before first tick
	if b.IsReachable(ctx) {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("openbao not ready: %w", ctx.Err())
		case <-ticker.C:
			if b.IsReachable(ctx) {
				return nil
			}
		}
	}
}

// SetIfAbsent writes a secret only if it does not already exist in OpenBao.
// Returns true if the value was written, false if it already existed.
func (b *Backend) SetIfAbsent(ctx context.Context, name, value string) (bool, error) {
	_, err := b.Get(ctx, name)
	if err == nil {
		return false, nil // already exists
	}
	// 404 means not found — safe to write
	if err := b.Set(ctx, name, value); err != nil {
		return false, err
	}
	return true, nil
}

func (b *Backend) Type() string { return "openbao" }

// dataPath returns the KV v2 data path: secret/data/{prefix}/{env}/{name}
func (b *Backend) dataPath(name string) string {
	return fmt.Sprintf("secret/data/%s/%s/%s", b.prefix, b.env, name)
}

// metadataPath returns the KV v2 metadata path for a specific secret.
func (b *Backend) metadataPath(name string) string {
	return fmt.Sprintf("secret/metadata/%s/%s/%s", b.prefix, b.env, name)
}

// listPath returns the KV v2 metadata list path.
func (b *Backend) listPath() string {
	return fmt.Sprintf("secret/metadata/%s/%s", b.prefix, b.env)
}

func (b *Backend) Get(ctx context.Context, name string) (string, error) {
	body, err := b.request(ctx, http.MethodGet, b.dataPath(name), nil)
	if err != nil {
		return "", fmt.Errorf("reading secret %q: %w", name, err)
	}

	val, err := extractKVv2Value(body)
	if err != nil {
		return "", fmt.Errorf("parsing secret %q: %w", name, err)
	}
	return val, nil
}

func (b *Backend) Set(ctx context.Context, name string, value string) error {
	payload := map[string]any{
		"data": map[string]any{
			"value": value,
		},
	}
	_, err := b.request(ctx, http.MethodPost, b.dataPath(name), payload)
	if err != nil {
		return fmt.Errorf("writing secret %q: %w", name, err)
	}
	return nil
}

func (b *Backend) Delete(ctx context.Context, name string) error {
	_, err := b.request(ctx, http.MethodDelete, b.metadataPath(name), nil)
	if err != nil {
		return fmt.Errorf("deleting secret %q: %w", name, err)
	}
	return nil
}

func (b *Backend) List(ctx context.Context) ([]string, error) {
	body, err := b.request(ctx, "LIST", b.listPath(), nil)
	if err != nil {
		// 404 means no secrets yet
		if strings.Contains(err.Error(), "404") {
			return nil, nil
		}
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	keys, err := extractKVv2Keys(body)
	if err != nil {
		return nil, fmt.Errorf("parsing secret list: %w", err)
	}
	return keys, nil
}

// approleLogin authenticates with AppRole and returns a token.
func (b *Backend) approleLogin(ctx context.Context, roleID, secretID string) (string, error) {
	payload := map[string]any{
		"role_id":   roleID,
		"secret_id": secretID,
	}
	body, err := b.request(ctx, http.MethodPost, "auth/approle/login", payload)
	if err != nil {
		return "", err
	}

	var resp struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parsing approle response: %w", err)
	}
	if resp.Auth.ClientToken == "" {
		return "", fmt.Errorf("approle login returned empty token")
	}
	return resp.Auth.ClientToken, nil
}

// request sends an HTTP request to the OpenBao API.
func (b *Backend) request(ctx context.Context, method, path string, payload any) ([]byte, error) {
	url := b.addr + "/v1/" + path

	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = strings.NewReader(string(data))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	if b.token != "" {
		req.Header.Set("X-Vault-Token", b.token)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openbao %s %s returned %d: %s", method, path, resp.StatusCode, string(body))
	}

	return body, nil
}

// extractKVv2Value extracts the "value" field from a KV v2 GET response.
// Response shape: {"data": {"data": {"value": "..."}}}
func extractKVv2Value(body []byte) (string, error) {
	var resp struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	val, ok := resp.Data.Data["value"]
	if !ok {
		return "", fmt.Errorf("no 'value' field in secret data")
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("secret 'value' is not a string")
	}
	return s, nil
}

// extractKVv2Keys extracts the key list from a KV v2 LIST response.
// Response shape: {"data": {"keys": [...]}}
func extractKVv2Keys(body []byte) ([]string, error) {
	var resp struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Data.Keys, nil
}
