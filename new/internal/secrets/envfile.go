package secrets

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type EnvFileBackend struct {
	path   string
	keyFor func(string) string
	mu     sync.Mutex
}

type EnvFileOption func(*EnvFileBackend)

func WithKeyMapper(mapper func(string) string) EnvFileOption {
	return func(b *EnvFileBackend) {
		if mapper != nil {
			b.keyFor = mapper
		}
	}
}

func NewEnvFileBackend(path string, opts ...EnvFileOption) *EnvFileBackend {
	b := &EnvFileBackend{path: path, keyFor: func(key string) string { return key }}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

func (b *EnvFileBackend) Get(ctx context.Context, key string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	values, err := b.load()
	if err != nil {
		return "", false, err
	}
	value, ok := values[b.keyFor(key)]
	return value, ok, nil
}

func (b *EnvFileBackend) Set(ctx context.Context, key, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	storageKey := b.keyFor(key)
	if err := validateKey(storageKey); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	values, err := b.load()
	if err != nil {
		return err
	}
	values[storageKey] = value
	return b.save(values)
}

func (b *EnvFileBackend) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	values, err := b.load()
	if err != nil {
		return err
	}
	delete(values, b.keyFor(key))
	return b.save(values)
}

func (b *EnvFileBackend) List(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	values, err := b.load()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (b *EnvFileBackend) load() (map[string]string, error) {
	values := map[string]string{}
	f, err := os.Open(b.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return values, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: missing '='", b.path, lineNo)
		}
		key = strings.TrimSpace(key)
		if err := validateKey(key); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", b.path, lineNo, err)
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func (b *EnvFileBackend) save(values map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out strings.Builder
	for _, key := range keys {
		out.WriteString(key)
		out.WriteByte('=')
		out.WriteString(strconv.Quote(values[key]))
		out.WriteByte('\n')
	}
	return os.WriteFile(b.path, []byte(out.String()), 0o600)
}

func validateKey(key string) error {
	if key == "" {
		return errors.New("secret key is empty")
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return fmt.Errorf("secret key %q contains invalid character %q", key, r)
	}
	return nil
}
