# angee

**Angee is the stack manager for [angee.ai](https://angee.ai).**

It compiles one declarative manifest (`angee.yaml`) into a running
environment — secrets resolved, services supervised, workspaces
provisioned — and exposes that environment over a stable HTTP+MCP
control surface so the runtime above it can mutate the stack
programmatically.

Think Docker Compose, plus host-process supervision, plus a workspace
provisioning primitive, plus an HTTP/MCP API the application layer
calls to self-manage its own stack. One Go binary, no Python runtime,
no Kubernetes required.

---

## Two consumers

Angee has exactly two callers, and the architecture flows from that:

### 1. Humans, via the CLI

`angee init`, `angee dev`, `angee up`, `angee workspace create`, etc.
Terminal-driven setup, dev loop, and day-to-day operations. What a
developer types to bring a stack up, debug it, and ship a branch.

### 2. Angee runtimes, via the operator HTTP+MCP API

Runtimes like [`django-angee`](https://github.com/fyltr/django-angee)
run *inside* a stack Angee manages and call the operator API to
**self-manage that stack and self-update their sources**. Spinning up a
session-scoped workspace, pulling fresh code into an existing worktree,
rotating a secret, scaling a worker — all happen as HTTP calls from the
runtime to its own operator, with no human in the loop.

```http
POST  ${ANGEE_OPERATOR_URL}/workspaces                   # provision a session workspace
POST  ${ANGEE_OPERATOR_URL}/sources/django-angee/pull    # self-update code
POST  ${ANGEE_OPERATOR_URL}/services                     # add a sidecar / agent runner
PATCH ${ANGEE_OPERATOR_URL}/workspaces/{name}            # extend a TTL
GET   ${ANGEE_OPERATOR_URL}/events  (SSE)                # react to lifecycle changes
```

The same operations are exposed as MCP tools at `/mcp`, so a model
client running inside a service can drive the stack the same way.

**Both paths share one business-logic layer (`service.Platform`).** The
CLI and the HTTP/MCP server are thin adapters over the same operations
— there is no "CLI-only" capability and no "API-only" capability.
That symmetry is what makes a runtime like `django-angee` capable of
self-managing the very stack it lives in.

---

## Quickstart

### Install

```sh
brew install angee
# or:
curl https://angee.ai/install.sh | sh
```

Host requirements: `docker`, `git`, and (for `angee dev`)
`process-compose` on `$PATH`.

### Spin up a local dev stack

```sh
git clone https://github.com/example/your-project.git
cd your-project
angee init --dev          # renders .angee/angee.yaml from .templates/stacks/dev
angee dev                 # operator + container sidecars + local processes, all foreground
```

`angee dev` brings up three things and tails them in one terminal:

- The operator HTTP+MCP server on `127.0.0.1:9000`
- Container sidecars (postgres, redis, openbao, …) via `docker compose`
- Local processes (Django, Vite, build watchers, …) via `process-compose`

Every supervised process gets `ANGEE_OPERATOR_URL` and
`ANGEE_OPERATOR_TOKEN` injected automatically, so runtime code can call
the operator out of the box without per-environment configuration.

### Provision a workspace

In a second terminal, while `angee dev` is running:

```sh
angee workspace create pr --input branch=feat/issue-123 --start
```

This:

1. Resolves the template at `.templates/workspaces/pr/`.
2. Allocates per-workspace ports from the operator pool.
3. Adds a `git worktree` on `feat/issue-123` off the project source's
   shared cache.
4. Renders the workspace into `.angee/workspaces/feat-issue-123/`.
5. Resolves and writes secrets/ports/addresses into the workspace's
   gitignored `.env`.
6. If the template chains a stack, provisions and starts the inner
   `.angee/` on per-workspace ports.

The exact same sequence is available to a runtime as
`POST /workspaces` over HTTP.

---

## Core concepts

Six concepts. No "agent" — agents are a runtime concern.

| Concept | Meaning |
|---|---|
| **Stack** | The environment for one `ANGEE_ROOT`: sources, workspaces, services, jobs, secrets, ports, volumes. |
| **Source** | A typed pool entry (`git`, `template`, `archive`, `url`, `local`, `volume`). Declared once and referenced by name. The compose-`volumes:` analogue. |
| **Workspace** | A materialized directory under `$ANGEE_ROOT/workspaces/<name>/`. Rendered from a chain of Copier templates with sources mounted at subpaths. Mountable into one or more services. |
| **Service** | A runnable workload. `runtime: container` → docker compose. `runtime: local` → process-compose. |
| **Job** | A one-shot or scheduled command. Same mounts/env semantics as a service. |
| **Operator** | The reconciler. Renders workspaces, resolves secrets, compiles backend files, applies runtime changes, exposes HTTP + MCP. Always running alongside the runtime. |

---

## Command surface (humans)

```sh
# Stack
angee init --dev [path]                                     # alias: stack init dev
angee stack init <template> [path] [--input k=v ...]
angee stack update | destroy

# Runtime
angee build [service...] [--no-cache] [--pull]              # docker compose build pass-through
angee up    [service...] [--build]                          # detached: compose only
angee dev                [--build]                          # foreground: compose + process-compose + operator
angee down
angee start | stop | restart | logs <service>...
angee status                                                # services + workspaces + jobs

# Services
angee service init    <name> (--image img | --command ...) [--mount URI ...] [--env K=V ...] [--port spec...] [--start]
angee service update  <name> [--image img] [--command ...] [--env K=V ...] [--mount URI ...]
angee service destroy <name>
angee service start | stop | restart | logs <name>
angee service list

# Jobs
angee job run <name> [--input k=v ...]
angee job list | logs <name>

# Workspaces
angee workspace create  <template> [--input k=v ...] [--name <n>] [--ttl <dur>] [--start]
angee workspace update  <name> [--input k=v ...] [--ttl <dur>] [--sync-sources]
angee workspace destroy <name> [--purge|--force]
angee workspace start | stop | restart | logs <name>
angee workspace list | get <name>
angee workspace push <name> [--ref <r>]                     # push every worktree-mode source
angee workspace git  <name>                                 # status of every worktree-mode source

# Sources
angee source list
angee source fetch | pull | push | status <name> [--ref <r>]

# Operator (standalone daemon)
angee operator [--bind 0.0.0.0] [--port 9000]
```

---

## Operator API (runtimes)

Same operations, exposed as HTTP + MCP. One operator process serves
exactly one `ANGEE_ROOT`. Bearer auth required for non-loopback binds.

```
# Stack
POST   /stack/{init|update|build|up|dev|down|destroy}    GET /stack/status    GET /stack/logs

# Services
POST   /services                                         PATCH /services/{n}
POST   /services/{n}/{start|stop|restart|destroy}
GET    /services    GET /services/{n}    GET /services/{n}/logs

# Jobs
POST   /jobs/{n}/run    GET /jobs    GET /jobs/{n}/logs

# Workspaces
POST   /workspaces                                       PATCH /workspaces/{n}
POST   /workspaces/{n}/{update|start|stop|restart|destroy}
GET    /workspaces    GET /workspaces/{n}    GET /workspaces/{n}/logs
POST   /workspaces/{n}/push    GET /workspaces/{n}/git

# Sources
GET    /sources    GET /sources/{n}/status
POST   /sources/{n}/{fetch|pull|push}

# Async + events
GET    /operations/{id}    GET /events  (SSE)

# MCP — same operations as JSON-RPC tools
GET    /mcp
```

---

## Reuse stance

Angee is not a new orchestrator. It is a thin Go binary that composes
existing tools and adds one provisioning primitive (the Workspace).

| Layer | Reused tool |
|---|---|
| Container runtime | `docker compose` (shell out) |
| Local-process supervisor | `process-compose` (shell out + REST API) |
| Templating | `github.com/fyltr/copier-go` (in-process) |
| Worktrees | `git` CLI via `os/exec` |
| Secrets | Pluggable `Backend` interface; v1 ships `.env-file` and OpenBao natively |

Angee's novel pieces: the `angee.yaml` schema, the two-pass secret-safety
compiler (resolved values never land in committed files), the
substitution grammar with consumer-aware address rewriting, and the
Workspace primitive (template chain + multi-source materialization +
port allocation + optional inner stack + TTL/GC + mount URI scheme).

---

## Status

Target design and execution plan:

- [`refactor/OVERVIEW-v2.md`](../refactor/OVERVIEW-v2.md) — full target shape
- [`refactor/PLAN.md`](../refactor/PLAN.md) — phased execution sequence
- [`QUICKSTART.md`](../QUICKSTART.md) — step-by-step walkthrough

## Development

```sh
make build     # → dist/angee, dist/angee-operator
make test      # go test -v -race ./...
make check     # fmt + vet + lint + test
```
