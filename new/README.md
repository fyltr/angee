# Angee

**Angee is the stack manager for [angee.ai](https://angee.ai).**

It compiles one declarative `angee.yaml` manifest into a running environment: secrets resolved, services supervised, jobs runnable, sources fetched, and workspaces provisioned. It also exposes the same control plane over CLI, HTTP, and MCP so humans and runtimes can operate the stack through one shared business-logic layer.

Think Docker Compose plus host-process supervision, source/workspace provisioning, secret-safe compilation, and an operator API that lets an application runtime manage the stack it is running inside.

## Goals

Angee keeps two flows boring and reproducible:

1. **Run an environment with secrets and services.** One manifest, one command, get a healthy stack with container services, local processes, jobs, ports, volumes, and secrets wired correctly.
2. **Provision workspaces from templates and sources.** One operation renders a Copier template chain, materializes sources at declared subpaths, and registers the resulting directory as a mountable resource.

Everything above those flows belongs to the application runtime. Runtime-specific concerns such as model selection, message routing, connector credentials, PR creation, and app-specific MCP servers are expressed as normal services, jobs, inputs, secrets, and mounts.

## Consumers

Angee has two callers.

### Humans, Via The CLI

Developers use `angee init`, `angee dev`, `angee up`, `angee workspace create`, `angee service init`, and related commands for setup, local development, debugging, and day-to-day operations.

### Runtimes, Via The Operator API

An application runtime runs inside an Angee-managed stack and calls the operator HTTP+MCP API to manage that stack programmatically.

Examples:

```http
POST  /workspaces                     # create a workspace for a session or task
POST  /services                       # add a workload that mounts a workspace
POST  /sources/app/fetch              # refresh a source cache
POST  /sources/app/push               # publish committed worktree changes
PATCH /workspaces/feat-issue-123      # update inputs or extend TTL
GET   /events                         # observe lifecycle events over SSE
GET   /mcp                            # same operations exposed as MCP tools
```

The CLI and the HTTP/MCP server are thin adapters over `service.Platform`. If an operation exists for one consumer, it exists for the other.

## Architecture

```text
angee CLI (cobra)
    |
    v
service.Platform   <--- one business-logic layer
    |
    +--> docker compose                       container services
    +--> process-compose binary + REST API    local processes
    +--> copier-go                            template render + update
    +--> git CLI                              clones, worktrees, fetch, pull, push
    +--> secrets backend                      env-file, OpenBao
```

Default CLI execution uses an in-process `service.Platform`. Passing `--operator` or setting `ANGEE_OPERATOR_URL` switches the CLI to HTTP.

`angee dev` starts an operator listener on loopback for the lifetime of the dev session. Production stacks run `angee operator` as a standalone daemon alongside the runtime.

## Core Concepts

| Concept | Meaning |
|---|---|
| Stack | The environment for one `ANGEE_ROOT`: sources, workspaces, services, jobs, secrets, ports, and volumes. |
| Source | A typed pool entry: `git`, `template`, `archive`, `url`, `local`, or `volume`. Declared once and referenced by name. |
| Workspace | A materialized directory under `$ANGEE_ROOT/workspaces/<name>/`, rendered from templates with sources mounted at subpaths. |
| Service | A supervised workload. `runtime: container` runs through Docker Compose; `runtime: local` runs through process-compose. |
| Job | A one-shot or scheduled command with the same env, secret, mount, and workdir semantics as a service. |
| Operator | The reconciler that renders, resolves, compiles, applies runtime changes, and exposes HTTP+MCP. |

## Command Surface

```sh
# Stack
angee init --dev [path]
angee stack init <template> [path] [--input k=v ...]
angee stack update
angee stack destroy

# Runtime
angee build [service...] [--no-cache] [--pull]
angee up    [service...] [--build]
angee dev   [--build]
angee down
angee start   <service>...
angee stop    <service>...
angee restart <service>...
angee logs    [service...]
angee status

# Services
angee service init <name> [--runtime <container|local>] \
  (--image <img> [--command <cmd...>] | --command <cmd...>) \
  [--mount URI ...] [--env K=V ...] [--port spec ...] [--workdir URI] [--start]
angee service update  <name> [--image <img>] [--command <cmd...>] [--env K=V ...]
angee service destroy <name>
angee service start|stop|restart|logs <name>
angee service list

# Jobs
angee job run <name> [--input k=v ...]
angee job list
angee job logs <name>

# Workspaces
angee workspace create <template> [--input k=v ...] [--name <name>] [--ttl <dur>] [--start]
angee workspace update <name> [--input k=v ...] [--ttl <dur>] [--sync-sources]
angee workspace destroy <name> [--purge|--force]
angee workspace start|stop|restart <name>
angee workspace logs <name>
angee workspace list
angee workspace get <name>
angee workspace push <name> [--ref <r>]
angee workspace git  <name>

# Sources
angee source list
angee source fetch  <name>
angee source pull   <name> [--ref <r>]
angee source push   <name> [--ref <r>]
angee source status <name>

# Operator
angee operator [--bind 0.0.0.0] [--port 9000]
```

`angee up` is compose-only and starts container services in detached mode.

`angee dev` starts container services, local process services, and the in-process operator, then blocks in the foreground while tailing logs and handling shutdown.

## Operator API

One operator process serves one `ANGEE_ROOT`. Non-loopback binds require bearer authentication.

```text
# Stack
POST   /stack/init
POST   /stack/update
POST   /stack/build
POST   /stack/up
POST   /stack/dev
POST   /stack/down
POST   /stack/destroy
GET    /stack/status
GET    /stack/logs

# Services
POST   /services
PATCH  /services/{name}
POST   /services/{name}/start
POST   /services/{name}/stop
POST   /services/{name}/restart
POST   /services/{name}/destroy
GET    /services
GET    /services/{name}
GET    /services/{name}/logs

# Jobs
POST   /jobs/{name}/run
GET    /jobs
GET    /jobs/{name}/logs

# Workspaces
POST   /workspaces
PATCH  /workspaces/{name}
POST   /workspaces/{name}/update
POST   /workspaces/{name}/start
POST   /workspaces/{name}/stop
POST   /workspaces/{name}/restart
POST   /workspaces/{name}/destroy
POST   /workspaces/{name}/push
GET    /workspaces
GET    /workspaces/{name}
GET    /workspaces/{name}/logs
GET    /workspaces/{name}/git

# Sources
GET    /sources
GET    /sources/{name}/status
POST   /sources/{name}/fetch
POST   /sources/{name}/pull
POST   /sources/{name}/push

# Operations and events
GET    /operations/{id}
GET    /events

# MCP
GET    /mcp
```

Long-running operations return an operation id and emit status over SSE.

## Manifest

Every stack is described by `$ANGEE_ROOT/angee.yaml`. Relative paths resolve from `$ANGEE_ROOT`.

```yaml
version: 1
kind: stack
name: notes

operator:
  url: http://127.0.0.1:9000
  token_secret: operator-token
  port_pool:
    web: { range: "8200-8299" }
    custom: { range: "10000-10999" }

secrets_backend:
  type: env-file
  path: .env

secrets:
  postgres-password: { generated: true, length: 32 }
  operator-token: { generated: true, length: 32 }
  api-key: { required: true, import: env:APP_API_KEY }

ports:
  web: { value: 8100, export_env: WEB_PORT }

volumes:
  app-data: { driver: local-fs, path: data }

sources:
  app:
    kind: git
    repo: git@github.com:example/app.git
    default_ref: main
    cache_path: sources/app
    auth:
      mode: ssh
      ssh_key_secret: github-deploy-key

workspaces:
  feat-issue-123:
    template: workspaces/pr
    inputs:
      branch: feat/issue-123
      base_branch: main
    sources:
      code:
        source: app
        mode: worktree
        branch: feat/issue-123
        subpath: code
    resolved:
      chain:
        - workspaces/pr
        - stacks/dev
      chain_root: code/.angee
    ttl: 24h

services:
  postgres:
    runtime: container
    image: postgres:16
    env:
      POSTGRES_PASSWORD: "${secret.postgres-password}"

  web:
    runtime: local
    workdir: workspace://feat-issue-123/code
    command: ["uv", "run", "python", "manage.py", "runserver", "127.0.0.1:${ports.web}"]

  runner:
    runtime: container
    image: ghcr.io/example/runner:latest
    mounts:
      - workspace://feat-issue-123:/workspace
    env:
      APP_API_KEY: "${secret.api-key}"
      ANGEE_OPERATOR_URL: "${operator.url}"
      WORKSPACE_PATH: "${workspace.feat-issue-123.path}"

jobs:
  migrate:
    runtime: local
    workdir: workspace://feat-issue-123/code
    command: ["uv", "run", "python", "manage.py", "migrate", "--noinput"]
```

## Secret Safety

Resolved secret values never land in committed files.

Committed files keep references or environment placeholders. Gitignored `.env` files carry resolved values with `0600` permissions and are passed to Docker Compose and process-compose at runtime.

The compiler runs in two passes:

1. Render templates while preserving `${ns.path}` references.
2. Resolve references into runtime env files and in-memory process environments.

## Substitutions

Angee supports `${ns.path | filter}` expressions.

| Form | Resolver |
|---|---|
| `${secret.name}` | Secrets backend, emitted as env placeholders in generated files. |
| `${service.name}`, `${service.name.host}`, `${service.name.port}` | Service registry with consumer-aware address rewriting. |
| `${workspace.name.path}` | Absolute path of a workspace. |
| `${source.name.path}` | Absolute path of a source cache. |
| `${ports.name}` | Declared stack port. |
| `${alloc.pool}` | Operator-allocated port from a pool. |
| `${persist.key}` | Template-declared persistent directory. |
| `${operator.url}`, `${operator.domain}` | Operator address with consumer-aware host rewriting. |
| `${inputs.key}` | Provisioning-time input. |
| `${name}` | Resolved instance name. |

Filters: `slug`, `lower`, `upper`, `local_part`, `truncate(n)`, `default('x')`, `required('msg')`, `b64encode`, and `replace(a,b)`.

## Workspace Lifecycle

`workspace create` performs eight ordered steps:

1. Load the entry-point workspace template and resolve its template chain.
2. Compute the instance name from `--name` or template metadata.
3. Allocate ports for `${alloc.<pool>}` references.
4. Resolve or generate required secrets.
5. Materialize sources under `$ANGEE_ROOT/workspaces/<name>/`.
6. Render the Copier template chain.
7. Prepare an inner ANGEE_ROOT when the chain renders one.
8. Register the workspace in the outer `angee.yaml` and write workspace env data.

`workspace start` starts the inner stack when one exists. `workspace stop` stops it. `workspace destroy` reverses provisioning and can remove persisted directories with `--purge`.

Workspace TTLs are stored on workspace records and swept by the operator loop.

## Mounts

Services and jobs use a shared URI syntax for mounts and working directories.

| URI | Container Runtime | Local Runtime |
|---|---|---|
| `workspace://name:/path[:ro]` | Bind workspace root at `/path`. | Set `WORKSPACE_NAME_PATH`. |
| `workspace://name/sub:/path[:ro]` | Bind workspace subpath at `/path`. | Set `WORKSPACE_NAME_SUB_PATH`. |
| `source://name:/path[:ro]` | Bind source cache at `/path`. | Set `SOURCE_NAME_PATH`. |
| `volume://name:/path` | Mount named volume. | Set `VOLUME_NAME_PATH`. |
| `bind:///abs/host:/path` | Host bind. | Set `BIND_<SANITIZED>`. |

Local services use `workdir:` to select the actual host working directory.

## Source Operations

Sources are global stack resources. Git sources can be shared by many workspaces through one cached clone and multiple worktrees.

Supported operations:

- `source fetch <name>` refreshes the cache.
- `source status <name>` reports cache and worktree branch/dirty state.
- `source pull <name>` fast-forwards a cache or worktree branch.
- `source push <name>` pushes committed worktree changes after safety checks.
- `workspace git <name>` reports source status scoped to one workspace.
- `workspace push <name>` pushes all worktree-mode sources in a workspace.

Auth modes are `ssh`, `https-token`, and `host`. Host auth is accepted only for loopback-bound operators.

## Templates

Templates are Copier templates with `_angee` metadata in `copier.yml`.

Template families:

- `stacks/<name>` render an `angee.yaml` and supporting stack files.
- `workspaces/<name>` render a workspace directory and may chain a stack template into an inner ANGEE_ROOT.

Lookup order:

1. `.templates/<kind>/<name>/` in the project repo.
2. Embedded templates in the Angee binary.
3. Remote refs, when enabled.

CLI commands accept bare names when the command already implies the kind. HTTP requests and manifest records use kind-relative names such as `stacks/dev` and `workspaces/pr`.

## Package Layout

```text
api/                              request/response types shared by CLI, HTTP, and MCP
cmd/angee/                        CLI entrypoint
cmd/operator/                     standalone operator entrypoint
internal/cli/                     Cobra adapters over service.Platform
internal/copierx/                 copier-go wrapper
internal/fslock/                  per-root advisory locks
internal/git/                     git clone, fetch, worktree, pull, push helpers
internal/manifest/                angee.yaml types and validation
internal/operator/                HTTP, SSE, and MCP server
internal/ports/                   port pools and leases
internal/runtime/backend.go       runtime backend interface
internal/runtime/compose/         Docker Compose adapter
internal/runtime/proccompose/     process-compose adapter
internal/secrets/                 backend interface and implementations
internal/service/                 service.Platform business logic
internal/substitute/              substitution grammar and address rewriting
templates/                         embedded starter stack and workspace templates
```

## Implementation Sequence

1. **Skeleton:** manifest types, substitutions, env-file secrets, port pools, locks, `service.Platform`, and compile-only tests.
2. **Container runtime:** Docker Compose generation and `build`, `up`, `down`, `logs`, `status`, and container service operations.
3. **Local runtime:** process-compose generation, `angee dev`, local service operations, in-process operator listener, and env injection.
4. **Workspaces and source read path:** template chains, mount URIs, source caches, worktrees, workspace lifecycle, source fetch/status, and TTL records.
5. **Source write path:** pull, push, workspace git status, workspace push, and git safety checks.
6. **Standalone operator:** HTTP, SSE, MCP, bearer auth, operation tracking, and TTL sweep loop.
7. **OpenBao secrets:** native OpenBao backend over `net/http`.
8. **Polish:** port lease cleanup, purge semantics, lifecycle conveniences, and documentation.

## Development

```sh
make build     # build CLI and operator
make test      # run Go tests with race detector
make lint      # run golangci-lint
make check     # fmt + vet + lint + test
```
