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
| `services` | Docker containers: web apps, databases, workers, proxies |
| `mcp_servers` | MCP tool servers that agents connect to |
| `agents` | AI agent containers with workspace, MCP, skills |
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
```

Secret name conversion: `db-password` → `DB_PASSWORD` (hyphens to underscores, uppercase).

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
│   │   └── workspace/                 # persistent workspace
│   └── developer/
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

Everything in ANGEE_ROOT is a git repo. `.env`, `operator.yaml`, and agent workspaces are gitignored. The committed state (angee.yaml + docker-compose.yaml) is the deployable truth.

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
6. **Self-managing** — the operator MCP server lets agents deploy, scale, and manage the platform.
7. **Two dependencies** — Cobra + yaml.v3. Everything else is stdlib. OpenBao uses pure net/http.
8. **Backend-agnostic** — RuntimeBackend interface means Docker Compose today, Kubernetes tomorrow. Same angee.yaml.
9. **No magic** — the compiler output (docker-compose.yaml) is readable. You can always inspect what angee generated.

---

## 12. Project Mode

The `angee` binary serves **two complementary modes**, distinguished by what's at the working directory root:

| Mode | Marker | Purpose |
|---|---|---|
| **Compose** | `~/.angee/angee.yaml` (or any dir with one) | Operator state for an agent stack. Sections 1–11 above describe this exclusively. |
| **Project** *(new)* | `<project>/.angee/project.yaml` | A consumer of an Angee runtime framework (e.g. `django-angee`). The Go binary acts as a thin polyglot dispatcher + dev orchestrator. |

These modes are orthogonal — the same shell session can `angee dev` in a project tree and `angee up` against `~/.angee/`. Disambiguation is by manifest filename: `project.yaml` ⇒ project-mode; `angee.yaml` ⇒ compose-mode.

### 12.1 Discovery (parent-walk)

When the user runs `angee build`, `angee migrate`, `angee doctor`, `angee fixtures`, or `angee dev`, the binary walks parents from CWD looking for `.angee/project.yaml` (analogous to `git rev-parse --show-toplevel`).

`cli/root.go:resolveRoot()` is tightened to skip CWD `.angee/` directories that contain `project.yaml` — those are project markers, not compose roots.

### 12.2 `.angee/project.yaml` — Runtime Manifest

```yaml
version: 1
runtime: django-angee                    # adapter selector — see §12.4
django:
  manage_py: src/manage.py               # consumer's runtime entry point
  invoker:   uv                          # uv | python3 | poetry  (resolution order in §12.5)
  uv:
    project: .                           # uv --project <this>; default = project root
  settings:  config.settings             # DJANGO_SETTINGS_MODULE
```

The Python framework remains unaware of `.angee/` — only the consumer's `settings.py` knows the convention. Path resolution in `settings.py` is hardcoded (`BASE_DIR.parent / ".angee" / "data"`); no env-var path overlay in v1.

### 12.3 Dispatcher: `angee {build,migrate,doctor,fixtures}` → runtime

These four are **forwarders**. The binary loads the runtime adapter by `runtime:` field, calls `Adapter.Dispatch(ctx, sub, args)` to build a `Process` (binary, argv, cwd, env), and `syscall.Exec`s it. For `runtime: django-angee` the typical exec is:

```
uv run --project <projectRoot> python <projectRoot>/src/manage.py angee build [args...]
```

`manage.py angee` itself adds **no new subcommands** — only a single new flag, `build --watch`, which the dev orchestrator depends on. The framework's "single entry point" contract from django-angee R-07 is preserved.

### 12.4 Runtime Adapter Interface

```go
// internal/projmode/adapter.go (new top-level package — avoids
// the existing internal/runtime/ which holds RuntimeBackend)
type Adapter interface {
    Name() string                                // "django-angee"
    Dispatch(ctx Ctx, sub string, args []string) (*Process, error)
    Watcher(ctx Ctx)    *Process                 // build --watch (long-running)
    DevServer(ctx Ctx)  *Process                 // runserver (long-running)
    Frontend(ctx Ctx)   *Process                 // pnpm dev (long-running, optional)
    FirstCycleMarker() string                    // line orchestrator waits for
}
```

One concrete impl ships in v1: `internal/projmode/django/`. Future Rust + Node adapters slot in by implementing the same interface — designed for, not built for. See [`RUNTIMES.md`](./RUNTIMES.md) for the adapter-author guide.

### 12.5 Python Resolution

Order of preference for the Django adapter:

1. **`uv run --project <consumerRoot> python …`** — preferred when `uv` is on `PATH` (handles transitive sync).
2. **`<consumerRoot>/.venv/bin/python …`** — fall-back when a venv exists.
3. **`python3` from `PATH`** — last resort; warn the user that no venv was detected.

### 12.6 `angee dev` — The Orchestrator

`angee dev` spawns three long-running children + extras:

| Slot | Source | Default for `django-angee` |
|---|---|---|
| `build` (watcher) | `Adapter.Watcher()` | `uv run python src/manage.py angee build --watch` |
| `runtime` (dev server) | `Adapter.DevServer()` | `uv run python src/manage.py runserver 127.0.0.1:8100` |
| `frontend` | `Adapter.Frontend()` | `pnpm dev` in `ui/react/web` (auto-detect) |
| extras | `pyproject.toml` `[tool.angee.dev.processes.*]` | docker-compose for Postgres/Redis/agents in real consumers |

**Start order:** Watcher → wait for `Adapter.FirstCycleMarker()` (locked: `angee build --watch: ready (cycle 1)`) → DevServer → Frontend → extras (lex order, deterministic). The Python side emits the marker on first successful build cycle so `runserver` never imports stale `runtime/`.

**Output modes:**

- **`--ui=lines`** (default) — line-prefixed, stable per-name colour, colour-only-on-TTY.
- **`--ui=panes`** — full-screen TUI (`charmbracelet/{bubbletea,lipgloss,bubbles}`). One scrollable viewport per child + an "all" tab. Keys: `Tab`/`Shift-Tab` cycle, `q` quits (SIGINT), `r` restarts focused child.
- **IDE auto-detect:** lines mode unless `--ui=panes` explicit, OR stdout is a TTY *and* `$TERM_PROGRAM` is unset / known-good (`tmux`, `screen`, `iTerm.app`, `Apple_Terminal`, `WezTerm`, `kitty`). VSCode/JetBrains embedded terminals get lines by default.

**Process supervision** (`internal/dev/orchestrator.go`):
- Each child in its own session (`Setsid: true` POSIX; `CREATE_NEW_PROCESS_GROUP` Windows).
- SIGINT to orchestrator → SIGTERM to each child group → 10 s grace → SIGKILL stragglers.
- Any non-zero child exit triggers orderly shutdown of remaining + non-zero overall exit.

**Flags:** `--only=<csv>`, `--except=<csv>`, `--no-watch`, `--no-frontend`, `--runtime=<name>`, `--config=<path>`, `--ui=lines|panes`.

### 12.7 `angee init --dev` runtime-only mode

The existing `cli/init.go` flow gains a runtime-only branch: when the loaded `.angee-template.yaml` has `services: []` AND `runtime != ""`, skip docker-compose generation; render only the runtime manifest + gitignore + the `pyproject.toml` merge fragment; create `.angee/data/{staticfiles,mediafiles,logs,tmp}/`; after the existing dev-env flow, run the framework's `migrate` then `loaddata <fixtures>`.

`tmpl.LoadMeta` is extended to parse two new top-level keys: `runtime`, `fixtures`. See [`TEMPLATES.md`](./TEMPLATES.md) for schema details.

### 12.8 Dependencies added for project-mode

- [`github.com/pelletier/go-toml/v2`](https://github.com/pelletier/go-toml/v2) — TOML parser for `pyproject.toml` `[tool.angee.dev.*]` blocks. Replaces the existing "two deps" rule's strict count, but stays minimal.
- `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles` — pane-mode TUI.

### 12.9 Cross-references

- Upstream framework decisions: django-angee `docs/DECISIONS.md` [R-15](../../django-angee/docs/DECISIONS.md#r-15) (project-mode contract), [R-16](../../django-angee/docs/DECISIONS.md#r-16) (polyglot CLI dispatch + orchestrator), [R-14](../../django-angee/docs/DECISIONS.md#r-14) (`angee dev` adapter shape).
- Original orchestrator brief (superseded in part): django-angee `docs/handoff/angee-go-dev-subcommand.md`. See its §13 "Decisions taken" addendum.
- Adapter-author guide: [`RUNTIMES.md`](./RUNTIMES.md).
