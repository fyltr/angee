package substitute

import "testing"

func TestResolveSubstitutionsAndFilters(t *testing.T) {
	ctx := Context{
		Secrets: map[string]string{"token": "secret"},
		Ports:   map[string]int{"web": 8100},
		Inputs:  map[string]string{"branch": "Feat/Issue 123", "email": "dev@example.com"},
		Name:    "workspace-one",
		Operator: Operator{
			URL: "http://127.0.0.1:9000",
		},
	}

	got, err := Resolve("${inputs.branch | slug | truncate(12)}:${ports.web}:${inputs.email | local_part}:${operator.url}", ctx)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "feat-issue-1:8100:dev:http://127.0.0.1:9000" {
		t.Fatalf("Resolve() = %q", got)
	}
}

func TestResolveSecretEnvPlaceholder(t *testing.T) {
	ctx := Context{
		Secrets:       map[string]string{"postgres-password": "secret"},
		SecretEnvVars: map[string]string{"postgres-password": "ANGEE_SECRET_POSTGRES_PASSWORD"},
	}
	got, err := Resolve("password=${secret.postgres-password}", ctx)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "password=${ANGEE_SECRET_POSTGRES_PASSWORD}" {
		t.Fatalf("Resolve() = %q", got)
	}
}

func TestRequiredFilterRejectsEmpty(t *testing.T) {
	_, err := Resolve("${name | required('name required')}", Context{})
	if err == nil {
		t.Fatal("Resolve() error = nil, want error")
	}
}

func TestSecretEnvName(t *testing.T) {
	if got := SecretEnvName("postgres-password"); got != "ANGEE_SECRET_POSTGRES_PASSWORD" {
		t.Fatalf("SecretEnvName() = %q", got)
	}
}

func TestTruncateCountsRunes(t *testing.T) {
	got, err := Resolve("${inputs.value | truncate(2)}", Context{Inputs: map[string]string{"value": "åßcd"}})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "åß" {
		t.Fatalf("Resolve() = %q", got)
	}
}
