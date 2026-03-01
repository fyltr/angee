package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadComponent(t *testing.T) {
	dir := t.TempDir()
	content := `name: angee/postgres
type: service
version: "1.0.0"
description: "PostgreSQL with pgvector"

requires:
  - angee/openbao

parameters:
  - name: DBSize
    default: "20Gi"

services:
  postgres:
    image: pgvector/pgvector:pg17
    lifecycle: sidecar
    env:
      POSTGRES_USER: angee

secrets:
  - name: db-password
    required: true
`
	path := filepath.Join(dir, "angee-component.yaml")
	os.WriteFile(path, []byte(content), 0644)

	comp, err := LoadComponent(path)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	if comp.Name != "angee/postgres" {
		t.Errorf("expected name 'angee/postgres', got %q", comp.Name)
	}
	if comp.Type != "service" {
		t.Errorf("expected type 'service', got %q", comp.Type)
	}
	if len(comp.Requires) != 1 || comp.Requires[0] != "angee/openbao" {
		t.Errorf("unexpected requires: %v", comp.Requires)
	}
	if len(comp.Parameters) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(comp.Parameters))
	}
	if _, ok := comp.Services["postgres"]; !ok {
		t.Error("expected postgres service")
	}
	if len(comp.Secrets) != 1 {
		t.Errorf("expected 1 secret, got %d", len(comp.Secrets))
	}
}

func TestLoadComponent_Credential(t *testing.T) {
	dir := t.TempDir()
	content := `name: angee/oauth-github
type: credential
version: "1.0.0"

credential:
  name: github-oauth
  type: oauth_client
  provider:
    name: github
    auth_url: https://github.com/login/oauth/authorize
    token_url: https://github.com/login/oauth/access_token
    scopes:
      - repo
      - read:org
  outputs:
    - type: env
      key: GITHUB_TOKEN
      value_path: access_token
    - type: file
      template: templates/github_auth.json.tmpl
      mount: /root/.config/opencode/providers/github.json

secrets:
  - name: github-client-id
    required: true
`
	path := filepath.Join(dir, "angee-component.yaml")
	os.WriteFile(path, []byte(content), 0644)

	comp, err := LoadComponent(path)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	if comp.Credential == nil {
		t.Fatal("expected credential definition")
	}
	if comp.Credential.Name != "github-oauth" {
		t.Errorf("expected cred name 'github-oauth', got %q", comp.Credential.Name)
	}
	if comp.Credential.Provider.Name != "github" {
		t.Errorf("expected provider 'github', got %q", comp.Credential.Provider.Name)
	}
	if len(comp.Credential.Outputs) != 2 {
		t.Errorf("expected 2 outputs, got %d", len(comp.Credential.Outputs))
	}
	if comp.Credential.Outputs[0].Type != "env" {
		t.Errorf("expected first output type 'env', got %q", comp.Credential.Outputs[0].Type)
	}
	if comp.Credential.Outputs[1].Mount != "/root/.config/opencode/providers/github.json" {
		t.Errorf("unexpected mount: %q", comp.Credential.Outputs[1].Mount)
	}
}

func TestLoadComponent_Module(t *testing.T) {
	dir := t.TempDir()
	content := `name: fyltr/django-billing
type: module
extends: fyltr/fyltr-django

django:
  app: billing
  migrations: true
  urls: billing/

services:
  stripe-webhook:
    lifecycle: worker

secrets:
  - name: stripe-api-key
    required: true
`
	path := filepath.Join(dir, "angee-component.yaml")
	os.WriteFile(path, []byte(content), 0644)

	comp, err := LoadComponent(path)
	if err != nil {
		t.Fatalf("LoadComponent: %v", err)
	}

	if comp.Extends != "fyltr/fyltr-django" {
		t.Errorf("expected extends 'fyltr/fyltr-django', got %q", comp.Extends)
	}
	if comp.Django == nil {
		t.Fatal("expected django module def")
	}
	if comp.Django.App != "billing" {
		t.Errorf("expected app 'billing', got %q", comp.Django.App)
	}
}
