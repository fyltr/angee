# Implementation Plan

This is the active build plan for the clean Angee CLI and operator implementation in this folder.

## Invariants

- `service.Platform` is the only business-logic layer.
- CLI, HTTP, and MCP are adapters over the same operations.
- One operator process serves exactly one `ANGEE_ROOT`.
- `angee up` starts container services only.
- `angee dev` starts container services, local processes, and an in-process operator.
- Resolved secrets never enter committed files.
- A stack with only `services:` and `jobs:` works without workspace or source code paths.
- Workspaces are the only provisioning primitive beyond services and jobs.

## Phase 1: Foundation

Status: complete.

Delivered:

- Standalone Go module under `new/`.
- CLI and operator entrypoints.
- Shared API request/response types.
- Manifest schema, strict load/save, validation, and path rules.
- Substitution engine with namespaces and filters.
- Env-file secrets backend with generated and imported secrets.
- Port pool allocation.
- Filesystem advisory locks.
- Runtime backend interface and generated Compose/process-compose document types.
- `service.Platform` with stack load, status, compile, and prepare.
- `angee internal stack compile` and `angee internal stack prepare`.
- Minimal operator server with `/healthz`, `/stack/status`, and `/stack/prepare`.
- Tests for manifest round-trip, substitutions, secret safety, env-file backend, port pools, locks, CLI, and operator guardrails.

Acceptance:

- `go test -race ./...`
- `make build`

## Phase 2: Container Runtime

Status: complete.

Goal: make container services fully runnable through Docker Compose.

Deliver:

- Compose renderer for container services, volumes, ports, env, command, build, and working directory.
- Docker Compose backend that shells out only through the runtime adapter.
- `angee build`, `angee up`, `angee down`, `angee logs`, `angee start`, `angee stop`, `angee restart`, and `angee status`.
- Container service init, update, destroy, build, and lifecycle operations.
- Secret-safe generated `docker-compose.yaml` plus runtime `.env` wiring.

Acceptance:

- A postgres/redis stack starts and stops cleanly.
- A build-backed service builds with `angee build` and starts with `angee up`.
- Resolved secret values are present only in gitignored env files.
- A dynamically added container service can start and be removed without restarting the full stack.

## Phase 3: Local Runtime

Status: complete.

Goal: make local process services runnable through process-compose and complete the local dev loop.

Deliver:

- process-compose renderer for local services, env, command, workdir, and dependencies.
- process-compose backend for local lifecycle operations.
- `angee dev` with container sidecars and local processes.
- Consumer-aware address rewriting for service and operator references.
- Operator URL/token injection into supervised local processes.

Acceptance:

- A mixed container/local stack runs under `angee dev`.
- A local process can call `/healthz` through `ANGEE_OPERATOR_URL`.
- Ctrl+C tears down local processes cleanly while container sidecars remain reusable.

## Phase 4: Workspaces And Source Read Path

Status: complete.

Goal: materialize workspaces and mount them into services.

Deliver:

- Source records, workspace records, mount URI parsing, and workdir URI resolution.
- Source cache materialization for `git` and `local` sources.
- Git clone, fetch, worktree add, worktree list, and status.
- Workspace create, update, start, stop, restart, logs, destroy, list, and get.
- In-process template-chain rendering.
- Inner ANGEE_ROOT preparation and lifecycle routing.
- Workspace TTL fields and port leases.

Acceptance:

- A workspace template renders into `$ANGEE_ROOT/workspaces/<name>/`.
- A git source can be materialized as a worktree under a workspace.
- A service can mount the workspace and start successfully.
- Workspace list/get report path, template, TTL, sources, and lifecycle state.

## Phase 5: Source Write Path

Status: complete.

Goal: let the operator publish committed worktree changes safely.

Deliver:

- Git pull and push operations with fast-forward and dirty-worktree checks.
- Source pull, source push, workspace git status, and workspace push.
- Dirty-worktree checks before push.

Acceptance:

- A committed change in a worktree is visible through workspace git status.
- Workspace push publishes committed changes and refuses dirty or untracked worktrees.

## Phase 6: Full Operator Surface

Status: complete.

Goal: expose the complete platform API over HTTP, SSE, and MCP.

Deliver:

- Stack, service, job, workspace, source, event, and MCP endpoints.
- Bearer auth for protected endpoints.
- Event stream.

Acceptance:

- An external runtime can create a workspace, start a service that mounts it, observe readiness, and reach the service without shell access.

## Phase 7: OpenBao Secrets

Status: complete.

Goal: support OpenBao as a native secrets backend.

Deliver:

- OpenBao KV backend over `net/http`.
- Backend selection through `secrets_backend.type`.
- Runtime env-file materialization for OpenBao-backed secrets.

## Phase 8: Polish

Status: complete.

Goal: harden lifecycle edges and documentation.

Deliver:

- Port lease release on workspace destroy.
- Purge semantics for stack and workspace runtime directories.
- Starter stack and workspace templates.
- README, active implementation plan, and command/API documentation.
