package credentials

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// EnvBackend reads and writes secrets from the .env file in ANGEE_ROOT.
// This preserves the existing behavior as the default backend.
type EnvBackend struct {
	EnvFilePath string
	mu          sync.Mutex
}

// NewEnvBackend creates a backend that reads/writes the given .env file.
func NewEnvBackend(envFilePath string) *EnvBackend {
	return &EnvBackend{EnvFilePath: envFilePath}
}

func (b *EnvBackend) Type() string { return "env" }

func (b *EnvBackend) Get(_ context.Context, name string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries, err := b.load()
	if err != nil {
		return "", err
	}

	envKey := secretToEnvKey(name)
	if val, ok := entries[envKey]; ok {
		return val, nil
	}
	return "", fmt.Errorf("secret %q not found", name)
}

func (b *EnvBackend) Set(_ context.Context, name string, value string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries, err := b.load()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if entries == nil {
		entries = make(map[string]string)
	}

	envKey := secretToEnvKey(name)
	entries[envKey] = value
	return b.save(entries)
}

func (b *EnvBackend) Delete(_ context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries, err := b.load()
	if err != nil {
		return err
	}

	envKey := secretToEnvKey(name)
	delete(entries, envKey)
	return b.save(entries)
}

func (b *EnvBackend) List(_ context.Context) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries, err := b.load()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for k := range entries {
		names = append(names, k)
	}
	sort.Strings(names)
	return names, nil
}

// load parses a .env file into key=value pairs.
func (b *EnvBackend) load() (map[string]string, error) {
	data, err := os.ReadFile(b.EnvFilePath)
	if err != nil {
		return nil, err
	}

	entries := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx > 0 {
			key := line[:idx]
			val := line[idx+1:]
			// Unescape $$ → $ (Docker Compose escaping)
			val = strings.ReplaceAll(val, "$$", "$")
			entries[key] = val
		}
	}
	return entries, nil
}

// save writes key=value pairs back to the .env file.
func (b *EnvBackend) save(entries map[string]string) error {
	var sb strings.Builder
	sb.WriteString("# Angee secrets — managed by angee\n")
	sb.WriteString("# DO NOT COMMIT — this file is gitignored\n\n")

	// Sort keys for deterministic output
	var keys []string
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		// Escape $ as $$ for Docker Compose
		val := strings.ReplaceAll(entries[k], "$", "$$")
		sb.WriteString(k + "=" + val + "\n")
	}
	return os.WriteFile(b.EnvFilePath, []byte(sb.String()), 0600)
}

// secretToEnvKey converts a secret name to an env var key.
// "db-password" → "DB_PASSWORD"
func secretToEnvKey(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}
