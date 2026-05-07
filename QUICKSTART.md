# Angee Quickstart

Status: target shape per [`refactor/OVERVIEW-v2.md`](./refactor/OVERVIEW-v2.md).

**Angee is the stack manager for angee.ai.** It compiles one
`angee.yaml` into a running environment and exposes a stable HTTP+MCP
control surface so the runtime above it can mutate the stack
programmatically. Two consumers drive Angee:

- **Humans, via the CLI** — what this quickstart walks through.
- **Angee runtimes (e.g. `django-angee`), via the operator HTTP+MCP
  API** — the runtime runs inside a stack Angee manages and calls the
  operator to self-manage that stack and self-update its sources. The
  HTTP examples below double as a sketch of what those calls look like.

Two flows below. One spins up a local dev stack from a template. The
other provisions a workspace on a worktree so an agent (or a human) can
build a PR against an isolated branch — done from the CLI here, but
identical when issued by a runtime over HTTP.

## Install

```sh
brew install angee
# or:
curl https://angee.ai/install.sh | sh
```

Requirements on the host: `docker`, `git`, and (for `angee dev`)
`process-compose` on `$PATH`.

## 1. Spin up a local dev stack

Clone a project that has `.templates/stacks/dev/` checked in:

```sh
git clone https://github.com/example/your-project.git
cd your-project
angee init --dev          # renders .angee/angee.yaml from .templates/stacks/dev
angee dev                 # operator + container sidecars + local processes, all foreground
```

`angee init --dev` is shorthand for `angee stack init dev` and resolves
the template from `.templates/stacks/dev/` in the project repo. (If the
project doesn't have one yet, the angee binary ships a starter `dev`
stack as a fallback.)

`angee dev` brings up three things and tails them in one terminal:

- The operator HTTP+MCP server on `127.0.0.1:9000`
- Container sidecars (postgres, redis, openbao, …) via `docker compose`
- Local processes (Django, Vite, build watchers, …) via `process-compose`

Every supervised process gets `ANGEE_OPERATOR_URL` and
`ANGEE_OPERATOR_TOKEN` injected, so app code can call the operator API
out of the box without per-environment configuration.

Ctrl+C tears it all down. Container sidecars (databases, etc.) survive
across `dev` runs unless the template says otherwise, so warm caches and
committed data persist.

In another terminal, useful pokes:

```sh
angee status                          # what's running
angee logs web --follow               # tail one service
angee stop  web && angee start web    # bounce a single process
angee down                            # stop everything
```

## 2. Provision a workspace to build a PR

Goal: an agent (or you) builds out `feat-ui-animation` against a fresh
worktree, with the chained dev stack alive inside it on its own ports.

In a second terminal, while `angee dev` is running:

```sh
angee workspace create pr --input branch=feat-ui-animation --start
```

This:

1. Resolves the template at `.templates/workspaces/pr/`.
2. Allocates per-workspace ports from the operator pool (so the inner
   stack doesn't collide with the outer one).
3. Adds a `git worktree` on `feat-ui-animation` off the project source's
   shared cache — fast, because the local clone is already there from
   step 1.
4. Renders the workspace into `.angee/workspaces/feat-ui-animation/`.
   The workspace name auto-derives from `instance_naming.pattern`
   declared in the template (typically `${inputs.branch | slug}`).
5. Resolves and writes secrets, ports, and addresses into the workspace's
   gitignored `.env`.
6. If the `pr` template chains a stack, provisions and starts the inner
   `.angee/` (a per-worktree dev stack on per-workspace ports).

To run an agent in that workspace, declare a service that mounts it:

```sh
angee service init agent-pr-feat-ui-animation \
  --image ghcr.io/example/claude-runner:latest \
  --mount workspace://feat-ui-animation:/workspace \
  --env ANTHROPIC_API_KEY='${secret.anthropic-api-key}' \
  --start
```

Or, equivalently, an application running under `angee dev` calls the
operator over HTTP with a `POST /workspaces` followed by `POST /services`.

The agent commits inside the worktree as it works. To check status and
ship the branch:

```sh
angee workspace get  feat-ui-animation     # path, mounted-by, ports
angee source  status django-angee          # branch / dirty state of every worktree off the source
angee workspace push feat-ui-animation     # push every worktree-mode source in the workspace
```

PR creation itself isn't an Angee operation — the agent (or you) runs
`gh pr create` inside the worktree using credentials wired through env.
Angee's job ends at "the worktree is on a named branch with an upstream
and `git push` works".

## Tear down

Service and workspace lifecycles are independent — Angee won't auto-stop
services when their workspace is destroyed. Compose teardown explicitly:

```sh
angee service   destroy agent-pr-feat-ui-animation
angee workspace destroy feat-ui-animation --purge   # --purge removes the worktree
angee down                                           # stop the dev stack
```

`workspace destroy` refuses if a running service still mounts the
workspace; the error names the offending services. `--force` overrides
for the "I really mean it" case.

## Self-managing runtimes (the operator API path)

Everything above can also be issued by a runtime *inside* the stack
over HTTP — same operations, same `service.Platform`, no human at a
terminal. The runtime is born with `ANGEE_OPERATOR_URL` and
`ANGEE_OPERATOR_TOKEN` already in its env, so the calls are one-liners.

`django-angee` self-managing its stack typically looks like:

```http
POST  ${ANGEE_OPERATOR_URL}/workspaces           # provision a session workspace
POST  ${ANGEE_OPERATOR_URL}/sources/django-angee/pull   # self-update code
POST  ${ANGEE_OPERATOR_URL}/services             # spin up a sidecar / agent runner
PATCH ${ANGEE_OPERATOR_URL}/workspaces/{n}       # extend a TTL
GET   ${ANGEE_OPERATOR_URL}/events  (SSE)        # react to lifecycle changes
```

The same operations are exposed as MCP tools at `${ANGEE_OPERATOR_URL}/mcp`,
so a model client running inside a service can drive the stack without
any HTTP-client glue. Full surface in
[`refactor/OVERVIEW-v2.md`](./refactor/OVERVIEW-v2.md).

## Where templates live

Angee resolves template names from these paths, in order:

1. `.templates/<kind>/<name>/` in the project repo (always wins if
   present). `<kind>` is `stacks` or `workspaces`.
2. Embedded in the angee binary — a small set of starter stacks plus a
   reference workspace template, so a fresh repo can run `angee init
   --dev` before it has its own `.templates/`.
3. (Phase 8) remote refs like `github.com/owner/repo//path@ref`.

So the two flows above resolve as:

- `angee init --dev`             → `.templates/stacks/dev/`
- `angee workspace create pr`    → `.templates/workspaces/pr/`

Templates are Copier templates with optional `_angee` metadata in
`copier.yml`. The metadata declares typed inputs, instance-naming
patterns, and (for workspaces) source mounts and optional inner stacks.

## Common commands

```sh
angee init --dev                                            # alias: stack init dev
angee dev   [--build]                                       # foreground: operator + compose + process-compose
angee up    [--build]                                       # detached: compose only (no host processes)
angee build [service...] [--no-cache]                       # docker compose build pass-through
angee status                                                # what's running across both backends
angee logs <service> [--follow]
angee start|stop|restart <service>...
angee service init|destroy <name>
angee workspace create <template> [--input k=v ...] [--start]
angee workspace update|destroy|push|get <name>
angee source list|fetch|pull|push|status <name>
angee stack update                                          # rerun copier; preserve secrets + ports
```

Full reference: [`docs/USAGE.md`](./docs/USAGE.md). Architecture and the
operator API surface: [`refactor/OVERVIEW-v2.md`](./refactor/OVERVIEW-v2.md).
