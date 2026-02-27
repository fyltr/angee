package tmpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	dir := t.TempDir()
	tmplContent := `name: {{ .ProjectName }}
domain: {{ .Domain }}
workers: {{ .DjangoWorkers }}`
	if err := os.WriteFile(filepath.Join(dir, "angee.yaml.tmpl"), []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	params := DefaultParams("my-project")
	result, err := Render(dir, params)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	if !strings.Contains(result, "name: my-project") {
		t.Errorf("expected 'name: my-project' in output, got: %s", result)
	}
	if !strings.Contains(result, "domain: localhost") {
		t.Errorf("expected 'domain: localhost' in output, got: %s", result)
	}
	if !strings.Contains(result, "workers: 3") {
		t.Errorf("expected 'workers: 3' in output, got: %s", result)
	}
}

func TestLoadMeta(t *testing.T) {
	dir := t.TempDir()
	metaContent := `
name: test-template
description: A test template
version: "1.0"
parameters:
  - name: domain
    description: The domain
    default: localhost
    required: false
secrets:
  - name: secret-key
    description: Django secret key
    generated: true
    length: 50
`
	if err := os.WriteFile(filepath.Join(dir, ".angee-template.yaml"), []byte(metaContent), 0644); err != nil {
		t.Fatal(err)
	}

	meta, err := LoadMeta(dir)
	if err != nil {
		t.Fatalf("LoadMeta() error: %v", err)
	}

	if meta.Name != "test-template" {
		t.Errorf("Name = %q, want %q", meta.Name, "test-template")
	}
	if meta.Version != "1.0" {
		t.Errorf("Version = %q, want %q", meta.Version, "1.0")
	}
	if len(meta.Parameters) != 1 {
		t.Fatalf("Parameters count = %d, want 1", len(meta.Parameters))
	}
	if meta.Parameters[0].Name != "domain" {
		t.Errorf("Parameter name = %q, want %q", meta.Parameters[0].Name, "domain")
	}
	if len(meta.Secrets) != 1 {
		t.Fatalf("Secrets count = %d, want 1", len(meta.Secrets))
	}
	if !meta.Secrets[0].Generated {
		t.Error("expected secret to be generated")
	}
}

func TestGenerateValue(t *testing.T) {
	def := SecretDef{Name: "key", Generated: true, Length: 32, Charset: "abcdefghijklmnopqrstuvwxyz0123456789"}
	val, err := generateValue(def)
	if err != nil {
		t.Fatalf("generateValue() error: %v", err)
	}
	if len(val) != 32 {
		t.Errorf("len(val) = %d, want 32", len(val))
	}

	// All chars should be from the charset
	for _, c := range val {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789", c) {
			t.Errorf("unexpected character %q in generated value", string(c))
		}
	}

	// Two generated values should be different (probabilistically)
	val2, _ := generateValue(def)
	if val == val2 {
		t.Error("two generated values should differ")
	}
}

func TestGenerateValueDefaultLength(t *testing.T) {
	def := SecretDef{Name: "key", Generated: true}
	val, err := generateValue(def)
	if err != nil {
		t.Fatalf("generateValue() error: %v", err)
	}
	if len(val) != 50 {
		t.Errorf("len(val) = %d, want 50 (default)", len(val))
	}
}

func TestResolveSecretsFromFlags(t *testing.T) {
	meta := &TemplateMeta{
		Secrets: []SecretDef{
			{Name: "db-password", Required: true},
			{Name: "secret-key", Generated: true, Length: 10},
		},
	}
	supplied := map[string]string{
		"db-password": "my-password",
		"secret-key":  "supplied-key",
	}

	resolved, err := ResolveSecrets(meta, supplied, "proj")
	if err != nil {
		t.Fatalf("ResolveSecrets() error: %v", err)
	}

	if len(resolved) != 2 {
		t.Fatalf("resolved count = %d, want 2", len(resolved))
	}

	// Both should be from flag since supplied values take priority
	for _, r := range resolved {
		if r.Source != "flag" {
			t.Errorf("%s.Source = %q, want %q", r.Name, r.Source, "flag")
		}
	}

	if resolved[0].Value != "my-password" {
		t.Errorf("db-password = %q, want %q", resolved[0].Value, "my-password")
	}
	if resolved[1].Value != "supplied-key" {
		t.Errorf("secret-key = %q, want %q", resolved[1].Value, "supplied-key")
	}
}

func TestResolveSecretsGenerated(t *testing.T) {
	meta := &TemplateMeta{
		Secrets: []SecretDef{
			{Name: "secret-key", Generated: true, Length: 20, Charset: "abc"},
		},
	}

	resolved, err := ResolveSecrets(meta, nil, "proj")
	if err != nil {
		t.Fatalf("ResolveSecrets() error: %v", err)
	}

	if len(resolved) != 1 {
		t.Fatalf("resolved count = %d, want 1", len(resolved))
	}
	if resolved[0].Source != "generated" {
		t.Errorf("Source = %q, want %q", resolved[0].Source, "generated")
	}
	if len(resolved[0].Value) != 20 {
		t.Errorf("len(Value) = %d, want 20", len(resolved[0].Value))
	}
}

func TestResolveSecretsDerived(t *testing.T) {
	meta := &TemplateMeta{
		Secrets: []SecretDef{
			{Name: "db-password", Generated: true, Length: 10, Charset: "x"},
			{Name: "db-url", Derived: "postgres://user:${db-password}@localhost/${project}"},
		},
	}

	resolved, err := ResolveSecrets(meta, nil, "myapp")
	if err != nil {
		t.Fatalf("ResolveSecrets() error: %v", err)
	}

	if len(resolved) != 2 {
		t.Fatalf("resolved count = %d, want 2", len(resolved))
	}

	dbURL := resolved[1]
	if dbURL.Source != "derived" {
		t.Errorf("Source = %q, want %q", dbURL.Source, "derived")
	}
	if !strings.Contains(dbURL.Value, "xxxxxxxxxx") {
		t.Errorf("expected derived URL to contain the generated password")
	}
	if !strings.Contains(dbURL.Value, "myapp") {
		t.Errorf("expected derived URL to contain project name")
	}
}

func TestSplitFragment(t *testing.T) {
	tests := []struct {
		input    string
		wantURL  string
		wantSub  string
	}{
		{"https://github.com/org/repo#templates/default", "https://github.com/org/repo", "templates/default"},
		{"https://github.com/org/repo", "https://github.com/org/repo", ""},
		{"./local/path", "./local/path", ""},
		{"https://github.com/org/repo#sub/dir", "https://github.com/org/repo", "sub/dir"},
	}
	for _, tt := range tests {
		gotURL, gotSub := splitFragment(tt.input)
		if gotURL != tt.wantURL || gotSub != tt.wantSub {
			t.Errorf("splitFragment(%q) = (%q, %q), want (%q, %q)", tt.input, gotURL, gotSub, tt.wantURL, tt.wantSub)
		}
	}
}

func TestFetchTemplateLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "angee.yaml.tmpl"), []byte("name: test"), 0644); err != nil {
		t.Fatal(err)
	}

	got, cleanupDir, err := FetchTemplate(dir)
	if err != nil {
		t.Fatalf("FetchTemplate() error: %v", err)
	}
	if cleanupDir != "" {
		t.Error("expected empty cleanupDir for local path")
	}
	if got != dir {
		t.Errorf("got = %q, want %q", got, dir)
	}
}

func TestFormatEnvFile(t *testing.T) {
	secrets := []ResolvedSecret{
		{Name: "db-password", Value: "hunter2", Source: "flag"},
		{Name: "secret-key", Value: "abc123", Source: "generated"},
	}

	output := FormatEnvFile(secrets)

	if !strings.Contains(output, "DB_PASSWORD=hunter2") {
		t.Errorf("expected DB_PASSWORD=hunter2 in output, got:\n%s", output)
	}
	if !strings.Contains(output, "SECRET_KEY=abc123") {
		t.Errorf("expected SECRET_KEY=abc123 in output, got:\n%s", output)
	}
	if !strings.Contains(output, "# Angee secrets") {
		t.Error("expected header comment")
	}
}
