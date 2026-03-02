# Angee Architecture & Design Blueprint

This document is the canonical reference for the angee system architecture. It defines every file format, every interface boundary, and every data flow. The refactor targets this design.

---

## 1. System Overview

Angee is a GitOps-driven agent orchestration engine. Three things make up the entire system:

1. **A git repository** (ANGEE_ROOT) — the source of truth. One file (`angee.yaml`) declares the entire stack. Every mutation is a commit. Every rollback is a revert.
2. **A runtime** (Docker Compose today, Kubernetes later) — containers, volumes, networks. The runtime is stateless from angee's perspective — it can always be reconstructed from git.
3. **A secrets backend** (OpenBao or `.env` fallback) — credentials never live in git. They live in a vault.

Two binaries operate on this:

- **`angee`** (CLI) — what the user types. Talks to the operator over HTTP.
- **`angee-operator`** (daemon) — owns ANGEE_ROOT, compiles config, manages the runtime, serves REST API + MCP endpoint.

```
User ──► angee CLI ──► angee-operator (:9000) ──► docker compose ──► containers
                              │        │
                              │        ├── REST API (humans, CI, external tools)
                              │        └── MCP server (AI agents self-managing)
                              │
                              ├── git repo (state)
                              ├── OpenBao (secrets)
                              └── Traefik (routing)
```

The operator is itself a container in the stack. Traefik is another. OpenBao is another. They are the three infrastructure services that every angee stack runs.

---

## 2. File Formats

Angee uses exactly **5 file formats**. Nothing else.

### 2.1 `angee.yaml` — Stack Declaration (source of truth)

The single file that declares the entire platform. Committed to git. This is what users and agents edit.

```yaml
name: my-project                              # required

# --- Connectors (OAuth, API keys, IMAP, SMTP, WhatsApp, etc.) ---
connectors:
  github:
    provider: github                           # well-known: google, github, anthropic
    type: oauth                                # oauth | api_key | token | setup_command
    oauth:
      client_id: "${secret:github-client-id}"
      client_secret: "${secret:github-client-secret}"
      scopes: [repo, read:org]
    env:                                       # how to expose to containers
      GH_TOKEN: oauth_token
    description: "GitHub for repo access"
    required: true

  email-work:
    provider: custom
    type: api_key
    description: "Work IMAP account"
    tags: [email, imap]                            # filterable tags
    metadata:                                      # non-secret connection details
      host: mail.company.com
      port: 993
      username: user@company.com
      ssl: true

  whatsapp:
    provider: custom
    type: setup_command
    setup_command:
      command: [whatsapp-bridge, auth]
      prompt: "Scan QR code to link WhatsApp"
      parse: stdout
    tags: [messaging, whatsapp]

# --- Platform services ---
services:
  web:
    image: myapp:latest
    lifecycle: platform                        # platform|sidecar|worker|system|agent|job
    domains:
      - host: myapp.io
        port: 8000
        tls: true
    env:
      DATABASE_URL: "${secret:db-url}"
    resources:
      cpu: "1.0"
      memory: "1Gi"
    health:
      path: /health
      port: 8000
      interval: 30s
      timeout: 5s
    volumes:
      - name: media
        path: /app/media
        persistent: true
    depends_on: [postgres]

  postgres:
    image: pgvector/pgvector:pg17
    lifecycle: sidecar
    env:
      POSTGRES_PASSWORD: "${secret:db-password}"
    volumes:
      - name: pgdata
        path: /var/lib/postgresql/data
        persistent: true

# --- MCP servers (tool providers for agents) ---
mcp_servers:
  angee-operator:
    transport: streamable-http
    url: http://operator:9000/mcp
    credentials:
      source: service_account
      scopes: [config.read, config.write, deploy, rollback, status, logs, scale]

  filesystem:
    transport: stdio
    image: ghcr.io/fyltr/angee-filesystem-mcp:latest
    command: [node, /usr/local/lib/mcp-filesystem/dist/index.js]
    args: [/workspace]

# --- AI agents ---
agents:
  admin:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system                          # system = always-on
    role: operator                             # operator = full platform access
    description: "Platform admin agent"
    mcp_servers: [angee-operator, filesystem]
    skills: [deploy]
    connectors: [github]                        # shared connector access
    files:
      - template: opencode.json.tmpl
        mount: /root/.config/opencode/opencode.json
    workspace:
      persistent: true
    env:
      ANTHROPIC_API_KEY: "${secret:anthropic-api-key}"

  developer:
    image: ghcr.io/anomalyco/opencode:latest
    lifecycle: system
    role: user
    skills: [code-review]
    connectors: [github, email]
    workspace:
      repository: base
      persistent: true

# --- Reusable agent capabilities ---
skills:
  deploy:
    description: "Deploy, rollback, and manage platform lifecycle"
    mcp_servers: [angee-operator]
    system_prompt: "You can deploy and manage the platform."

  code-review:
    description: "Review PRs and suggest improvements"
    mcp_servers: [filesystem]

# --- Source code repositories ---
repositories:
  base:
    url: https://github.com/you/your-app
    branch: main
    path: src/base

# --- Secrets ---
secrets:
  - name: db-password
    generated: true
    length: 32

  - name: db-url
    derived: "postgresql://app:${db-password}@postgres:5432/mydb"

  - name: anthropic-api-key
    required: true
    description: "Anthropic API key (console.anthropic.com)"

  - name: github-client-id
    required: true

  - name: github-client-secret
    required: true

# --- Secrets backend configuration ---
secrets_backend:
  type: openbao                                # openbao | env
  openbao:
    address: http://openbao:8200
    auth:
      method: token
      token_env: BAO_TOKEN
    prefix: angee
```

#### Top-level sections

| Section | Purpose |
|---------|---------|
| `name` | Project identifier (required) |
| `connectors` | External service connections (OAuth, IMAP, WhatsApp, etc.) shared across agents/services |
| `services` | Docker containers: web apps, databases, workers, proxies |
| `mcp_servers` | MCP tool servers that agents connect to |
| `agents` | AI agent containers with workspace, MCP, skills, connector access |
| `skills` | Reusable capability bundles (MCP servers + prompt) |
| `repositories` | Source code repos linked to agent workspaces |
| `secrets` | Credential declarations (generated, derived, required) |
| `secrets_backend` | Where secrets are stored (OpenBao or .env) |

#### Lifecycles

Every service and agent has a lifecycle that determines restart policy and routing:

| Lifecycle | Restart | Routing | Use case |
|-----------|---------|---------|----------|
| `platform` | unless-stopped | Traefik labels + domains | Web apps, APIs |
| `sidecar` | unless-stopped | internal only | Postgres, Redis, caches |
| `worker` | unless-stopped | internal only | Celery, background jobs |
| `system` | always | varies | Operator, Traefik, always-on agents |
| `agent` | unless-stopped | none | On-demand AI agents |
| `job` | no | none | Migrations, seeds, one-shot tasks |

#### Secret references

In any `env` value: `${secret:name}` → resolved to `${ENV_NAME}` in docker-compose (which reads from `.env`).

#### Connector types

| Type | Flow | Storage |
|------|------|---------|
| `oauth` | Browser redirect → callback → token exchange | access token in vault |
| `api_key` | Interactive prompt during `angee connect` | key in vault |
| `token` | Interactive prompt during `angee connect` | token in vault |
| `setup_command` | Run host command, capture stdout | parsed output in vault |

#### Connector fields

| Field | Purpose |
|-------|---------|
| `provider` | Well-known provider (`google`, `github`, `anthropic`) or `custom` |
| `type` | Auth type: `oauth`, `api_key`, `token`, `setup_command` |
| `env` | Map of env var name → credential field to inject at deploy time |
| `tags` | Filterable labels (e.g., `[email, imap]`, `[messaging, whatsapp]`) |
| `metadata` | Non-secret connection details (host, port, username, ssl) |
| `description` | Human-readable label |
| `required` | Block init if not connected |

#### Connector management

The operator's connector API (`/connectors`) is the **single source of truth**. All connectors — whether added via `angee connect`, by an agent via MCP, or by the application via the operator API — are managed the same way: declaration in `angee.yaml` (git-tracked), credential in OpenBao.

The `env` field is a convenience: if present, the compiler injects the credential as an environment variable at deploy time (useful for agent API keys). But the canonical way for applications to access connectors at runtime is through the operator API — `GET /connectors?tags=email` to discover, `GET /credentials/connector-{name}` to read credentials. This allows dynamic multi-account scenarios (multiple Gmail/IMAP accounts) without redeploying.

Applications use a client library (e.g., `django-angee` for Django) that wraps the operator API, giving them clean access to connectors without knowing about angee.yaml or OpenBao directly.

### 2.2 `operator.yaml` — Local Runtime Config

Runtime-specific settings. **Never committed to git** (gitignored). Lives in ANGEE_ROOT.

```yaml
runtime: docker-compose                    # docker-compose | kubernetes
port: 9000
bind_address: "0.0.0.0"
api_key: "sk-..."                          # optional API auth
cors_origins:
  - "http://localhost:*"

docker:
  socket: /var/run/docker.sock
  network: angee-net

kubernetes:                                # phase 2
  context: my-cluster
  namespace: angee
  ingress_class: nginx
  storage_class: standard
```

### 2.3 `docker-compose.yaml` — Generated Runtime Manifest

**Never manually edited.** Generated by the compiler from `angee.yaml`. Lives in ANGEE_ROOT.

The compiler applies these transformations:

- Lifecycle → restart policy (`system` → `always`, others → `unless-stopped`)
- `${secret:name}` → `${ENV_NAME}` (Docker Compose interpolation from `.env`)
- Platform services with domains → Traefik labels
- Agents prefixed with `agent-` (e.g., `admin` → `agent-admin`)
- MCP server URLs injected as env vars (`ANGEE_MCP_SERVERS`, `ANGEE_MCP_{NAME}_URL`)
- Agent workspace volumes resolved (explicit path > repository > default)
- Connector credentials injected as env vars
- Kubernetes memory format normalized (`1Gi` → `1g`)
- Agent config files rendered from templates
- Single network (`angee-net`) attached to all services

### 2.4 `.env` — Secrets File

**Never committed to git** (gitignored). `KEY=VALUE` format. Generated by `angee init`, managed by credential backend.

```
# Angee secrets — DO NOT COMMIT
DB_PASSWORD=a1b2c3d4...
DB_URL=postgresql://app:a1b2c3d4...@postgres:5432/mydb
ANTHROPIC_API_KEY=sk-ant-...
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
CONNECTOR_GITHUB=gho_...        # connector tokens
CONNECTOR_EMAIL=...
```

Secret name conversion: `db-password` → `DB_PASSWORD` (hyphens to underscores, uppercase).
Connectors stored as: `CONNECTOR_{NAME}` → credential value.

### 2.5 `.angee-template.yaml` — Template Metadata

Describes a template that `angee init` can use to scaffold a stack.

```yaml
name: official/default
description: "Operator + Traefik + OpenBao + admin agent"
version: "1.0"

parameters:
  - name: ProjectName
    description: "Project name (lowercase, hyphens)"
    required: true
    validate: "^[a-z][a-z0-9-]+$"
  - name: Domain
    description: "Primary domain"
    default: "localhost"

secrets:
  - name: django-secret-key
    generated: true
    length: 50
  - name: db-url
    derived: "postgresql://app:${db-password}@postgres:5432/${project}"
```

Templates contain `angee.yaml.tmpl` (Go `text/template`) that renders into `angee.yaml` using the declared parameters.

---

## 3. ANGEE_ROOT Filesystem

```
~/.angee/                              # or ANGEE_ROOT env, or .angee/ in project
├── angee.yaml                         # source of truth (committed)
├── docker-compose.yaml                # generated by compiler (gitignored or committed)
├── operator.yaml                      # local runtime config (gitignored)
├── .env                               # secrets (gitignored)
├── .gitignore
│
├── agents/                            # per-agent directories
│   ├── admin/
│   │   ├── .env                       # agent-specific env (connector creds)
│   │   └── workspace/                 # persistent workspace
│   └── developer/
│       ├── .env
│       └── workspace/
│
├── src/                               # cloned repositories
│   └── base/                          # from repositories.base
│
├── components/                        # installed component records
│   ├── postgres.yaml
│   └── redis.yaml
│
├── credential-outputs/                # cached component credential outputs
│   └── postgres.yaml
│
└── templates/                         # rendered config file templates
    └── opencode.json.tmpl
```

Everything in ANGEE_ROOT is a git repo. `.env`, `operator.yaml`, agent workspaces, and agent `.env` files are gitignored. The committed state (angee.yaml + docker-compose.yaml) is the deployable truth.

---

## 4. Package Architecture

```
cmd/
├── angee/                             # CLI binary entry point
│   └── main.go                        #   → cli.Execute()
└── operator/                          # Operator binary entry point
    └── main.go                        #   → operator.New() + Start()

cli/                                   # CLI command implementations
├── root.go                            # global flags: --root, --operator, --json, --api-key
├── init.go                            # angee init (scaffold, secrets, templates)
├── up.go                              # angee up (compile + docker compose up)
├── deploy.go                          # angee deploy (POST /deploy)
├── plan.go                            # angee plan (GET /plan)
├── rollback.go                        # angee rollback (POST /rollback)
├── ls.go                              # angee ls (GET /status)
├── logs.go                            # angee logs (GET /logs)
├── chat.go                            # angee chat/admin/develop (docker exec)
├── connect.go                         # angee connect (connectors)
├── credential.go                      # angee credential (secrets management)
├── add.go                             # angee add (components)
├── remove.go                          # angee remove (components)
└── ...

api/
└── types.go                           # shared request/response types (CLI ↔ operator)

internal/
├── config/                            # YAML config types + validation
│   ├── angee.go                       #   AngeeConfig and all sub-specs
│   ├── operator.go                    #   OperatorConfig
│   ├── component.go                   #   ComponentConfig
│   └── validate.go                    #   structural validation rules
│
├── compiler/                          # angee.yaml → docker-compose.yaml
│   ├── compose.go                     #   compilation logic + secret resolution
│   └── hooks.go                       #   agent file rendering (templates → mounted files)
│
├── runtime/                           # runtime backend abstraction
│   ├── backend.go                     #   RuntimeBackend interface
│   └── compose/
│       └── backend.go                 #   Docker Compose implementation
│
├── operator/                          # HTTP server + handlers
│   ├── server.go                      #   Server struct, routing, middleware
│   ├── handlers.go                    #   deploy, rollback, status, logs, agents, config
│   ├── handlers_credentials.go        #   credential CRUD endpoints
│   ├── handlers_connectors.go        #   Connector flows (OAuth, status, list)
│   ├── mcp.go                         #   MCP JSON-RPC 2.0 endpoint + tool dispatch
│   ├── healthcheck.go                 #   active health probing
│   └── openapi.go                     #   OpenAPI 3.1 schema generation
│
├── credentials/                       # credential storage backends
│   ├── backend.go                     #   Backend interface (Get/Set/Delete/List)
│   ├── env.go                         #   .env file backend (default fallback)
│   └── openbao/
│       └── openbao.go                 #   OpenBao KV v2 backend (pure net/http)
│
├── secrets/                           # secret resolution during init
│   └── resolve.go                     #   generated, derived, required resolution
│
├── root/                              # ANGEE_ROOT filesystem management
│   └── root.go                        #   directory creation, config I/O, git integration
│
├── git/                               # git CLI wrapper
│   └── git.go                         #   init, commit, revert, log, clone, etc.
│
├── tmpl/                              # template system
│   └── fs.go                          #   FetchTemplate, render, parameter handling
│
└── component/                         # component add/remove
    └── add.go                         #   resolve, fetch, merge into angee.yaml
```

### Dependency rule

No package imports a "higher" package. The dependency graph flows strictly downward:

```
cmd/ → cli/ → api/
               ↓
         internal/operator/ → internal/compiler/
                            → internal/runtime/
                            → internal/credentials/
                            → internal/root/ → internal/git/
                            → internal/config/
```

External dependencies: **two only** — `github.com/spf13/cobra` (CLI) and `gopkg.in/yaml.v3` (config). OpenBao uses pure `net/http`.

---

## 5. Operator REST API

Base: `http://localhost:9000`

Auth: `Authorization: Bearer <api-key>` (when configured). `/health` and `/openapi.json` are always public.

### Configuration

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | Liveness probe |
| GET | `/config` | Read angee.yaml as JSON |
| POST | `/config` | Write angee.yaml, validate, git commit, optionally deploy |

### Deployment

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/deploy` | Compile angee.yaml → docker-compose.yaml, apply |
| GET | `/plan` | Dry-run: show what would change |
| POST | `/rollback` | Revert to SHA, recompile, redeploy |

### Runtime

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/status` | All services + agents with health |
| GET | `/logs/{service}` | Stream logs (query: follow, lines, since) |
| POST | `/scale/{service}` | Set replica count |
| POST | `/down` | Stop entire stack |

### Agents

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/agents` | List agents with status |
| POST | `/agents/{name}/start` | Start agent |
| POST | `/agents/{name}/stop` | Stop agent |
| GET | `/agents/{name}/logs` | Agent logs |

### Connectors

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/connectors` | List all connectors + status (query: `tags`, `provider`) |
| POST | `/connectors` | Create a new connector (declaration + credential) |
| GET | `/connectors/{name}` | Get connector details |
| PATCH | `/connectors/{name}` | Update connector metadata/tags |
| DELETE | `/connectors/{name}` | Remove connector + credential |
| GET | `/connectors/{name}/start` | Initiate OAuth flow (redirects to provider) |
| GET | `/connectors/callback` | OAuth callback (exchanges code for token) |
| GET | `/connectors/{name}/status` | Check if connector is connected |

### Credentials

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/credentials` | List all credentials |
| POST | `/credentials` | Set a credential |
| DELETE | `/credentials/{name}` | Delete a credential |

### History

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/history` | Git commit log (query: n=20) |
| GET | `/openapi.json` | OpenAPI 3.1 schema |

---

## 6. MCP Server (Agent Self-Management)

Endpoint: `POST /mcp` — JSON-RPC 2.0 over HTTP.

This is how AI agents running inside the stack manage the platform. The operator MCP server exposes these tools:

### Platform tools
- `platform_health` — operator liveness
- `platform_status` — all services + agents state
- `platform_down` — bring stack down

### Config tools
- `config_get` — read angee.yaml
- `config_set` — write + validate + commit + optionally deploy

### Deploy tools
- `deploy` — compile and apply
- `deploy_plan` — dry-run diff
- `deploy_rollback` — revert to SHA

### Service tools
- `service_logs` — get logs for any service
- `service_scale` — scale a service

### Agent tools
- `agent_list` — list all agents
- `agent_start` — start an agent
- `agent_stop` — stop an agent
- `agent_logs` — get agent logs

### Credential tools
- `credentials_list` — list stored credentials
- `credentials_set` — store a credential
- `credentials_delete` — remove a credential

### History tools
- `history` — git commit log

---

## 7. Core Interfaces

### RuntimeBackend

The single abstraction for runtime orchestration. Docker Compose today, Kubernetes later.

```go
type RuntimeBackend interface {
    Diff(ctx, composeFile)   → (*ChangeSet, error)    // what would change
    Apply(ctx, composeFile)  → (*ApplyResult, error)   // make it so
    Pull(ctx)                → error                    // pull latest images
    Status(ctx)              → ([]*ServiceStatus, error) // current state
    Logs(ctx, service, opts) → (io.ReadCloser, error)  // stream logs
    Scale(ctx, service, n)   → error                    // set replicas
    Stop(ctx, services...)   → error                    // stop services
    Down(ctx)                → error                    // tear down everything
}
```

### Credential Backend

Pluggable secret storage.

```go
type Backend interface {
    Get(name)        → (string, error)
    Set(name, value) → error
    Delete(name)     → error
    List()           → ([]string, error)
    Type()           → string              // "env" | "openbao"
}
```

Implementations:
- **EnvBackend** — reads/writes `.env` file (default fallback, dev mode)
- **OpenBaoBackend** — KV v2 via pure `net/http` (production)

---

## 8. Key Flows

### Deploy

```
CLI: angee deploy -m "add redis"
  → POST /deploy to operator
  → Load angee.yaml from git
  → Validate config
  → EnsureAgentDirs (create agents/*/workspace/)
  → RenderAgentFiles (templates → mounted configs)
  → Compile (angee.yaml → docker-compose.yaml)
  → Write docker-compose.yaml to disk
  → Backend.Apply() → docker compose up -d
  → Git commit angee.yaml
  → Restart health probes
  ← ApplyResult {started, updated, removed}
```

### Rollback

```
CLI: angee rollback HEAD~1
  → POST /rollback {sha} to operator
  → Git revert to SHA
  → Load reverted angee.yaml
  → Full deploy flow (compile → apply)
  ← RollbackResponse {rolled_back_to, deploy}
```

### Connect (OAuth)

```
CLI: angee connect github
  → Print URL: http://localhost:9000/connectors/github/start
  → User opens browser → redirected to GitHub OAuth
  → GitHub calls back: GET /connectors/callback?code=...&state=...
  → Operator exchanges code for token
  → Token stored in credential backend as connector-github
  → CLI polls GET /connectors/github/status until connected
```

### Agent Self-Management

```
Agent (inside container):
  → POST /mcp {method: "tools/call", params: {name: "deploy"}}
  → Operator compiles + applies
  → Agent receives ApplyResult via JSON-RPC response
```

---

## 9. Infrastructure Services

Every angee stack runs three infrastructure services (marked `infrastructure: true` in the default template):

### angee-operator
- Image: `ghcr.io/fyltr/angee-operator:latest`
- Port: 9000
- Mounts: docker socket + ANGEE_ROOT
- Purpose: REST API + MCP server + compilation + deployment

### traefik
- Image: `traefik:v3`
- Ports: 80 (HTTP), 443 (HTTPS)
- Mounts: docker socket (read-only)
- Purpose: reverse proxy, TLS termination, domain routing for platform services

### openbao
- Image: `openbao/openbao:2`
- Port: 8200
- Purpose: secrets backend (KV v2), replaces manual .env management in production

---

## 10. Component System

Components are pre-packaged service definitions that can be added to any stack.

```bash
angee add postgres      # adds postgres service + db-password secret
angee add redis         # adds redis service
angee remove postgres   # removes it
```

Component definition (`component.yaml` or `angee-component.yaml`):

```yaml
name: postgres
description: "PostgreSQL with pgvector"
version: "1.0"

services:
  postgres:
    image: pgvector/pgvector:pg17
    lifecycle: sidecar
    env:
      POSTGRES_PASSWORD: "${secret:db-password}"
    volumes:
      - name: pgdata
        path: /var/lib/postgresql/data
        persistent: true

secrets:
  - name: db-password
    generated: true
    length: 32

credential_outputs:
  - name: db-url
    template: "postgresql://postgres:${db-password}@postgres:5432/postgres"
```

Resolution order: local path → embedded (`templates/components/`) → shorthand name → git URL.

Install records stored in `.angee/components/{name}.yaml`.

---

## 11. Design Principles

1. **One file** — `angee.yaml` declares everything. No scattered configs.
2. **GitOps** — git is the state store. Every change is a commit. Every rollback is a revert.
3. **Secrets never in git** — credentials live in OpenBao or `.env`, both gitignored.
4. **Compiler, not runtime** — angee compiles to docker-compose.yaml. Docker Compose is the runtime. Clean separation.
5. **Agents are containers** — no special runtime. An agent is a Docker container with MCP connections and a persistent workspace.
6. **Shared connectors** — connectors are stack-level resources, shared across agents and services.
7. **Self-managing** — the operator MCP server lets agents deploy, scale, and manage the platform.
8. **Two dependencies** — Cobra + yaml.v3. Everything else is stdlib. OpenBao uses pure net/http.
9. **Backend-agnostic** — RuntimeBackend interface means Docker Compose today, Kubernetes tomorrow. Same angee.yaml.
10. **No magic** — the compiler output (docker-compose.yaml) is readable. You can always inspect what angee generated.
