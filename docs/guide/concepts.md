# Concepts

Angee is a **self-managed stack manager**. The `angee` CLI and the
`angee-operator` HTTP daemon — both written in Go — pull a set of source
repositories, render them into a working stack, and run that stack on
docker-compose or process-compose. The same primitives drive both
**development workspaces** and **production stacks**, so a feature branch
you develop in a workspace can be promoted to production by pointing the
same Sources at a different Stack.

## What "self-managed" means

Angee is the deployment plane *and* the development plane for the same
codebase, configured with the same `angee.yaml`. There is no separate
CI/CD system that knows how to build your app:

1. **GitOps over Sources** — your code is declared as Sources (git
   repositories or local paths) in `angee.yaml`. Angee fetches, caches,
   and (when needed) worktrees them.
2. **Workspaces compose Sources for development** — render a Copier
   template that materializes a chosen set of Sources on a feature
   branch, allocates ports, and brings up an isolated inner stack.
3. **Stacks compose Sources for deployment** — the same `angee.yaml`
   compiles to runtime files (Docker Compose or process-compose) and is
   driven by the operator.
4. **The operator promotes between environments** — the REST and GraphQL
   surfaces let CI, agents, or another tool drive the same lifecycle.

```text
        ┌────────────────┐
git ──► │    Sources     │ ─────────┐
        └────────────────┘          │
                ▼                   ▼
        ┌────────────────┐   ┌────────────────┐
        │   Workspaces   │   │     Stack      │
        │  (dev / agent) │   │  (production)  │
        └────────────────┘   └────────────────┘
                ▼                   ▼
        ┌────────────────────────────────────┐
        │ docker compose / process-compose   │
        └────────────────────────────────────┘
```

## The engine boundary

Everything below is implemented by the Go engine in this repository
(`angee-go`). It is intentionally generic: it knows nothing about Django,
React, or any specific framework.

| Concept | Role | Where it lives |
| --- | --- | --- |
| **Stack** | One `ANGEE_ROOT` containing `angee.yaml` plus generated runtime files. Materialized from a Stack template. | `internal/manifest/`, `internal/service/` |
| **Service** | A long-running workload. `runtime: container` → Docker Compose; `runtime: local` → process-compose. | `internal/runtime/` |
| **Job** | An explicitly invoked command with the same env, mount, and workdir handling as a Service. | `internal/service/` |
| **Source** | Reusable source material. Implemented kinds: `git` (cached and optionally worktreed) and `local` (path-mounted). | `internal/git/`, `internal/service/` |
| **Workspace** | A rendered Copier template at `$ANGEE_ROOT/workspaces/<name>` with materialized Sources, allocated ports, optional inner Stack. | `internal/copierx/`, `internal/service/` |
| **Operator** | The REST + GraphQL control-plane server for one root. | `internal/operator/` |
| **Secrets backend** | env-file by default; OpenBao for production. Resolved values land in `run/secrets.env`. | `internal/secrets/` |
| **Port pool** | Named ranges (`workspace`, `django`, `ui`, …) with leases, so workspaces don't collide. | `internal/ports/` |
| **Stack template** | A Copier template with `_angee.kind: stack` that produces an `angee.yaml`. | `internal/copierx/` |
| **Workspace template** | A Copier template with `_angee.kind: workspace` that produces a workspace tree, declares Sources to materialize, and may chain an inner Stack template. | `internal/copierx/` |

Everything here has a `service.Platform` method, a CLI command, and a
REST + GraphQL surface. See [Surface parity](/reference/surfaces).

## Above the engine

Angee is designed so application frameworks plug in *on top* of the
engine. The engine deploys whatever Services you declare; an
**application runtime** decides what those Services actually do, what
gets composed inside them, and how features are added.

| Term | Meaning | Status in `angee-go` |
| --- | --- | --- |
| **Host** | An application runtime that runs *inside* one or more of a Stack's Services — for example a Django process, a React build, or an MCP server. The Host is what end-user code talks to. | Not a manifest concept. The engine just runs Services. |
| **Block** | A unit of application code that contributes to the Host runtime — for example a Python pip distribution that adds models, GraphQL types, permissions, and React views. | Not a manifest concept. Defined by the Host. |
| **Build** | The Host's own build step (e.g. `manage.py angee build`) that composes Blocks into a deterministic `runtime/` tree before the Service starts. | Not invoked by the engine; usually a Job or a service entrypoint step. |

The engine treats a Host as just another container or local process. It
will mount Sources, set env, allocate ports, and start the Service — what
runs there (Django? Node? a static site? an agent loop?) is entirely up
to the Host.

### `angee-django` — the first default Host

[`angee-django`](https://github.com/fyltr/angee-django) is the first and
currently the default application runtime. It is a **Block compiler**
that produces a working Django + GraphQL + React application:

- Each Block is a pip distribution that contributes abstract models,
  GraphQL fragments, REBAC permissions, and React views.
- `manage.py angee build` composes every installed Block into a
  deterministic `runtime/` tree.
- The output runs as a single Django Service inside an Angee Stack.

`angee-django` ships its own Stack and Workspace Copier templates under
`templates/stacks/dev/` and `templates/workspaces/dev-pr/` — those
templates are what `angee init --dev` and
`angee workspace create <name> --template dev-pr` render when you work on a
Django consumer.

Other Hosts (a Node service, a Go API, a static site, anything that runs
in a container or as a local process) plug in the same way: ship a Stack
template that declares the right Services and Sources, and Angee will
pull, render, and run it.

## What "Self-Building" Looks Like

Putting the pieces together, a typical loop looks like this:

1. **Declare Sources.** Your app repos go into `angee.yaml` under
   `sources:`. Angee fetches them into a shared cache.
2. **Render a Workspace.** `angee workspace create fix-issue-123 --template dev-pr`
   renders a Copier template, materializes each Source as a worktree on
   `workspace/fix-issue-123`, allocates ports from the configured pool,
   and chains an inner Stack template so you have a runnable environment
   per feature.
3. **Develop or run agents.** Inside the Workspace, run `angee dev` to
   start container + local Services together. The Workspace can host a
   long-running agent process the same way it hosts a development
   server.
4. **Push.** `angee workspace push fix-issue-123` pushes each Source's
   workspace branch upstream. CI (or another operator) merges into
   `main`.
5. **Sync the production Stack.** The production root pulls those same
   Sources at the new ref and the operator brings the Stack up via
   `POST /stack/up`.

Stack and Workspace templates are the only place where the deployment
*shape* (which Services, which ports, which Sources) is declared.
Everything else is just running them.

## Where to next

- [Getting started](/guide/getting-started) — install and first commands.
- [Manifest](/guide/manifest) — `angee.yaml` schema and substitutions.
- [Templates](/guide/templates) — how Stack and Workspace templates are
  resolved and what the `_angee` metadata block declares.
- [Commands](/guide/commands) — full CLI surface.
- [Operator API](/reference/operator-api) — REST + GraphQL transports.
