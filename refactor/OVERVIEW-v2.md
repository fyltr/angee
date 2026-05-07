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
| Secrets | Pluggable backend interface; v1 implements `.env` and `openbao` natively, accepts `env:` imports | Angee owns `${secret.name}` resolution at compile time. The interface is shaped so a Teller-compatible adapter is feasible if needed. |
| Worktrees | `git worktree add` via stdlib `os/exec` | No wrapper library is worth a dependency. |
| MCP servers | Standard MCP servers (`@modelcontextprotocol/server-filesystem`, `@playwright/mcp`, etc.) | Angee declares them per agent; the agent runtime starts them. |

What stays Angee's:
- The `angee.yaml` manifest schema and the `angee.yaml → docker-compose.yaml` + `angee.yaml → process-compose.yaml` compilers.
- The Copier `_angee` metadata convention.
- `${secret.name}` resolution at compile time.
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
# Canonical shape: template positional, --input k=v repeatable, optional --name override.
# v1 has no per-template "pretty" flags — see "CLI = operator-as-library".
angee agent init <template> [--input key=value ...] [--name <explicit-name>] [--start] [--yes]
angee agent update <name>
angee agent destroy <name>
angee agent start|stop|restart|logs <name>
angee agent list

# Operator
angee operator [--bind 0.0.0.0] [--port 9000]
```

### `up` vs. `dev` — hard split

| Command | Container services | Local processes | Use |
|---|---|---|---|
| `angee up`  | `docker compose up -d` | **not started** | Production / staging / sidecars-only. |
| `angee dev` | `docker compose up -d` | `process-compose up` (foreground) | Local development with the full stack alive. |

`angee up` is **compose-only**. It never starts host processes. If your `angee.yaml` has only `runtime: container` services, `up` brings up the whole stack. If it has `runtime: local` services, those are simply not started by `up` — use `dev` for that.

`angee dev` is **compose + process-compose**. It runs the operator HTTP/MCP server on loopback for the lifetime of the session and starts both backends together (see "`angee dev`" section below).

`angee start/stop/restart/logs <name>` route by `runtime:` of the named service: container services → docker compose, local services → process-compose's REST API. `angee down` stops both backends.

The custom logic between user input and the underlying tool is only secret resolution, port substitution, and address rewriting — not orchestration.

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
  postgres-password: { generated: true, length: 32 }
  operator-token:    { generated: true, length: 32 }
  anthropic-api-key:                              # imported from host env on first run
    required: true
    import: env:ANTHROPIC_API_KEY

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
      POSTGRES_PASSWORD: "${secret.postgres-password}"

jobs:
  migrate:
    runtime: local
    source: app
    command: ["uv", "run", "python", "manage.py", "migrate", "--noinput"]

agents:
  default_template: agents/angee-developer
```

## Template Chaining

Template chaining is the answer to "an agent needs its own dev stack inside its worktree." There is no nested operator process and no nested operator state. The reconciliation invariant **one operator process serves one ANGEE_ROOT at runtime** is preserved.

What chaining is, exactly:

- A **second, sibling ANGEE_ROOT** is provisioned at a different path (e.g., inside the agent's worktree at `<worktree>/<app>/.angee/`).
- The outer operator performs that provisioning by **calling its own `service.Platform.StackInit()` as a library**, with `ANGEE_ROOT` set to the chained path. This is one provisioning call into a different root, not a recursive operator process.
- Once provisioned, the chained ANGEE_ROOT is a **normal, standalone angee project**. A developer can `cd` into the worktree and run `angee dev` from there if they want — that invocation runs its own in-process operator scoped to the chained root, the same way any `angee dev` in any directory does. There is no "parent" relationship at runtime; the runtime sees an independent ANGEE_ROOT.
- During `angee dev` from the **outer** root, the outer operator drives the chained root's runtime via lifecycle triggers (it again calls `service.Platform.StackUp(<chained-root>)` etc. as library calls). The outer operator process is the only operator process; it just calls into multiple roots.

```yaml
# in agents/claude-angee-developer/template/agent.yaml.jinja
chain:
  - kind: stack
    template: stacks/angee-notes-dev
    workdir: ${workspace.code_path}/${inputs.app_subpath}
    root:    ${workspace.code_path}/${inputs.app_subpath}/.angee   # ← second ANGEE_ROOT
    inputs:
      project_name: angee-notes-${name}
      ANGEE_ROOT:   .angee
      app_path:     .
      django_port:  ${alloc.django}
      react_port:   ${alloc.react}
    lifecycle:
      provision_on: agent.init        # → service.Platform.StackInit(root, template, inputs)
      up_on:        agent.start       # → service.Platform.StackUp(root)
      down_on:      agent.stop        # → service.Platform.StackDown(root)
      destroy_on:   agent.destroy     # → service.Platform.StackDestroy(root)
```

This preserves the invariant: one operator process owns runtime reconciliation for one ANGEE_ROOT at a time. Multiple ANGEE_ROOTs (the outer and the chained) can be **provisioned and driven** by the same operator process via library calls. Two simultaneous **runtime** operators on the same root would still be illegal — and chaining never creates that situation, because the chained root only has runtime through library calls from its outer.

Worktree + chained stack = an agent that owns its own dev environment without needing a second operator process.

## Goal 1: Environments with Secrets + Services

The flow, end to end:

```sh
angee init --dev --yes        # copier render → angee.yaml + .copier-answers.yml
angee dev                     # local dev: compose sidecars + process-compose locals + operator
# ── or, for prod/staging where every service is runtime: container ──
angee up                      # docker compose up -d (containers only; no process-compose)
angee logs web --follow
angee down
```

Compilation pipeline on every `up` / `dev` / `start`:

1. Load `angee.yaml`.
2. Resolve secrets through the configured backend (`.env` or OpenBao). Generate any `generated: true` secrets that don't exist yet. Import `env:VAR` references from the host environment and persist into the backend.
3. Substitute `${secret.name}` and `${ports.name}` references.
4. Split services by `runtime`:
   - `runtime: container` → emit `docker-compose.yaml`.
   - `runtime: local` → emit `process-compose.yaml`.
5. For `angee up`: **compose-only.** Run `docker compose up -d` for `runtime: container` services. Local services are not started — `up` is for production/staging or sidecar-only scenarios. Use `angee dev` to start local services.
6. For `angee dev`: **compose + process-compose.** Run `docker compose up -d` for sidecars, then `process-compose up` for local services, with cross-boundary `depends_on` resolved by readiness probes (process-compose can wait on HTTP/exec health of compose services, and vice versa via health endpoints).
7. For `angee down/stop/restart/logs <name>`: route by the service's `runtime:` — container services hit docker compose, local services hit process-compose's REST API. `angee down` (no service arg) stops both backends.

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

### Bootstrap: host env vs. backend

The host process environment is the **bootstrap input**, not a backend. Three rules:

1. **At first `up`/`dev`/`init`**, any secret declaration with `import: env:VAR_NAME` reads `VAR_NAME` from the host environment and persists it into the configured backend (`.env` or OpenBao). This is how `ANTHROPIC_API_KEY=...` in your shell becomes a stored secret.
2. **At every subsequent `up`/`dev`**, the backend is the source of truth. Host env is only consulted to pick up newly added imports.
3. **`generated: true`** secrets are created in the backend on first run and persist there forever (or until `--rotate`).

So: same shell command works whether you're starting fresh (host env imports) or resuming an existing project (backend reads). `angee dev` always runs with secrets resolved into the process environment of every supervised service. The user never has to think about which backend "won" — host env is the bootstrap, the chosen backend is the durable home.

### Hard rule: no resolved secrets in committed files

The compiler MUST NEVER write a resolved secret value into any file that the user might commit. Specifically:

- **`angee.yaml`** — only references (`${secret.x}`) and declarations are committed. Resolved values: never.
- **Generated `docker-compose.yaml` / `process-compose.yaml`** — keep `${secret.x}` references as-is. Compose itself supports `${VAR}` interpolation against a secrets-loaded env, so the resolved file passed to `docker compose` / `process-compose` references env vars, never literals.
- **Per-service `.env` files inside agent dirs** — these may contain resolved secrets and **must be gitignored**. The operator writes them with `0600` perms and refuses to write to a path inside a tracked directory unless that path is in `.gitignore`.
- **`.copier-answers.yml`** — committed. Must never contain a resolved secret. If a Copier answer is sourced from a secret, the answer file stores the reference, not the value.
- **Template-rendered files at `agents/<name>/CLAUDE.md`, `.mcp.json`, etc.** — these may be committed. They MUST contain `${secret.x}` references, not resolved values. Resolution happens at process-launch time via env-var interpolation by the consuming runtime (compose, process-compose, the agent process reading its own `.mcp.json`).

The operator's compile pipeline performs **two passes**:

1. **Render pass** (Copier): resolves only static answers. Preserves `${ns.path}` references verbatim.
2. **Resolve-into-env pass** (operator): writes resolved values **only** into gitignored `.env` files and into the in-memory environment passed to compose/process-compose at launch. Never modifies committed files.

Generated compose/process-compose files commit-or-not is a template choice, but in either case they contain `${ENV_VAR}` references that are resolved by docker compose / process-compose at launch from the gitignored env-file.

## Goal 2: Agent Provisioning

This is the only place Angee builds something the ecosystem doesn't already have. One command does everything an AI agent needs to start coding against a repo.

```sh
# Canonical CLI shape. Template is positional. --input k=v provides
# template-declared inputs (see _angee.inputs). --name overrides the
# auto-derived instance name from instance_naming.pattern.
angee agent init agents/claude-angee-developer \
  --input branch=feat/issue-123 \
  --input base_branch=dev \
  --start --yes
# → name auto-derived: "feat-issue-123" (slug of --input branch)

# Override the derived name explicitly:
angee agent init agents/claude-angee-developer \
  --input branch=feat/issue-123 \
  --name reviewer \
  --start --yes
```

For `angee stack init`, same shape: `angee stack init <template> [path] [--input k=v ...]`.

What `agent init` does, in this order:

1. **Resolve template** (`agents/claude-angee-developer`) and read its `_angee` block (inputs, instance_naming, persist:, chain:, mcp_servers).
2. **Compute the instance name** from `instance_naming.pattern` against `--input` values (e.g., `branch=feat/issue-123` → `feat-issue-123`). `--name` overrides.
3. **Allocate ports** from the operator's pool for every `${alloc.<pool>}` reference (Django, React, Playwright, agent-service ports).
4. **Resolve & generate secrets** in the configured backend (`.env`/OpenBao). `generated: true` secrets are minted now (e.g., `agent-operator-token`); `import: env:VAR` imports happen here too.
5. **Materialize workspace sources** into `$ANGEE_ROOT/agents/<name>/workspace/`:
   - `kind: git, mode: worktree` (the developer-agent case) → `git worktree add <worktree_path> -b <branch> <base_branch>` off the global source's `cache_path` (shared local clone). Multiple agents share one fetch.
   - `kind: git, mode: clone` → shallow clone with locked commit.
   - `kind: template` → `copier copy` of a workspace template into a subpath.
   - `kind: url|archive` → download + checksum verify + extract.
   - `kind: local` → bind or copy.
   - `kind: volume` → mount a declared volume at a subpath; `scope: agent` derives a per-agent volume.
6. **Render agent files** with copier-go: `agents/<name>/agent.yaml`, `.env`, `CLAUDE.md`, `.mcp.json`, helper scripts. All `${secret.*}`, `${ports.*}`, `${persist.*}`, `${service.*}`, `${operator.*}` references are resolved at this point. Address-resolution rewrites the host portion per consumer.
7. **Create persistent dirs** declared in the template's `persist:` blocks (e.g., Playwright `browser-data`).
8. **Start the per-agent Playwright MCP** as a local process on the allocated port. Wait for its health endpoint to come green. Register it with the operator's service registry so `${service.playwright}` resolves correctly for downstream consumers (the chained stack's tests, the agent process's `.mcp.json`).
9. **Run the chained stack `init`** (`service.Platform.StackInit(<chained-root>, "stacks/angee-notes-dev", inputs)`): copier-render the chained `angee.yaml` into `<worktree>/<app>/.angee/` with the per-agent allocated Django/React ports baked in.
10. **Run the chained stack's `dev`** (`service.Platform.StackDev(<chained-root>)`) when `--start` is set: bring up postgres/redis sidecars via docker compose, start Django/Vite/build-watch via process-compose. Wait for `web` and `ui` to become healthy.
11. **Register and start the bundled agent service** in the outer `angee.yaml` under `services.<name>`. The service runs the agent process (`claude`, `codex`, ...) declared by the template. It mounts the workspace and inherits `.env` (operator URL, operator token, model API key, addresses of the chained Django/React/Playwright). `runtime: local` agents go through process-compose; `runtime: container` agents go through docker compose.
12. **Compute the agent's address** from operator config (`${operator.url}/agents/<name>`) and write it into the agent's own `.env` so the agent process can introspect its identity.

`--start=false` skips steps 8, 10, and the start half of 11 (everything is provisioned but nothing runs). `agent start` later runs them in the same order.

`agent destroy` runs the inverse: stop bundled service → chained stack `down` → stop Playwright MCP → optionally remove worktree (kept by default; `--purge` removes it and any persistent dirs).

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

### Addressability (preview)

Every agent has a stable name and a stable URL (`${operator.url}/agents/<name>`). The application stores only the name. What the application can do with that URL depends on the agent's declared capabilities (see "Minimum agent service contract" below):

- **Always** — `GET /agents/<name>` for status, `GET /agents/<name>/logs` for logs, `POST /agents/<name>/start|stop|restart`.
- **When the agent template declares `service.http_port`** — `POST /agents/<name>/messages` and any other paths the bundled HTTP service exposes are proxied through the operator.
- **When the agent template declares `service.mcp_port`** — the agent's own MCP server is reachable at `${operator.url}/agents/<name>/mcp`.

For the developer-agent (a `claude` CLI process), only the "always" set is available — the agent has no HTTP and no MCP server of its own. For the personal-assistant (a containerized HTTP service), all three sets are available.

### Lifecycle

| Command | Implementation |
|---|---|
| `agent init` | Composite provisioning above. |
| `agent update` | `copier update` on agent template, re-materialize sources whose lock/ref changed, rewrite agent.yaml + service registration, optionally restart. |
| `agent start/stop/restart` | Same as `service start/stop/restart` on the bundled service. |
| `agent logs` | Same as `service logs`. |
| `agent destroy` | Stop service, remove service from manifest. Worktree and persistent sources kept by default; `--purge` removes them. |
| `agent list` | Join services tagged `agent` with their agent.yaml metadata. |

## Elegant shorthand: `_angee.inputs` and `instance_naming`

Both the CLI and the HTTP/MCP API speak the same input language because every template declares it once.

In `copier.yml`'s `_angee` block:

```yaml
_angee:
  kind: agent
  name: claude-angee-dev

  # How to derive the agent NAME from inputs when --name is not given.
  instance_naming:
    pattern: "${inputs.branch | slug}"
    fallback: "${inputs.name}"
    max_length: 40

  # Typed inputs. CLI flags and HTTP `inputs:` map through `maps_to`.
  inputs:
    branch:
      type: str
      required: true
      cli_flag: --branch
      maps_to:
        copier: branch                                    # → Copier answer
        angee:  agents.${name}.workspace.sources.code.branch
    base_branch:
      type: str
      default: dev
      cli_flag: --base-branch
      maps_to: { copier: base_branch }
```

Two equivalent invocations:

```sh
# CLI shorthand — the operator pulls the template, reads _angee.inputs,
# registers the declared flags on cobra, and computes the name via instance_naming.
angee agent init claude-angee-dev --branch refactor-angee-notes
```

```http
POST /agents
{ "template": "agents/claude-angee-dev",
  "inputs": { "branch": "refactor-angee-notes" } }
```

Both resolve to the same provisioning call: `name = "refactor-angee-notes"` (from the `instance_naming` pattern), `branch = refactor-angee-notes`, all other inputs at defaults. No hardcoding in the CLI; the template owns its surface.

For Django spinning up a personal assistant for one user:

```http
POST /agents
{ "template": "agents/personal-assistant",
  "inputs": {
    "user": "alex@example.com",
    "mcp_credentials": {
      "gmail":    { "url": "https://gmail-mcp.example.com" },
      "calendar": { "url": "https://calendar-mcp.example.com" }
    }
  } }
```

The `personal-assistant` template uses `pattern: "${inputs.user | local_part | slug}"` to derive `name = "alex"`. Per-user MCP credentials are passed as resolver references (`${connector.gmail.alex}`), not as bearer tokens — the operator asks a registered resolver (typically Django) for the actual token at agent start, and refreshes when expired.

### Substitution grammar

| Form | Meaning | Where |
|---|---|---|
| `${secret.name}` | Read from secrets backend | Anywhere |
| `${connector.provider.user}` | Resolve via registered connector resolver | Mostly per-user agents |
| `${service.name}` | Resolve to `host:port` of another service in the same stack | Anywhere |
| `${service.name.host}` / `${service.name.port}` | Just the host or just the port | Anywhere |
| `${ports.name}` | Read from this manifest's `ports` block | Anywhere |
| `${alloc.pool}` | Request a port allocation from the operator's pool at provisioning | Agent templates only |
| `${persist.<key>}` | Absolute path to a template-declared persistent dir (see `persist:` block) | Within the template that declared it |
| `${operator.url}`, `${workspace.root}`, `${name}`, `${inputs.<key>}` | Context refs | Anywhere |

Full grammar (delimiter, namespaces, filter pipeline, escape rules, anti-patterns) is specified in `examples-v2/SUBSTITUTIONS.md`. Single `${...}` delimiter; `.` as namespace separator (not `:`); `|` for filter pipelines (`${inputs.user | local_part | slug | truncate(40)}`). Borrows from Terraform's namespace dot-path, GitHub Actions' namespace taxonomy, and Ansible's filter pipelines. `{{ }}` is reserved for Copier's Jinja2 render pass and is never used in operator-resolved positions.

### Sources are a global pool

The top-level `sources:` block in `angee.yaml` is the source pool — exactly like `volumes:` in docker compose. Sources are declared once at the stack level and referenced by name from agents (and from anywhere else that needs content). Per-reference overrides (branch, ref, mode, worktree path) live at the reference site.

```yaml
# angee.yaml — declared once at the stack
sources:
  django-angee:
    kind: git
    repo: ../django-angee
    default_ref: dev
    cache_path: .angee/sources/django-angee   # shared local clone
  agent-skills:
    kind: git
    repo: https://github.com/fyltr/agent-skills
    default_ref: main
    cache_path: .angee/sources/agent-skills
```

```yaml
# An agent references the global source by name and adds per-instance overrides.
agents:
  reviewer:
    workspace:
      sources:
        code:
          subpath: code
          source: django-angee     # ← references the global pool by name
          mode: worktree
          ref: dev
          branch: feat/issue-123
          worktree_path: ../django-angee.workspaces/${name}
        skills:
          subpath: skills
          source: agent-skills     # same source, different agent
          ref: main
          mode: clone
```

Two agents pointing at the same global git source share the local clone (`cache_path`) and `git worktree add` distinct worktrees off it. This means one fetch, many isolated checkouts — fast to provision and cheap on disk.

The reference shape mirrors compose volumes: `source: <name>` is the equivalent of compose's `volumes: [name:/target]`. Per-instance options on the reference line override the source's defaults; the source declaration is reused, not copied.

### Address resolution: the operator picks the host portion

Addresses (`${operator.url}`, `${service.<name>}`, `${service.<name>.host}`) resolve to **different host strings depending on where they're consumed**. The operator does this rewrite at render time, not the template author.

The rule:

| Source runs as | Destination consumes from | Host portion |
|---|---|---|
| host process (process-compose, host-local MCP) | host process | `127.0.0.1` |
| host process | container in operator network | `host.docker.internal` |
| container | host process | `127.0.0.1` (compose publishes port) |
| container | container, same docker network | compose service DNS (`postgres`) |
| container | container, different network | `host.docker.internal` |

So the same `${operator.url}` that resolves to `http://127.0.0.1:9000` in a host-process agent's `.env` becomes `http://host.docker.internal:9000` in a containerized agent's env, and the same `${service.postgres}` resolves to `postgres:5432` inside a compose-network sidecar but `127.0.0.1:5432` for a host-process Django server. Templates write the reference once; the operator writes the right host into each rendered destination.

The operator knows where each service runs (host process vs. container) from the service's `runtime:` field. It knows where each *consumer* runs the same way. With both sides known, the rewrite is deterministic.

### `mcp_servers` tolerance

Templates may list MCP servers under a service's `mcp_servers: [...]` that aren't declared in the same manifest (e.g., a conditionally-rendered template that ends up listing a server it didn't wire). The operator skips unknown entries with a warning rather than failing. This keeps conditional-render logic in templates simple — the listing and the declaration don't have to be perfectly aligned.

### Persistent paths declared by templates, not hardcoded

Anything an MCP server (or any service) needs to keep across restarts — Playwright user-data-dir, browser caches, model-weights downloads, etc. — is declared by the template under a `persist:` block on the service or MCP entry:

```yaml
mcp_servers:
  playwright:
    runtime:
      kind: local-process
      command: ["npx", "-y", "@playwright/mcp@latest", "--user-data-dir", "${persist.browser-data}"]
    persist:
      browser-data: { subpath: .playwright-data, scope: agent }
```

The operator:

- Creates the directory on first start under `${workspace.root}/<subpath>` (or under a stack-scoped path if `scope: stack`).
- Exposes the absolute path as `${persist.<key>}` for use anywhere in the same template.
- Preserves it across `agent stop`/`agent start` and `agent update`.
- Removes it on `agent destroy --purge` (or stack `--purge`).

Nothing about Playwright (or any MCP server) is hardcoded in the operator. Templates own the names, paths, and scopes; the operator owns the lifecycle.

### CLI = operator-as-library

There is no separate CLI flag-marshaling code path. The CLI binary always goes through `service.Platform`, either by:

- starting an in-process operator (when no daemon is running), or
- calling a daemon over HTTP (when `--operator` / `ANGEE_OPERATOR_URL` is set, or a daemon is detected on the local socket).

In both cases the CLI's job is identical: parse flags into a `service.Platform.Provision()` request, hand it off, and render the response. Template-aware behavior (reading `_angee.inputs`, computing names, allocating ports) lives entirely inside `service.Platform` — the CLI has no template knowledge of its own.

The CLI shape for v1 is fixed and template-blind:

```sh
angee agent init <template> [--input key=value ...] [--name explicit-name]
```

`--input key=value` is repeatable. There are no per-template "pretty" flags in v1 — the CLI does not introspect templates to register typed flags. This keeps the CLI deterministic and removes the chicken-and-egg of needing to resolve a template before parsing flags. If/when pretty flags are revisited, they are an opt-in addition that doesn't change the canonical shape.

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
POST   /agents/{n}/messages            (optional — only when service.http_port set; 404 otherwise)
*      /agents/{n}/mcp/*               (optional — only when service.mcp_port  set; 404 otherwise)
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
  │  (only when service.http_port set —   │                                │
  │   not for the dev-agent CLI shape)    │                                │
  ├──────────────────────────────────────►│ proxy to bundled HTTP service  │
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
    "django":     "auto",
    "react":      "auto",
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
  "ports": { "django": 8247, "react": 5374, "playwright": 9312 },
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

### Minimum agent service contract

Bundled agent services come in two shapes. The operator only requires the **minimum contract**; everything else is optional.

| Capability | Required | Provided by |
|---|---|---|
| **Process lifecycle** (start/stop/restart, exit code, log stream) | **Yes** | The runtime (compose / process-compose). Always available for any registered service. |
| **Logs** (`angee agent logs <name>`) | **Yes** | Same — read from compose/process-compose. |
| **Health endpoint** | Optional | If the template declares `health.http: <path>` the operator gates `start` on it. |
| **HTTP API** (incl. `/messages`, custom tool calls) | **Optional** | Only if the agent template declares `service.http_port` and a corresponding `runtime: container/local` service that listens. |
| **MCP server** (the agent itself exposes one) | **Optional** | Only if the agent template declares `service.mcp_port`. |

The **developer-agent** in `examples-v2/` is a `claude` CLI process — no HTTP, no MCP-as-server. The operator's promise for it is: "the process is running with the worktree mounted, the operator URL/token in env, and the supervised lifecycle endpoints work." Talking to the agent is a TUI/keyboard-driven affair (the developer attaches to the process or its tmux/zellij session) — not an HTTP message bus.

The **personal-assistant agent** is a `runtime: container` HTTP service (e.g., `agent-claude` on port 3007). Its template declares `service.http_port: 3007`, so the operator exposes:
- `GET   /agents/<name>` (status — always available)
- `GET   /agents/<name>/logs` (logs — always available)
- `POST  /agents/<name>/messages` (proxied to the bundled HTTP service — **only when `service.http_port` is set**)
- `<operator>/agents/<name>/mcp` (proxied MCP — only when `service.mcp_port` is set)

For agents without an HTTP service, `POST /agents/<name>/messages` returns `404 Not Found` with a body explaining the agent doesn't expose a message bus. Status, logs, lifecycle calls still work.

### Addressability

After provisioning, `GET /agents/<name>` returns the same shape regardless of agent type, with optional fields for the optional capabilities:

```json
{
  "name": "feat-issue-123",
  "status": "healthy",
  "template": "agents/claude-angee-developer",
  "service": "feat-issue-123",
  "runtime": "local",                                      // local | container
  "url":      "http://operator.example.com:9000",          // operator URL (always present)
  "ports":    { "django": 8247, "react": 5374, "playwright": 9312 },
  "worktree": {
    "path":   "/srv/angee/worktrees/feat-issue-123",
    "branch": "feat/issue-123",
    "base":   "dev",
    "commit": "ab12cd34..."
  },
  "http":     null,                                        // null when service has no http_port
  "mcp":      null,                                        // null when service has no mcp_port
  "created_at":      "...",
  "ttl_expires_at":  "..."
}
```

For the `personal-assistant` shape:

```json
{
  "name": "alex",
  "runtime": "container",
  "http": { "url": "http://operator.example.com:10204",
            "proxy": "https://operator.example.com/agents/alex" },
  "mcp":  { "url": "http://operator.example.com:10204/mcp",
            "proxy": "https://operator.example.com/agents/alex/mcp" }
}
```

When `http` is non-null, two paths exist on purpose:
- **Direct URL** — fast, agent's own HTTP. Use when the app and operator share a network.
- **Operator proxy** — same TLS + auth as the operator. Use over the public internet.

Django stores only the agent name and the operator URL. Everything else is looked up.

### MCP wiring across the boundary

The agent's `.mcp.json` is **a Copier-rendered file**, not an output of a separate auth subsystem. It contains exactly the substitutions the agent needs:

```jinja
{
  "mcpServers": {
    "angee-operator": {
      "type": "http",
      "url": "${operator.url}/mcp",
      "headers": { "Authorization": "Bearer ${secret.agent-operator-token}" }
    },
    "playwright": {
      "type": "http",
      "url": "http://127.0.0.1:${ports.playwright}/mcp"
    }
  }
}
```

The per-agent operator token is just a `generated: true` secret declared in the agent's `agent.yaml`:

```yaml
secrets:
  agent-operator-token: { generated: true, length: 32, scope: agent }
```

The operator generates it on `agent init`, persists it in the backend, substitutes it into `.mcp.json` and into the agent's `.env`, and rotates it on `agent update`. There is no scope-grammar, no permission DSL — the operator just checks that an incoming bearer matches a known agent-scoped token and rejects access to anything outside that agent's namespace. Anything more elaborate is deferred until a real need surfaces.

### PR flow

The agent uses `git` directly. The container/process inherits the operator host's git config and credentials (SSH agent forwarding for local dev; a `GITHUB_TOKEN` or deploy key supplied as a secret in production). The agent runs `git commit`, `git push`, and `gh pr create` like any developer.

Angee's responsibility ends at "the worktree exists, git is configured, the branch is reachable." There is no Angee API for PR creation — that would just be a wrapper around the same git CLI the agent can already call.

### Multi-tenancy and isolation

One operator per trust boundary. For Django shipping multiple environments:

- **Per-environment operator** — staging gets its own operator + ANGEE_ROOT + port pool; prod gets another.
- **Per-tenant operator** — if Django serves multiple end-customers and each customer's agents must not see each other's worktrees or volumes, run one operator per tenant. The operator binary is small; spin up multiple.

The operator is not designed to multiplex tenants inside one ANGEE_ROOT. Pushing tenancy into one process means re-deriving auth/quota/isolation primitives that Linux containers + separate operator processes already give us.

### TTLs and cleanup

Every agent created by an external runtime should have a TTL. The operator runs a periodic sweep (or honors `ttl_expires_at` on each agent record) and calls `agent destroy` when expired. Django can extend a TTL with `POST /agents/{n}` and a new `ttl` value.

Without TTLs, ephemeral PR-review agents accumulate worktrees and ports forever. With TTLs, garbage collection is the operator's job, not Django's.

## `angee dev`

`angee dev` is the integrated dev loop. One command brings up three things and keeps them running together for the lifetime of the session:

1. **The operator HTTP + MCP server** on loopback (port from `angee.yaml`'s `operator.url`, bearer from the `operator-token` secret).
2. **Container sidecars** via `docker compose up -d` (postgres, redis, openbao, anything `runtime: container`).
3. **Local processes** via `process-compose up` (anything `runtime: local`: Django, Vite, build watchers, etc.).

```
       ┌─────────────────────────────────┐
       │       angee dev (foreground)    │
       └────────────┬────────────────────┘
                    │
        ┌───────────┼─────────────────────────────────┐
        ▼           ▼                                  ▼
   operator     docker compose up -d            process-compose up
   :9000        (sidecars)                      (local processes)
   /mcp                                          ▲
        ▲                                         │
        │      injected env: ANGEE_OPERATOR_URL,  │
        └──────  ANGEE_OPERATOR_TOKEN, ports ─────┘
```

Startup order:

1. Resolve secrets, allocate/confirm ports, compile `docker-compose.yaml` and `process-compose.yaml`.
2. Start the operator HTTP+MCP listener in-process. It binds to the configured loopback port and serves the same API surface as the standalone operator.
3. Start container sidecars via `docker compose up -d`.
4. Start `process-compose up` in daemon mode. Every local process gets `ANGEE_OPERATOR_URL`, `ANGEE_OPERATOR_TOKEN`, and resolved port env vars injected automatically — so a Django dev server running under `angee dev` can call the operator out of the box.
5. Tail logs from all three sources into a unified TUI (process-compose's TUI for local processes; compose log stream for containers; operator events on a status pane).

Shutdown on Ctrl+C is the reverse: stop process-compose (graceful per-process shutdown), stop the operator, leave container sidecars up unless template policy says otherwise (so postgres data and warm caches survive across `dev` runs).

### Provisioning agents during dev

Because the operator is live throughout the session, the developer (or any local process) can spin up agents into the running `.angee/` without leaving the dev loop:

```sh
# from another terminal, while `angee dev` is running:
angee agent init agents/claude-angee-developer \
  --input branch=feat/issue-123 \
  --start --yes
```

Or equivalently, a Django process running under `angee dev` calls the operator over HTTP:

```python
# Django code, env already populated by angee dev
import os, requests
requests.post(
    f"{os.environ['ANGEE_OPERATOR_URL']}/agents",
    headers={"Authorization": f"Bearer {os.environ['ANGEE_OPERATOR_TOKEN']}"},
    json={
        "template": "agents/claude-angee-developer",
        "inputs":   {"branch": "feat/issue-123"},
        "start":    True,
    },
)
```

Either path goes through the same `service.Platform`. The operator:

1. Renders the agent template via `copier-go`.
2. Allocates ports from the pool, materializes worktree + sources.
3. Registers the new service in the in-memory + on-disk `angee.yaml`.
4. **Hot-applies** the change: regenerates `docker-compose.yaml` / `process-compose.yaml`, then either `docker compose up -d <new-service>` or calls process-compose's REST API to add the new process. No `angee dev` restart required.
5. Streams provisioning events on the operator's `/events` SSE stream — the dev TUI can pick those up and show "agent reviewer provisioning…" → "healthy at http://127.0.0.1:8247".

### Why the operator runs during dev (not just on staging)

This is a deliberate design call. Two reasons:

- **Develop against the real API.** Django code that calls `POST /agents` in prod also calls it in dev. No mock, no env-specific branch.
- **Use agents in your own dev loop.** Spin up a `reviewer` agent inside `angee dev`, point it at the worktree of the feature you're working on, talk to it via MCP, then `angee agent destroy reviewer` when done. The agent shares the dev stack's postgres, redis, etc. — it's a peer of the services you're developing.

### Process-compose control

`process-compose` already provides depends_on with health gating, readiness probes, log fanout, TUI, REST API, env files, and cron-like scheduling. Angee adds nothing to its supervisor loop.

For runtime control during the dev session, Angee uses process-compose's REST API (or the `src/client` Go client) — `GET /processes` for status, `POST /process/start/{name}`, `POST /process/stop/{name}`, `GET /process/logs/{name}`. The `angee` CLI commands `start`/`stop`/`restart`/`logs` route to either docker compose or the process-compose REST endpoint depending on which backend owns the named service.

## ANGEE_ROOT Layout

```
$ANGEE_ROOT/
  angee.yaml                  # source of truth, committed.
                              # Contains ${secret.*} references only — never resolved values.
  .copier-answers.yml         # committed. Stores references, not resolved secrets.
  docker-compose.yaml         # generated. Committed or gitignored — template choice.
                              # Contains ${ENV_VAR} refs resolved by docker compose at launch.
  process-compose.yaml        # generated. Same rules as docker-compose.yaml.
  .env                        # ALWAYS gitignored. Contains resolved secrets at 0600.
  agents/<name>/              # rendered scaffold + materialized workspace.
    agent.yaml                # committed. References only.
    .env                      # gitignored. Resolved secrets at 0600.
    CLAUDE.md, .mcp.json      # committed. References only.
  volumes/                    # local persistent volumes when local-fs driver used
  run/                        # volatile operator working files, gitignored
```

**Hard rule (also stated above):** the compiler MUST refuse to write a resolved secret into any path inside a tracked directory unless that path appears in `.gitignore`. This is enforced at write-time, not by convention.

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
3. `angee up` is **compose-only** (containerized services). `angee dev` is **compose + process-compose** (containerized + host processes, plus the operator HTTP/MCP server in-process). `angee start/stop/restart/logs <name>` route by the service's `runtime:` field. No custom orchestration logic between user input and the underlying tools.
4. `angee dev` compiles to `process-compose.yaml` and starts the process-compose binary; runtime control goes through its REST API. No custom local-process supervisor.
5. Secrets go through a `Backend` interface; v1 implements `.env` and `openbao` natively in Go. The interface is the contract, not the implementations.
6. Templates are Copier templates with `_angee` metadata, rendered in-process by the `copier-go` library (no Python runtime). `copier update` powers `stack update` and `agent update`.
7. The Agent provisioning primitive is the one feature Angee builds from scratch, because nothing else does worktree + workspace + MCP + secrets + addressable service in one command.
8. Workspace is not a CLI noun. It only exists as part of an Agent.
9. **One operator process owns runtime reconciliation for one `ANGEE_ROOT` at a time.** A single operator process can still *call into* multiple roots as library calls (this is how template chaining works — the outer operator drives the chained root's lifecycle via `service.Platform.StackUp(<chained-root>)` etc.). Run **separate operator processes** for environments that should be runtime-isolated (different machines, different trust boundaries, different staging deployments).
10. No backward-compatibility branches in active code. Old paths get deleted.

## Success Shape

```sh
# Goal 1 — dev loop with everything running together
angee init --dev --yes
angee dev
# → operator HTTP+MCP on 127.0.0.1:9000
# → postgres, redis, openbao container sidecars
# → Django, Vite, build-watch as supervised local processes
# → ANGEE_OPERATOR_URL/TOKEN injected into every local process

# Goal 1, staging
angee stack init staging-docker --yes --set domain=staging.example.com
angee up
angee logs web --follow

# Goal 2 — provision an agent into the live dev session, no restart
# (in a second terminal, while `angee dev` is running)
angee agent init agents/claude-angee-developer \
  --input branch=feat/issue-123 \
  --start --yes
# → name auto-derived from instance_naming.pattern → "feat-issue-123"
# → worktree added off the global "django-angee" source
# → chained angee-notes-dev stack provisioned in the worktree on per-agent ports
# → MCPs wired (filesystem + operator + playwright + inner-django)
# → bundled service hot-added (process-compose for runtime: local agents,
#   docker compose for runtime: container agents)

# Or from a Django process running inside `angee dev`:
curl -H "Authorization: Bearer $ANGEE_OPERATOR_TOKEN" \
  -X POST $ANGEE_OPERATOR_URL/agents \
  -H "Content-Type: application/json" \
  -d '{
        "template": "agents/claude-angee-developer",
        "inputs":   { "branch": "feat/issue-123" },
        "start":    true
      }'

angee agent logs feat-issue-123 --follow
angee agent destroy feat-issue-123    # service hot-removed; worktree kept unless --purge
```

If those flows are boring — and the dev loop, the staging deploy, and the on-the-fly agent provisioning all use the same operator surface — v1 is done.
