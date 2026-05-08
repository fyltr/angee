package mount

import "testing"

func TestResolveContainerWorkspaceMount(t *testing.T) {
	got, err := ResolveContainer("workspace://feat/code:/workspace:ro", Resolver{Workspaces: map[string]string{"feat": "/root/workspaces/feat"}})
	if err != nil {
		t.Fatalf("ResolveContainer() error = %v", err)
	}
	if got != "/root/workspaces/feat/code:/workspace:ro" {
		t.Fatalf("ResolveContainer() = %q", got)
	}
}

func TestResolveLocalEnv(t *testing.T) {
	key, value, err := ResolveLocalEnv("source://app/pkg:/src", Resolver{Sources: map[string]string{"app": "/root/sources/app"}})
	if err != nil {
		t.Fatalf("ResolveLocalEnv() error = %v", err)
	}
	if key != "SOURCE_APP_PKG_PATH" || value != "/root/sources/app/pkg" {
		t.Fatalf("ResolveLocalEnv() = %q %q", key, value)
	}
}

func TestResolveWorkdir(t *testing.T) {
	got, err := ResolveWorkdir("workspace://feat/code", Resolver{Workspaces: map[string]string{"feat": "/root/workspaces/feat"}})
	if err != nil {
		t.Fatalf("ResolveWorkdir() error = %v", err)
	}
	if got != "/root/workspaces/feat/code" {
		t.Fatalf("ResolveWorkdir() = %q", got)
	}
}
