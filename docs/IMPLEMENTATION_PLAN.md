# Implementation Plan: P0 + P1

## P0: Credential Backend + Environment Layering

### Goal
Replace flat `.env` secrets with a pluggable backend (OpenBao first). Add environment overlays for dev/staging/prod isolation.

### New packages

#### `internal/credentials/backend.go` — Interface

```go
package credentials

import "context"

// Backend is the pluggable secrets resolution interface.
// The operator uses this to resolve ${secret:name} references
// and to manage credentials via the REST API.
type Backend interface {
    // Get retrieves a secret value.
    Get(ctx context.Context, name string) (string, error)

    // Set stores a secret value.
    Set(ctx context.Context, name string, value string) error

    // Delete removes a secret.
    Delete(ctx context.Context, name string) error

    // List returns all secret names (not values) under the prefix.
    List(ctx context.Context) ([]string, error)

    // Type returns the backend type name.
    Type() string
}
```

#### `internal/credentials/env.go` — Env backend (current behavior)

```go
// EnvBackend reads/writes secrets from the .env file in ANGEE_ROOT.
// This is the current behavior, preserved as the default backend.
type EnvBackend struct {
    EnvFilePath string
}
```

- `Get` → parse .env file, find key
- `Set` → update or append key in .env file
- `Delete` → remove key from .env file
- `List` → return all keys from .env file

This preserves full backward compatibility.

#### `internal/credentials/openbao/openbao.go` — OpenBao backend

```go
// OpenBaoBackend resolves secrets from an OpenBao KV v2 store.
type OpenBaoBackend struct {
    Client     *api.Client
    MountPath  string   // "secret"
    PathPrefix string   // "angee"
    Env        string   // "dev" | "staging" | "prod"
}
```

Path resolution: `{mount}/data/{prefix}/{env}/{name}`

Example: `secret/data/angee/dev/db-password`

Dependencies: `github.com/openbao/openbao/api/v2` (single new dependency)

### Config changes

#### `internal/config/angee.go` — Add fields

```go
type AngeeConfig struct {
    Name            string                    `yaml:"name"`
    Version         string                    `yaml:"version,omitempty"`
    Environment     string                    `yaml:"environment,omitempty"`       // NEW
    SecretsBackend  *SecretsBackendConfig     `yaml:"secrets_backend,omitempty"`   // NEW
    Repositories    map[string]RepositorySpec `yaml:"repositories,omitempty"`
    // ... existing fields
}

// SecretsBackendConfig configures the credential resolution backend.
type SecretsBackendConfig struct {
    Type    string          `yaml:"type"`              // "env" | "openbao"
    OpenBao *OpenBaoConfig  `yaml:"openbao,omitempty"`
}

type OpenBaoConfig struct {
    Address string              `yaml:"address"`
    Auth    OpenBaoAuthConfig   `yaml:"auth"`
    Prefix  string              `yaml:"prefix,omitempty"`
}

type OpenBaoAuthConfig struct {
    Method      string `yaml:"method"`                // "token" | "approle"
    TokenEnv    string `yaml:"token_env,omitempty"`    // env var name
    RoleIDEnv   string `yaml:"role_id_env,omitempty"`
    SecretIDEnv string `yaml:"secret_id_env,omitempty"`
}
```

### Environment overlay loading

#### `internal/config/overlay.go` — New file

```go
// LoadWithOverlay loads angee.yaml and merges environment overlay.
func LoadWithOverlay(rootPath string, env string) (*AngeeConfig, error) {
    base, err := Load(filepath.Join(rootPath, "angee.yaml"))
    if err != nil {
        return nil, err
    }
    if env == "" {
        env = base.Environment
    }
    if env == "" {
        env = "dev"
    }

    overlayPath := filepath.Join(rootPath, "environments", env+".yaml")
    if _, err := os.Stat(overlayPath); os.IsNotExist(err) {
        return base, nil // no overlay for this env
    }

    overlay, err := Load(overlayPath)
    if err != nil {
        return nil, fmt.Errorf("loading %s overlay: %w", env, err)
    }

    return merge(base, overlay), nil
}
```

Deep-merge rules:
- Maps: overlay keys override base keys
- Lists: overlay replaces base (not appended)
- Scalars: overlay wins
- Empty/zero values in overlay do NOT override base

### Backend factory

#### `internal/credentials/factory.go`

```go
func NewBackend(cfg *config.AngeeConfig, rootPath string) (Backend, error) {
    if cfg.SecretsBackend == nil || cfg.SecretsBackend.Type == "" || cfg.SecretsBackend.Type == "env" {
        return &EnvBackend{EnvFilePath: filepath.Join(rootPath, ".env")}, nil
    }
    if cfg.SecretsBackend.Type == "openbao" {
        return openbao.New(cfg.SecretsBackend.OpenBao, cfg.Environment)
    }
    return nil, fmt.Errorf("unknown secrets backend: %s", cfg.SecretsBackend.Type)
}
```

### Compiler changes

#### `internal/compiler/compose.go`

The `resolveSecretRefs` function currently does string replacement: `${secret:name}` → `${ENV_NAME}`.

With the OpenBao backend, the compiler needs access to the backend to resolve values at compile time:

```go
// Compiler gets a new field:
type Compiler struct {
    Network   string
    RootPath  string
    APIKey    string
    Secrets   credentials.Backend  // NEW — nil means use env-style string replacement
}
```

When `Secrets` is nil (env backend), behavior is unchanged: `${secret:name}` → `${ENV_NAME}`.

When `Secrets` is set (openbao backend), the compiler resolves the actual value and injects it directly into the per-agent `.env` files or compose environment.

### Operator changes

#### `internal/operator/server.go`

Server gains a `credentials.Backend` field, initialized at startup.

#### `internal/operator/handlers_credentials.go` — New file

```go
// GET  /credentials      → list secret names
// GET  /credentials/{name} → get metadata (not value)
// POST /credentials/{name} → set value
// DELETE /credentials/{name} → delete
```

#### `internal/operator/mcp.go`

Add MCP tools:
- `credentials_list` — list credential names
- `credentials_set` — set a credential
- `credentials_delete` — delete a credential

This lets the admin agent manage secrets via MCP.

### CLI changes

#### `cli/credential.go` — New file

```
angee credential list [--env dev]
angee credential get <name> [--env dev]
angee credential set <name> <value> [--env dev]
angee credential delete <name> [--env dev]
```

For `env` backend: reads/writes .env file directly.
For `openbao` backend: calls operator API.

### Files changed (P0)

| Action | Path |
|--------|------|
| **New** | `internal/credentials/backend.go` |
| **New** | `internal/credentials/env.go` |
| **New** | `internal/credentials/factory.go` |
| **New** | `internal/credentials/openbao/openbao.go` |
| **New** | `internal/config/overlay.go` |
| **New** | `internal/operator/handlers_credentials.go` |
| **New** | `cli/credential.go` |
| **Modify** | `internal/config/angee.go` — add SecretsBackend, Environment fields |
| **Modify** | `internal/config/validate.go` — validate new fields |
| **Modify** | `internal/compiler/compose.go` — use Backend for resolution |
| **Modify** | `internal/operator/server.go` — init backend, register routes |
| **Modify** | `internal/operator/mcp.go` — add credential MCP tools |
| **Modify** | `go.mod` — add `github.com/openbao/openbao/api/v2` |

### Testing (P0)

- `internal/credentials/env_test.go` — round-trip .env read/write
- `internal/credentials/openbao/openbao_test.go` — requires testcontainers or mock
- `internal/config/overlay_test.go` — merge logic
- `internal/compiler/compose_test.go` — verify both backends produce correct output

---

## P1: Agent Registry + Internal JWT Auth

### Goal
Versioned agent identities via filesystem registry. Operator-issued JWT tokens for agent→MCP internal authentication.

### Agent Registry

#### Filesystem layout

```
.angee/registry/
├── agents/
│   └── angee-admin/
│       ├── manifest.yaml
│       └── versions/
│           └── v1.0.0/
│               └── agent.yaml
└── mcp/
    └── operator/
        ├── manifest.yaml
        └── versions/
            └── v1.0.0/
                └── mcp.yaml
```

#### `manifest.yaml` format

```yaml
name: angee-admin
latest: v1.0.0
versions:
  - v1.0.0
```

#### `agent.yaml` format (registry entry)

```yaml
name: angee-admin
version: v1.0.0
image: ghcr.io/anomalyco/opencode@sha256:abc123
command: serve --hostname 0.0.0.0 --port 4096

permissions:
  - mcp:operator:*
  - mcp:files:*

runtime:
  lifecycle: system
  role: operator
  resources:
    cpu: "1.0"
    memory: "2Gi"

skills:
  - deploy
```

#### New package: `internal/registry/`

```go
package registry

// Registry resolves versioned agent and MCP server definitions.
type Registry interface {
    ResolveAgent(name, version string) (*AgentDefinition, error)
    ResolveMCP(name, version string) (*MCPDefinition, error)
    List(kind string) ([]Entry, error)
}

// AgentDefinition is the resolved agent identity from the registry.
type AgentDefinition struct {
    Name        string
    Version     string
    Image       string
    Command     string
    Permissions []string
    Runtime     RuntimeDefaults
    Skills      []string
}

// FilesystemRegistry reads from .angee/registry/.
type FilesystemRegistry struct {
    RootPath string
}
```

Version resolution rules:
- `@v1` → latest version matching `v1.*.*`
- `@v1.2` → latest version matching `v1.2.*`
- `@v1.2.3` → exact version
- `@latest` → manifest.latest

#### Config changes

Add `ref` field to `AgentSpec`:

```go
type AgentSpec struct {
    Ref          string            `yaml:"ref,omitempty"`          // registry://agents/<name>@<version>
    Permissions  []string          `yaml:"permissions,omitempty"`  // operator-enforced
    // ... existing fields
}
```

#### Compiler changes

Before compilation, the compiler resolves `ref` fields:

```go
func (c *Compiler) resolveRefs(cfg *config.AngeeConfig, reg registry.Registry) error {
    for name, agent := range cfg.Agents {
        if agent.Ref == "" {
            continue
        }
        // Parse: registry://agents/angee-admin@v1
        kind, refName, version := parseRef(agent.Ref)
        def, err := reg.ResolveAgent(refName, version)
        if err != nil {
            return err
        }
        // Merge: registry defaults ← angee.yaml overrides
        merged := mergeAgentDef(def, agent)
        cfg.Agents[name] = merged
    }
    return nil
}
```

Merge rule: angee.yaml fields override registry defaults. Empty angee.yaml fields inherit from registry.

#### CLI commands

```
angee registry init                          # create .angee/registry/ structure
angee registry list agents                   # list registered agents
angee registry publish agent <name> <ver>    # publish version from angee.yaml agent def
angee registry resolve <uri>                 # resolve and print definition
```

### Internal JWT Auth

#### New package: `internal/auth/`

```go
package auth

import (
    "crypto/ed25519"
    "time"
)

// Issuer creates and validates agent identity tokens.
type Issuer struct {
    PrivateKey ed25519.PrivateKey
    PublicKey  ed25519.PublicKey
    Issuer     string  // "angee-operator"
}

// NewIssuer generates a new Ed25519 key pair.
func NewIssuer() (*Issuer, error)

// IssueToken creates a signed JWT for an agent.
func (i *Issuer) IssueToken(claims AgentClaims) (string, error)

// ValidateToken verifies a JWT and returns claims.
func (i *Issuer) ValidateToken(token string) (*AgentClaims, error)

// JWKS returns the JSON Web Key Set for token verification.
func (i *Issuer) JWKS() []byte

type AgentClaims struct {
    Sub         string   `json:"sub"`         // agent://angee-admin
    Env         string   `json:"env"`         // dev | staging | prod
    Stack       string   `json:"stack"`       // project name
    Role        string   `json:"role"`        // operator | user
    Permissions []string `json:"permissions"` // mcp:operator:*, etc.
    Exp         int64    `json:"exp"`         // short TTL: 15 min
    Iat         int64    `json:"iat"`
    Iss         string   `json:"iss"`         // angee-operator
}
```

Implementation notes:
- Uses only Go stdlib: `crypto/ed25519`, `encoding/json`, `encoding/base64`
- JWT is compact JWS with Ed25519 signature (alg: EdDSA)
- No external JWT library needed — format is simple enough
- Key pair generated at operator startup, stored in memory (regenerated on restart)
- For persistence across restarts, key can be stored in OpenBao: `secret/data/angee/operator/signing-key`

#### Operator endpoints

```go
// POST /agent/token
// Request: { "agent": "angee-admin" }
// Response: { "token": "eyJ...", "expires_at": "..." }
func (s *Server) handleAgentToken(w http.ResponseWriter, r *http.Request)

// GET /.well-known/jwks.json
// Response: { "keys": [{ "kty": "OKP", "crv": "Ed25519", ... }] }
func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request)
```

#### MCP endpoint changes

The MCP endpoint (`POST /mcp`) gains optional JWT verification:

```go
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
    // Extract Bearer token from Authorization header
    token := extractBearer(r)
    if token != "" {
        claims, err := s.Auth.ValidateToken(token)
        if err != nil {
            jsonErr(w, 401, "invalid token")
            return
        }
        // Enforce permissions: check claims.Permissions against requested tool
        if !s.authorizeCall(claims, method) {
            jsonErr(w, 403, "insufficient permissions")
            return
        }
    }
    // ... existing MCP handling
}
```

#### Agent token injection

At deploy time, the operator:
1. Issues a JWT for each agent
2. Writes it to `agents/<name>/.env` as `ANGEE_AGENT_TOKEN=<jwt>`
3. Agent uses this token in Authorization header when calling MCP servers

### Files changed (P1)

| Action | Path |
|--------|------|
| **New** | `internal/registry/registry.go` |
| **New** | `internal/registry/filesystem.go` |
| **New** | `internal/registry/resolve.go` |
| **New** | `internal/auth/issuer.go` |
| **New** | `internal/auth/jwt.go` |
| **New** | `internal/auth/jwks.go` |
| **New** | `internal/operator/handlers_auth.go` |
| **New** | `cli/registry.go` |
| **Modify** | `internal/config/angee.go` — add Ref, Permissions to AgentSpec |
| **Modify** | `internal/config/validate.go` — validate ref URIs, permissions syntax |
| **Modify** | `internal/compiler/compose.go` — resolve refs before compile |
| **Modify** | `internal/operator/server.go` — init auth issuer, register routes |
| **Modify** | `internal/operator/mcp.go` — JWT verification, permission enforcement |
| **Modify** | `internal/operator/handlers.go` — inject agent tokens at deploy time |

### Testing (P1)

- `internal/registry/filesystem_test.go` — version resolution, merge logic
- `internal/auth/issuer_test.go` — issue + validate round trip
- `internal/auth/jwt_test.go` — token format, expiry, claims
- `internal/compiler/compose_test.go` — ref resolution in compile pipeline

---

## Dependency Summary

| Dependency | Version | License | Purpose |
|-----------|---------|---------|---------|
| `github.com/openbao/openbao/api/v2` | latest | MPL-2.0 | OpenBao client (P0) |
| `golang.org/x/oauth2` | latest | BSD-3 | OAuth token broker (P2, future) |

No new dependencies for P1 — JWT implementation uses Go stdlib only.

---

## Implementation Order

### P0: Implemented

```
✅ P0 Step 1: internal/credentials/ (Backend interface + EnvBackend)
✅ P0 Step 2: internal/config/ (new types + overlay loading + component config)
✅ P0 Step 4: internal/compiler/ (credential binding resolution in agent compilation)
✅ P0 Step 6: cli/credential.go (CLI commands: list, get, set, delete)
✅ P0 Step 7: Tests (credentials, overlay, component, compiler credential bindings)
✅ P0-C Step 1: internal/config/component.go (ComponentConfig, CredentialDef, CredentialOutput)
✅ P0-C Step 2: internal/component/add.go (full add lifecycle: fetch → merge → record)
✅ P0-C Step 3: internal/component/remove.go (remove + list installed)
✅ P0-C Step 4: cli/add.go, cli/remove.go (angee add / remove / components)
```

### P0: Remaining

```
⬜ P0 Step 3: internal/credentials/openbao/ (OpenBao backend — requires openbao API dep)
⬜ P0 Step 5: internal/operator/ (credential handlers + MCP tools for credentials)
⬜ P0-C Step 5: Credential output caching in .angee/credential-outputs/ during angee add
⬜ P0-C Step 6: Render credential file templates at deploy time (RenderAgentFiles integration)
```

### P1: Not started

```
⬜ P1 Step 1: internal/registry/ (filesystem registry + resolution)
⬜ P1 Step 2: internal/auth/ (JWT issuer)
⬜ P1 Step 3: internal/config/ (ref field on AgentSpec)
⬜ P1 Step 4: internal/compiler/ (ref resolution in compile)
⬜ P1 Step 5: internal/operator/ (token endpoint, JWKS, permission enforcement)
⬜ P1 Step 6: cli/registry.go (CLI commands)
⬜ P1 Step 7: Tests
```

Each step is independently testable and backwards-compatible.

---

## Files Added/Modified (Complete)

### New files

| Path | Description |
|------|-------------|
| `internal/config/component.go` | ComponentConfig, CredentialDef, CredentialOutput, InstalledComponent types |
| `internal/config/overlay.go` | MergeConfigs, LoadWithOverlay (environment overlays) |
| `internal/credentials/backend.go` | Backend interface (Get/Set/Delete/List) |
| `internal/credentials/env.go` | EnvBackend (.env file read/write with $ escaping) |
| `internal/credentials/factory.go` | NewBackend factory (env or openbao) |
| `internal/component/add.go` | Add() — full component installation lifecycle |
| `internal/component/remove.go` | Remove(), List() — component removal and listing |
| `cli/add.go` | `angee add` command (--param, --deploy, --yes) |
| `cli/remove.go` | `angee remove` + `angee components` commands |
| `cli/credential.go` | `angee credential list/get/set/delete` commands |
| `docs/ANGEE_SPEC.md` | Complete angee.yaml format reference |
| `docs/IMPLEMENTATION_PLAN.md` | This file |
| `docs/COMPONENT_SPEC.md` | Component specification |

### Modified files

| Path | Changes |
|------|---------|
| `internal/config/angee.go` | Added Environment, SecretsBackend, CredentialBindings, Permissions fields |
| `internal/compiler/compose.go` | Added CredentialOutputs to Compiler, credential binding resolution in compileAgent, LoadCredentialOutputs |
| `cli/root.go` | Registered addCmd, removeCmd, listComponentsCmd, credentialCmd |

### Test files

| Path | Tests |
|------|-------|
| `internal/credentials/env_test.go` | RoundTrip, SecretToEnvKey, DollarEscaping, EmptyFile, Type |
| `internal/config/overlay_test.go` | ScalarOverride, ServiceEnvMerge, AddNewService, SecretDedup, AgentOverride, LoadWithOverlay |
| `internal/config/component_test.go` | LoadComponent (service, credential, module types) |
| `internal/compiler/compose_test.go` | Added: CredentialBindings, CredentialBindings_NilOutputs |
