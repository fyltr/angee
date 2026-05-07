# Angee Refactor Overview v2

Status: target clean design, scoped to the two things Angee actually exists to make easy.

This revision narrows v1 to the two user goals and explicitly delegates everything else to existing tools. It supersedes `OVERVIEW.md`. The earlier document tried to redesign the world above docker-compose; this one only builds what is genuinely missing.

## The Two Goals

Angee exists to make these two flows boring and reproducible:

1. **Set up an environment with secrets and services.** One manifest, one command, get a running stack with secrets resolved and services healthy.
2. **Provision an agent with a templated workspace, a worktree to work in, secrets, MCP wiring, and a known address to contact it.** One command, get an isolated AI agent that can read/write code in its own git worktree, talk to MCPs (filesystem, operator, playwright, app), and be controlled over HTTP/MCP from the application.

Anything not in service of those two goals is out of scope for v1.

## Reuse Stance

Angee is not a new orchestrator. It is a thin Go binary that composes existing tools and adds one novel provisioning primitive (the Agent bundle).

| Layer | Reused tool | Why |
|---|---|---|
| Container runtime | `docker compose` (shell out) | Already does up/down/start/stop/restart/logs/networks/volumes. Angee's commands are one-line shims. |
| Local-process supervisor | `process-compose` ([F1bonacc1/process-compose](https://github.com/F1bonacc1/process-compose)) — shell out to the binary, control via its REST API | Already does depends_on, health checks, readiness probes, env files, TUI, cron, REST API. Angee generates `process-compose.yaml`, starts the daemon, and drives it via HTTP. Embedding the Go packages directly is technically possible but pulls Gin + bubbletea + the full scheduler — not worth the dep weight. |
| Templating | **`github.com/fyltr/copier-go`** — embedded as a Go library (in-tree at `../copier-go`) | Native Go reimplementation of Copier including `copier update`. No Python runtime dependency. Replaces shelling out to `pip install copier`. |
| Secrets | Pluggable backend interface; v1 implements `.env` and `openbao` natively, accepts `env:` imports | Angee owns `${secret:name}` resolution at compile time. The interface is shaped so a Teller-compatible adapter is feasible if needed. |
| Worktrees | `git worktree add` via stdlib `os/exec` | No wrapper library is worth a dependency. |
| MCP servers | Standard MCP servers (`@modelcontextprotocol/server-filesystem`, `@playwright/mcp`, etc.) | Angee declares them per agent; the agent runtime starts them. |

What stays Angee's:
- The `angee.yaml` manifest schema and the `angee.yaml → docker-compose.yaml` + `angee.yaml → process-compose.yaml` compilers.
- The Copier `_angee` metadata convention.
- `${secret:name}` resolution at compile time.
- **The Agent provisioning primitive** — one command that creates a worktree, materializes workspace sources, wires MCP servers, injects secrets, starts a compose service, and registers an addressable agent.
- The Operator HTTP + MCP API that exposes agent provisioning to the application layer.

## Architecture

```
angee CLI (cobra)
    │
    ▼
service.Platform   ◄── one business-logic layer; CLI/HTTP/MCP are thin adapters
    │
    ├─► docker compose                       (shell out — containerized services)
    ├─► process-compose binary + REST API    (shell out to start, HTTP to control — local processes)
    ├─► git worktree                         (shell out — per-agent code workspace)
    ├─► copier-go (library)                  (in-process — template render + update)
    └─► secrets backend                      (in-process — .env | openbao)
```

CLI default is in-process Platform; `--operator` / `ANGEE_OPERATOR_URL` switches to HTTP. No command implements provisioning or runtime logic directly.

## Core Concepts

Five concepts. Workspace is no longer a top-level concept — it's an implementation detail of an Agent.

| Concept | Meaning |
|---|---|
| Stack | The environment: services, jobs, agents, secrets, ports, sources, volumes. One per `ANGEE_ROOT`. |
| Service | A runnable workload. `runtime: container` compiles to compose; `runtime: local` compiles to process-compose. |
| Job | A one-shot or scheduled command (migrations, fixtures, asset builds). Compiles to a one-shot compose service or a process-compose task. |
| Agent | A bundle: one service + one git worktree + one materialized workspace + declared MCP servers + injected secrets + a known operator-addressable name. |
| Operator | The reconciler. Renders templates, resolves secrets, compiles backend files, applies runtime changes, and exposes HTTP + MCP endpoints. |

## Command Surface

```sh
# Stack
angee init --dev [path]                  # alias for: angee stack init dev [path]
angee stack init <template> [path]
angee stack update
angee stack destroy

# Runtime power (literal compose / process-compose shims)
angee up [service...]
angee down
angee start [service...]
angee stop [service...]
angee restart [service...]
angee logs [service...]
angee status                             # alias: ls, ps
angee dev                                # runs process-compose for local + compose for containers

# Services
angee service init|update|destroy <name>
angee service start|stop|restart|logs <name>
angee service list

# Jobs
angee job run <name>
angee job list
angee job logs <name>

# Agents (the novel primitive)
angee agent init <name> [--template ...] [--source key=ref ...] [--start] [--yes]
angee agent update <name>
angee agent destroy <name>
angee agent start|stop|restart|logs <name>
angee agent list

# Operator
angee operator [--bind 0.0.0.0] [--port 9000]
```

`angee init/up/start/stop/down/restart/logs` collectively contain near-zero custom logic. Everything between the user typing `angee up` and `docker compose up -d` is secret resolution + port substitution, nothing else.

## Manifest: `$ANGEE_ROOT/angee.yaml`

One file. Source of truth. Generated by Copier from a stack template; users may edit it; template updates are the preferred path for structural changes.

```yaml
version: 1
kind: stack
name: angee-notes

template:
  active: stacks/angee-notes-dev
  answers_file: .copier-answers.yml

operator:
  url: http://127.0.0.1:9000
  token_secret: operator-token         # references secrets.operator-token
  service_registration:
    app: angee-notes
    environment: dev
    mcp_url: http://127.0.0.1:9000/mcp

secrets_backend:
  type: env-file                       # env-file | openbao
  path: .angee/.env

secrets:
  django-secret-key: { generated: true, length: 50 }
  operator-token:    { generated: true, length: 32 }
  anthropic-api-key: { required: true } # supplied via --secret name=env:ANTHROPIC_API_KEY

ports:
  django: { value: 8100, export_env: DJANGO_PORT }
  react:  { value: 5173, export_env: REACT_PORT }

volumes:
  app-data: { driver: local-fs, path: .angee/data }

sources:
  app: { kind: local, path: ../django-angee/examples/angee-notes }

services:
  web:
    runtime: local                     # → process-compose
    source: app
    command: ["uv", "run", "python", "manage.py", "runserver", "127.0.0.1:${ports.django}"]
    env_file: .angee/.env
    after: [migrate]

  postgres:
    runtime: container                 # → docker compose
    image: postgres:16
    env:
      POSTGRES_PASSWORD: "${secret:postgres-password}"

jobs:
  migrate:
    runtime: local
    source: app
    command: ["uv", "run", "python", "manage.py", "migrate", "--noinput"]

agents:
  default_template: agents/angee-developer
```

## Goal 1: Environments with Secrets + Services

The flow, end to end:

```sh
angee init --dev --yes        # copier render → angee.yaml + .copier-answers.yml
angee up                      # generate compose + process-compose; resolve secrets; start everything
angee logs web --follow
angee down
```

Compilation pipeline on every `up` / `dev` / `start`:

1. Load `angee.yaml`.
2. Resolve secrets through the configured backend (`.env` or OpenBao). Generate any `generated: true` secrets that don't exist yet. Import `env:VAR` references from the host environment and persist into the backend.
3. Substitute `${secret:name}` and `${ports.name}` references.
4. Split services by `runtime`:
   - `runtime: container` → emit `docker-compose.yaml`.
   - `runtime: local` → emit `process-compose.yaml`.
5. For `angee up`: run `docker compose up -d`. Skip process-compose (containers only).
6. For `angee dev`: run `docker compose up -d` for sidecars, then `process-compose up` for local services, with cross-boundary `depends_on` resolved by readiness probes (process-compose can wait on HTTP/exec health of compose services, and vice versa via health endpoints).
7. For `angee down/stop/restart/logs`: shell out to whichever backend(s) own the named services.

### Secrets backend interface

```go
type Backend interface {
    Get(ctx context.Context, key string) (string, bool, error)
    Set(ctx context.Context, key, value string) error
    Delete(ctx context.Context, key string) error
    List(ctx context.Context) ([]string, error)
}
```

v1 implementations: `EnvFileBackend` (reads/writes `.angee/.env`), `OpenBaoBackend` (KV v2 over HTTP). Adding a backend later (Doppler, AWS SSM, Infisical) is one struct that satisfies the interface. The interface is deliberately Teller-shaped so a Teller adapter is feasible if needed.

Secret values are never written to `angee.yaml`. Only references and declarations are committed.

## Goal 2: Agent Provisioning

This is the only place Angee builds something the ecosystem doesn't already have. One command does everything an AI agent needs to start coding against a repo.

```sh
angee agent init reviewer \
  --template agents/angee-developer \
  --source app=https://github.com/fyltr/app#feature-x \
  --secret anthropic-api-key=env:ANTHROPIC_API_KEY \
  --start --yes
```

What `agent init` does, atomically:

1. **Resolve agent template** (`agents/angee-developer`) via Copier.
2. **Render scaffold** into `$ANGEE_ROOT/agents/reviewer/`: `agent.yaml`, `.env`, `CLAUDE.md`, `.mcp.json`, helper scripts.
3. **Materialize workspace sources** into `$ANGEE_ROOT/agents/reviewer/workspace/`:
   - `kind: git` with `mode: worktree` → `git worktree add <worktree_path> -b <branch> <base_branch>`, then symlink/bind into `workspace/code/`. Worktree lives outside `ANGEE_ROOT` so it can host a normal dev environment.
   - `kind: git` (clone mode) → shallow clone with locked commit.
   - `kind: template` → `copier copy` of a workspace template into a subpath.
   - `kind: url|archive` → download + checksum verify + extract.
   - `kind: local` → bind or copy.
   - `kind: volume` → mount a declared volume at a subpath; `scope: agent` derives a per-agent volume.
4. **Wire MCP servers** declared in the agent template:
   - `filesystem` (stdio, scoped to workspace root)
   - `operator` (HTTP, points at `${operator.url}/mcp` with the operator token)
   - `playwright` (HTTP, started as a local process by Angee on a per-agent port)
   - Any app-specific MCPs (e.g., a Django app MCP)
   Emit `.mcp.json` so tools like Claude Code pick them up automatically.
5. **Inject secrets** into the agent's `.env` and into the compose service environment via `${secret:name}`.
6. **Register the bundled service** in `angee.yaml` under `services.<name>`. The service mounts the workspace and runs the agent process (`claude`, `codex`, etc.) declared by the template.
7. **Compute the agent's address** from the operator config: the agent is reachable as `${operator.url}/agents/reviewer/...` and addressable by name in MCP tool calls. This address is written into the agent's own `.env` so the agent process can introspect its own identity.
8. **Start** the bundled service if `--start` is set.

The result on disk:

```
$ANGEE_ROOT/
  angee.yaml                                # reviewer is now a registered agent
  .env
  docker-compose.yaml
  process-compose.yaml
  agents/reviewer/
    agent.yaml                              # agent-specific manifest, rendered
    .env                                    # agent-scoped env (incl. its own operator URL/token)
    CLAUDE.md
    .mcp.json
    scripts/start-playwright-mcp.sh
    workspace/
      code/        → ../../../django-angee.workspaces/feat-reviewer/  (git worktree)
      skills/                                # persistent: true
      memory/                                # volume-backed
      docs/                                  # archive source
```

The worktree itself lives at e.g. `../django-angee.workspaces/feat-reviewer/`, branched off `dev` onto `feat/reviewer-feature-x`. The agent edits there. Pushes go to that branch. The application opens PRs against the base branch.

### Addressability

Every agent has a stable name (`reviewer`) and a stable URL (`${operator.url}/agents/reviewer`). The application stores only the name. To contact an agent:

- HTTP: `POST ${operator.url}/agents/reviewer/messages` (or whatever the bundled service exposes; the operator proxies).
- MCP: the agent process reads `.mcp.json` and connects to the operator's MCP endpoint with its bearer token. The operator's MCP tools include `agents.list`, `agents.start`, `agents.stop`, `agents.send_message`.
- Logs: `angee agent logs reviewer` or `GET ${operator.url}/agents/reviewer/logs`.

### Lifecycle

| Command | Implementation |
|---|---|
| `agent init` | Composite provisioning above. |
| `agent update` | `copier update` on agent template, re-materialize sources whose lock/ref changed, rewrite agent.yaml + service registration, optionally restart. |
| `agent start/stop/restart` | Same as `service start/stop/restart` on the bundled service. |
| `agent logs` | Same as `service logs`. |
| `agent destroy` | Stop service, remove service from manifest. Worktree and persistent sources kept by default; `--purge` removes them. |
| `agent list` | Join services tagged `agent` with their agent.yaml metadata. |

## Templates

Copier templates with `_angee` metadata in `copier.yml`. Three families:

```
templates/stacks/<name>/      # one rendered angee.yaml + supporting files
templates/agents/<name>/      # one rendered agent.yaml + workspace source declarations + MCP wiring
templates/workspaces/<name>/  # only useful as inputs to agent templates (kind: template sources)
```

`_angee` metadata declares: `kind`, default ports, default agent template, MCP servers the agent template wires up, `local_runtime` hints, source-mode defaults (e.g. `git-worktree`).

Rendering is done in-process by `github.com/fyltr/copier-go`. It implements `copier copy`, `copier update`, and the answers-file lifecycle natively in Go, so there is no Python runtime requirement. `internal/copier/` becomes a thin wrapper around the library — call its `Render` and `Update` APIs directly from `service.Platform`. `copier update` is what makes `angee stack update` and `angee agent update` viable; reimplementing template diff/merge logic is exactly the work the library has already done.

## Operator HTTP + MCP API

Thin adapter over `service.Platform`. Same surface for HTTP and MCP.

```
POST   /stack/up                       POST /stack/down       GET  /stack/status
POST   /services/init                  POST /services/{n}/{start|stop|restart|destroy}
GET    /services/{n}/logs              GET  /services
POST   /jobs/{n}/run                   GET  /jobs             GET  /jobs/{n}/logs
POST   /agents                         POST /agents/{n}/update
POST   /agents/{n}/{start|stop|restart|destroy}
GET    /agents/{n}/logs                GET  /agents           GET  /agents/{n}
POST   /agents/{n}/messages            (proxied to the bundled service)
GET    /operations/{id}                (async provisioning status)
GET    /events                         (SSE stream of operator events)
GET    /mcp                            (MCP JSON-RPC; same operations as tools)
```

Bearer auth required for non-loopback binds. One operator serves exactly one `ANGEE_ROOT`.

## Operator-as-Service: Spinning Up Agents From an External Runtime

This is the core Goal-2 integration story: a Django app needs to create a new agent from a template, on demand, with its own ports, and contact it.

### Lifecycle

```
Django                              angee-operator                       host
  │                                       │                                │
  │ POST /agents { template, answers,     │                                │
  │   sources, secrets, start: true }     │                                │
  ├──────────────────────────────────────►│                                │
  │ 202 { name, operation_id, status_url }│                                │
  │◄──────────────────────────────────────┤                                │
  │                                       │ render copier-go               │
  │                                       │ git worktree add               │
  │                                       │ resolve secrets                │
  │                                       │ allocate ports from pool       │
  │                                       │ write angee.yaml entry         │
  │                                       │ docker compose up -d <svc>     │
  │                                       │   (or process-compose start)   │
  │                                       │                                │
  │ GET /events  (SSE)                    │                                │
  ├──────────────────────────────────────►│                                │
  │ ◄── agent.provisioning ──             │                                │
  │ ◄── agent.healthy { url, mcp_url } ──│                                │
  │                                       │                                │
  │ POST /agents/reviewer-123/messages    │                                │
  ├──────────────────────────────────────►│ proxy to bundled service       │
  │                                       ├───────────────────────────────►│
```

### `POST /agents` request

```json
{
  "name": "reviewer-123",
  "template": "agents/angee-developer",
  "answers": {
    "branch": "feat/issue-123",
    "base_branch": "main",
    "model": "claude-sonnet-4-5"
  },
  "sources": {
    "code": { "repo": "https://github.com/fyltr/app", "ref": "main" }
  },
  "secrets": {
    "anthropic-api-key": "env:ANTHROPIC_API_KEY"
  },
  "ports": {
    "playwright": "auto"
  },
  "start": true,
  "ttl": "24h"
}
```

Response (202 Accepted):

```json
{
  "name": "reviewer-123",
  "operation_id": "op_01J...",
  "status_url": "/operations/op_01J...",
  "agent_url": "/agents/reviewer-123"
}
```

Provisioning is async because it can take minutes (clone, copier render, image pull, container start, health gate). The client either polls `GET /operations/{id}` or subscribes to `GET /events` for SSE.

### Port leasing

The operator owns a port pool declared in `angee.yaml`:

```yaml
operator:
  port_pool:
    django:     { range: "8200-8299" }
    react:      { range: "5300-5399" }
    playwright: { range: "9300-9399" }
    custom:     { range: "10000-10999" }
```

When a template asks for a port (`ports.django: auto`), the operator picks an unused port from the matching pool, persists the assignment in `angee.yaml` under the agent's entry, and propagates the concrete value into the rendered scaffold + compose/process-compose files. The lease is released on `agent destroy` or TTL expiry.

Two principles:
1. **Concrete ports always end up in `angee.yaml`.** The operator never relies on dynamic Docker port assignment for agent-facing services — Django needs deterministic URLs.
2. **The pool ranges are the operator's contract with the host.** Whoever provisions the operator host decides the pool size, which caps concurrent agents.

### Addressability

After provisioning, `GET /agents/reviewer-123` returns:

```json
{
  "name": "reviewer-123",
  "status": "healthy",
  "template": "agents/angee-developer",
  "service": "reviewer-123",
  "url": "http://operator.example.com:8247",        // the agent's own HTTP
  "mcp_url": "http://operator.example.com:8247/mcp",
  "operator_proxy_url": "https://operator.example.com/agents/reviewer-123",
  "ports": { "django": 8247, "playwright": 9312 },
  "worktree": {
    "path": "/srv/angee/worktrees/reviewer-123",
    "branch": "feat/issue-123",
    "base": "main",
    "commit": "ab12cd34..."
  },
  "created_at": "...",
  "ttl_expires_at": "..."
}
```

Two addressability paths exist on purpose:

- **Direct URL** (`http://host:8247`) — fast, agent talks to its own HTTP/MCP. Use when the app and operator share a network.
- **Operator proxy** (`https://operator/agents/reviewer-123`) — same TLS termination + auth as the operator, route to the right backend. Use over the public internet.

Django stores only the agent name and the operator URL. Everything else is looked up.

### MCP wiring across the boundary

The agent's `.mcp.json` is rendered with the operator URL and a per-agent token:

```json
{
  "operator": {
    "transport": "streamable-http",
    "url": "http://operator.example.com:9000/mcp",
    "headers": { "Authorization": "Bearer <agent-scoped-token>" }
  }
}
```

The operator-scoped token is minted at agent provisioning and only allows operations on that agent's own resources (read its workspace, write logs, mark itself ready, request a redeploy). The token is rotated on `agent update`.

### PR flow

The agent works in its own worktree, branched from the requested base. When the agent decides it's done:

1. The agent commits and pushes its branch directly (its `.env` includes a scoped git credential — typically a deploy key or short-lived GitHub App token, supplied by Django at provisioning time as a secret).
2. Either the agent calls a Django webhook to "open the PR" (Django has the GitHub App auth for PR creation), or the agent has its own narrowly-scoped GitHub token and opens the PR itself.

Angee's responsibility ends at "the worktree exists, the branch is pushed." PR opening is policy that lives in the application layer — Django decides who opens PRs on which repos under which identity.

### Multi-tenancy and isolation

One operator per trust boundary. For Django shipping multiple environments:

- **Per-environment operator** — staging gets its own operator + ANGEE_ROOT + port pool; prod gets another.
- **Per-tenant operator** — if Django serves multiple end-customers and each customer's agents must not see each other's worktrees or volumes, run one operator per tenant. The operator binary is small; spin up multiple.

The operator is not designed to multiplex tenants inside one ANGEE_ROOT. Pushing tenancy into one process means re-deriving auth/quota/isolation primitives that Linux containers + separate operator processes already give us.

### TTLs and cleanup

Every agent created by an external runtime should have a TTL. The operator runs a periodic sweep (or honors `ttl_expires_at` on each agent record) and calls `agent destroy` when expired. Django can extend a TTL with `POST /agents/{n}` and a new `ttl` value.

Without TTLs, ephemeral PR-review agents accumulate worktrees and ports forever. With TTLs, garbage collection is the operator's job, not Django's.

## `angee dev`

Ephemeral in-process operator runtime. Generates `process-compose.yaml` from the local services + jobs, generates `docker-compose.yaml` from the container services, then:

```
docker compose up -d <container-services>      # sidecars (postgres, redis, openbao, etc.)
process-compose up --config process-compose.yaml \
  --port <auto>                                # daemon mode; we control via its REST API
```

`process-compose` already provides: depends_on with health gating, readiness probes, log fanout, TUI, REST API for control, env files, cron-like scheduling. Angee adds nothing to its supervisor loop.

For control beyond start/stop, Angee uses process-compose's REST API (or the `src/client` Go client wrapping it) — `GET /processes` for status, `POST /process/start/{name}`, `POST /process/stop/{name}`, `GET /process/logs/{name}`. The `angee` CLI commands `start`/`stop`/`restart`/`logs` route to either docker compose or the process-compose REST endpoint depending on which backend owns the named service.

On Ctrl+C: process-compose handles graceful shutdown of local processes; container sidecars stay up unless template policy says otherwise.

## ANGEE_ROOT Layout

```
$ANGEE_ROOT/
  angee.yaml                  # source of truth, committed
  .copier-answers.yml         # committed
  docker-compose.yaml         # generated, committed (or gitignored — template choice)
  process-compose.yaml        # generated, committed
  .env                        # gitignored when secrets_backend.type == env-file
  agents/<name>/              # rendered scaffold + materialized workspace
  volumes/                    # local persistent volumes when local-fs driver used
  run/                        # volatile operator working files, gitignored
```

No `state/` directory. No `operator.yaml`. The manifest is the state.

## Out Of Scope For v1

Explicitly deferred so the v1 ships:

- Kubernetes runtime backend.
- Durable workflow engine.
- ReBAC / SpiceDB authorization (use bearer auth until a real need surfaces).
- Bidirectional database ↔ file-backend sync.
- Generic mount type system beyond what Compose already supports (`bind`, `volume`, `tmpfs`).
- Custom secrets backends beyond `.env` + OpenBao.
- A web UI.
- Workspace as a standalone CLI noun (folded into agent).
- Framework-specific dispatch (Django, Vite, pnpm, uv) — anything framework-specific lives in templates as declared commands.

## Refactor Decisions

1. `angee.yaml` is the source of truth. There is no `operator.yaml`, no separate state dir.
2. `service.Platform` is the only business logic layer.
3. `up/down/start/stop/restart/logs` are literal shims over docker compose. No custom orchestration code in between.
4. `angee dev` compiles to `process-compose.yaml` and starts the process-compose binary; runtime control goes through its REST API. No custom local-process supervisor.
5. Secrets go through a `Backend` interface; v1 implements `.env` and `openbao` natively in Go. The interface is the contract, not the implementations.
6. Templates are Copier templates with `_angee` metadata, rendered in-process by the `copier-go` library (no Python runtime). `copier update` powers `stack update` and `agent update`.
7. The Agent provisioning primitive is the one feature Angee builds from scratch, because nothing else does worktree + workspace + MCP + secrets + addressable service in one command.
8. Workspace is not a CLI noun. It only exists as part of an Agent.
9. One operator serves one `ANGEE_ROOT`. Run multiple operators for multiple environments.
10. No backward-compatibility branches in active code. Old paths get deleted.

## Success Shape

```sh
# Goal 1
angee init --dev --yes
angee dev                              # compose sidecars + process-compose locals, healthy and tailing
angee down

# Goal 1, staging
angee stack init staging-docker --yes --set domain=staging.example.com
angee up
angee logs web --follow

# Goal 2
angee agent init reviewer \
  --template agents/angee-developer \
  --source app=https://github.com/fyltr/app#feature-x \
  --start --yes
# → worktree at ../app.workspaces/feat-reviewer, MCPs wired, reachable at $OPERATOR_URL/agents/reviewer

curl -H "Authorization: Bearer $TOKEN" \
  -X POST $OPERATOR_URL/agents/reviewer/messages \
  -d '{"text": "open a PR with the schema migration"}'

angee agent logs reviewer --follow
angee agent destroy reviewer           # service removed; worktree kept unless --purge
```

If those flows are boring, v1 is done.
