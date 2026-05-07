package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreEnsureCreatesLayout(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	for _, rel := range []string{".", LocksDir, RunsDir} {
		path := filepath.Join(store.Root, rel)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", path)
		}
	}
}

func TestPortLeasesRoundTrip(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().UTC().Truncate(time.Second)
	leases := map[string]PortLease{
		"web": {Name: "web", Port: 8100, Band: "app", Owner: "stack", UpdatedAt: now},
	}
	if err := store.SavePortLeases(leases); err != nil {
		t.Fatalf("SavePortLeases() error: %v", err)
	}
	loaded, err := store.LoadPortLeases()
	if err != nil {
		t.Fatalf("LoadPortLeases() error: %v", err)
	}
	if loaded["web"].Port != 8100 {
		t.Fatalf("loaded web port = %d", loaded["web"].Port)
	}
	info, err := os.Stat(filepath.Join(store.Root, PortLeasesFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("port lease mode = %v", info.Mode().Perm())
	}
}

func TestSecretsRoundTripUsesPrivatePermissions(t *testing.T) {
	store := New(t.TempDir())
	secrets := map[string]Secret{
		"api-key": {Name: "api-key", Value: "secret", Source: "generated", UpdatedAt: time.Now().UTC()},
	}
	if err := store.SaveSecrets(secrets); err != nil {
		t.Fatalf("SaveSecrets() error: %v", err)
	}
	loaded, err := store.LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}
	if loaded["api-key"].Value != "secret" {
		t.Fatalf("loaded secret value = %q", loaded["api-key"].Value)
	}
	info, err := os.Stat(filepath.Join(store.Root, SecretsFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("secret mode = %v", info.Mode().Perm())
	}
}

func TestMissingFilesLoadAsEmpty(t *testing.T) {
	store := New(t.TempDir())
	ports, err := store.LoadPortLeases()
	if err != nil {
		t.Fatalf("LoadPortLeases() error: %v", err)
	}
	if len(ports) != 0 {
		t.Fatalf("expected empty ports, got %v", ports)
	}
	secrets, err := store.LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected empty secrets, got %v", secrets)
	}
}
