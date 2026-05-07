package provision

import (
	"os"
	"testing"
	"time"

	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/state"
)

func TestResolvePortLeasesPreservesExisting(t *testing.T) {
	store := state.New(t.TempDir())
	if err := store.SavePortLeases(map[string]state.PortLease{
		"web": {Name: "web", Port: 12345, UpdatedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatal(err)
	}
	leases, err := ResolvePortLeases(store, map[string]config.PortLeaseSpec{
		"web": {Default: 54321, Band: "app"},
	}, nil, "stack")
	if err != nil {
		t.Fatalf("ResolvePortLeases() error: %v", err)
	}
	if leases["web"].Port != 12345 {
		t.Fatalf("web port = %d", leases["web"].Port)
	}
}

func TestResolvePortLeasesAppliesOverride(t *testing.T) {
	store := state.New(t.TempDir())
	leases, err := ResolvePortLeases(store, map[string]config.PortLeaseSpec{
		"web": {Default: 0, Band: "app"},
	}, map[string]int{"web": 12346}, "stack")
	if err != nil {
		t.Fatalf("ResolvePortLeases() error: %v", err)
	}
	if leases["web"].Port != 12346 {
		t.Fatalf("web port = %d", leases["web"].Port)
	}
}

func TestResolvePortLeasesRejectsUnknownOverride(t *testing.T) {
	store := state.New(t.TempDir())
	_, err := ResolvePortLeases(store, map[string]config.PortLeaseSpec{"web": {}}, map[string]int{"ui": 12347}, "stack")
	if err == nil {
		t.Fatal("expected unknown override error")
	}
}

func TestResolvePortLeasesAvoidsExistingGlobalLease(t *testing.T) {
	store := state.New(t.TempDir())
	if err := store.SavePortLeases(map[string]state.PortLease{
		"web": {Name: "web", Port: 12352, UpdatedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatal(err)
	}
	leases, err := ResolvePortLeases(store, map[string]config.PortLeaseSpec{
		"workspaces/feat/web": {Default: 12352},
	}, nil, "workspace:feat")
	if err != nil {
		t.Fatalf("ResolvePortLeases() error: %v", err)
	}
	if leases["workspaces/feat/web"].Port == 12352 {
		t.Fatal("expected workspace lease to avoid existing stack port")
	}
}

func TestResolveSecretsGeneratesAndPreserves(t *testing.T) {
	store := state.New(t.TempDir())
	secrets, err := ResolveSecrets(store, map[string]config.SecretSpec{
		"token": {Generated: true, Length: 16},
	}, nil)
	if err != nil {
		t.Fatalf("ResolveSecrets() error: %v", err)
	}
	first := secrets["token"].Value
	if len(first) != 16 {
		t.Fatalf("generated len = %d", len(first))
	}
	secrets, err = ResolveSecrets(store, map[string]config.SecretSpec{
		"token": {Generated: true, Length: 16},
	}, nil)
	if err != nil {
		t.Fatalf("ResolveSecrets() second error: %v", err)
	}
	if secrets["token"].Value != first {
		t.Fatal("expected generated secret to be preserved")
	}
}

func TestResolveSecretsSuppliedEnv(t *testing.T) {
	t.Setenv("ANGEE_TEST_SECRET", "from-env")
	store := state.New(t.TempDir())
	secrets, err := ResolveSecrets(store, map[string]config.SecretSpec{
		"api-key": {Required: true},
	}, map[string]string{"api-key": "env:ANGEE_TEST_SECRET"})
	if err != nil {
		t.Fatalf("ResolveSecrets() error: %v", err)
	}
	if secrets["api-key"].Value != "from-env" {
		t.Fatalf("secret value = %q", secrets["api-key"].Value)
	}
	if secrets["api-key"].Source != "env:ANGEE_TEST_SECRET" {
		t.Fatalf("secret source = %q", secrets["api-key"].Source)
	}
}

func TestResolveSecretsRequiredMissing(t *testing.T) {
	store := state.New(t.TempDir())
	_, err := ResolveSecrets(store, map[string]config.SecretSpec{
		"api-key": {Required: true},
	}, nil)
	if err == nil {
		t.Fatal("expected required secret error")
	}
}

func TestResolveSecretsRejectsUnknownSupplied(t *testing.T) {
	store := state.New(t.TempDir())
	_, err := ResolveSecrets(store, map[string]config.SecretSpec{"known": {}}, map[string]string{"unknown": "value"})
	if err == nil {
		t.Fatal("expected unknown supplied secret error")
	}
}

func TestSuppliedSecretMissingEnv(t *testing.T) {
	_ = os.Unsetenv("ANGEE_MISSING_SECRET")
	store := state.New(t.TempDir())
	_, err := ResolveSecrets(store, map[string]config.SecretSpec{"api-key": {Required: true}}, map[string]string{"api-key": "env:ANGEE_MISSING_SECRET"})
	if err == nil {
		t.Fatal("expected missing env error")
	}
}
