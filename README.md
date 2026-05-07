# angee

**Angee is the stack manager for [angee.ai](https://angee.ai).** It compiles one declarative manifest (`angee.yaml`) into a running environment — secrets resolved, services supervised, workspaces provisioned — and exposes that environment over a stable HTTP+MCP control surface so the runtime above it can mutate the stack programmatically.

Think Docker Compose, but with workspaces, sources, secrets, port leases, and an HTTP/MCP API that lets the application layer self-manage its own stack.

## Two Consumers

Angee has exactly two callers, and the architecture flows from that:

1. **Humans, via the CLI** — `angee init`, `angee dev`, `angee up`, `angee workspace create`, etc. Terminal-driven setup, dev loop, day-to-day ops.
2. **Angee runtimes, via the operator HTTP+MCP API** — e.g. `django-angee` runs *inside* a stack Angee manages, and calls `POST /workspaces`, `POST /sources/<n>/pull`, `POST /services`, etc. to **self-manage the stack and self-update its sources**. Spinning up a session-scoped workspace, pulling fresh code into an existing worktree, rotating a secret, scaling a worker — all happen as HTTP calls from the runtime to its own operator, with no human in the loop. The same operations are exposed as MCP tools at `/mcp`.

Both paths go through one business-logic layer (`service.Platform`); the CLI and the HTTP/MCP server are thin adapters. Anything a human can do from a terminal, a runtime can do over HTTP, and vice versa.

## Target Model

- One manifest: `$ANGEE_ROOT/angee.yaml`.
- Templates are Copier templates with Angee metadata under `_angee`.
- The operator owns provisioning and reconciliation, and runs continuously alongside the runtime — in-process under `angee dev`, as a standalone daemon in production.
- The `angee` CLI has the operator compiled in. Local commands dispatch to the in-process operator runtime without opening ports; `--operator` / `ANGEE_OPERATOR_URL` switches to a remote operator over HTTP.
- Framework details belong in templates and rendered manifests, not in the Go CLI.

## Quick Start

```sh
angee init --yes
angee dev
```

`angee init` is shorthand for the default stack init, usually `angee stack init dev`.

Explicit stack initialization:

```sh
angee stack init dev --yes
angee dev
```

Staging-style stack:

```sh
angee stack init staging-docker \
  --set domain=staging.example.com \
  --secret anthropic-api-key=env:ANTHROPIC_API_KEY \
  --yes
angee up
```

## Commands

```sh
angee init [path]
angee stack init <name> [path]
angee stack update
angee dev
angee up
angee deploy
angee status
angee logs <service>
angee workspace init <name>
angee workspace update <name>
angee agent init <name>
angee agent update <name>
angee agent list
angee agent logs <name>
```

Full target usage reference: [`docs/USAGE.md`](./docs/USAGE.md).

## Development

```sh
make build
make test
```

Current refactor plan: [`docs/REFACTOR.md`](./docs/REFACTOR.md).
