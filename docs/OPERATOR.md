# The angee-operator

The angee-operator is the control plane for a platform defined in `angee.yaml`. It owns ANGEE_ROOT — a git-managed directory that is the single source of truth for your entire stack — and exposes two interfaces for managing it: an HTTP API (for apps, dashboards, CLIs) and an MCP endpoint (for AI agents).

## What it does

The operator sits between your configuration and your runtime. Its job is a compile loop:

1. **Read** `angee.yaml` from ANGEE_ROOT
2. **Compile** it into runtime-specific manifests (docker-compose.yaml, k8s manifests, etc.)
3. **Apply** the manifests to the runtime backend
4. **Observe** the running state and report it back
5. **Record** every change as a git commit for rollback and audit

Every mutation — config changes, deploys, rollbacks — goes through this loop. The git history in ANGEE_ROOT is a complete audit trail of what was deployed and when.

## Runtime-agnostic by design

The operator never calls Docker or Kubernetes APIs directly. All runtime interaction goes through the `RuntimeBackend` interface:

```go
type RuntimeBackend interface {
    Diff(ctx, composeFile)    // What would change?
    Apply(ctx, composeFile)   // Make it match the desired state
    Status(ctx)               // What's running now?
    Logs(ctx, service, opts)  // Stream logs
    Scale(ctx, service, n)    // Adjust replicas
    Stop(ctx, services...)    // Stop services
    Down(ctx)                 // Tear down the stack
}
```

The v1 implementation is Docker Compose — it compiles `angee.yaml` into `docker-compose.yaml` and shells out to `docker compose`. But the API and MCP surface are the same regardless of backend. Agents and apps that talk to the operator don't know or care which runtime is underneath.

| Backend | Status | Compiles to |
|---------|--------|------------|
| Docker Compose | Implemented | `docker-compose.yaml` |
| Docker Swarm | Planned | Swarm stack files |
| Kubernetes | Planned | Helm charts / manifests |

The `operator.yaml` file (gitignored, local to each host) selects which backend to use:

```yaml
runtime: docker-compose   # or: kubernetes
```

## Two interfaces

### HTTP API

Traditional REST API on port 9000. Used by the CLI (`angee`), web dashboards, CI/CD pipelines, and any application that needs to manage the platform. See [API.md](API.md) for the full reference.

### MCP (Model Context Protocol)

Streamable-HTTP endpoint at `/mcp`. AI agents connect to the operator as an MCP server and get tools for deploying, observing, and managing the platform through structured JSON-RPC calls. See [MCP.md](MCP.md) for the tool reference.

Both interfaces expose the same capabilities — they're two views of the same control plane.

## What it manages

- **Configuration** — Read and write `angee.yaml`, with validation and git commits
- **Deployments** — Compile and apply, with dry-run (plan) and rollback
- **Services** — Status, logs, scaling, stop, teardown
- **Agents** — List, start, stop, logs for AI agents defined in the config
- **Repositories** — Clone, pull, checkout, status for source repos *(planned)*
- **Git history** — Full audit trail of every config change and deploy

## Security model

- **API key auth**: All endpoints except `/health` require a `Bearer` token when `ANGEE_API_KEY` is set (env var or `operator.yaml`)
- **CORS**: Configurable allowed origins with wildcard support
- **Docker socket**: The operator needs access to the Docker socket (or kube config) to manage the runtime. It runs as a privileged service within the platform
- **Network isolation**: Services run on an internal Docker network (`angee-net`). Only services with `platform` lifecycle get Traefik routing labels and external exposure

## How it differs from Docker's MCP tools

Docker has its own MCP ecosystem:

- **Docker MCP Gateway** — Runs MCP servers in isolated containers, proxies connections
- **Docker MCP Catalog** — Registry of MCP server images
- **mcp-server-docker** / **docker-mcp** — Generic Docker management: create containers, manage images, inspect networks

The angee-operator is not a generic Docker tool. It doesn't manage arbitrary containers or images. It manages a **specific platform** defined in `angee.yaml` — a known set of services, agents, MCP servers, and repositories with defined lifecycles, domains, and relationships.

| | Generic Docker MCP | angee-operator |
|---|---|---|
| Scope | Any container/image/network | One platform defined in angee.yaml |
| State management | Stateless commands | GitOps — every change is a commit |
| Configuration | None — imperative commands | Declarative YAML with validation |
| Rollback | Manual | `POST /rollback` to any commit SHA |
| Agents | Not a concept | First-class citizens with lifecycle management |
| Compile step | None | angee.yaml → runtime manifests |
| Runtime | Docker only | Pluggable backend (Compose, Swarm, K8s) |

## ANGEE_ROOT layout

```
~/.angee/                     # or wherever ANGEE_ROOT points
├── angee.yaml                # source of truth (committed)
├── docker-compose.yaml       # generated by compiler (committed)
├── operator.yaml             # local runtime config (gitignored)
├── .env                      # secrets (gitignored)
├── .gitignore
└── agents/
    └── <name>/
        └── workspace/        # per-agent persistent workspace
```

## The compile loop in detail

When a deploy is triggered (`POST /deploy`), the operator:

1. Loads and validates `angee.yaml`
2. Ensures per-agent directories exist under `agents/`
3. Renders agent config files from templates (Claude config, MCP settings, etc.)
4. Compiles `angee.yaml` into `docker-compose.yaml` through the `Compiler`
   - Sets restart policies based on lifecycle
   - Generates Traefik labels for `platform` services
   - Injects environment variables for agents (API keys, MCP server URLs)
   - Configures volume mounts, resource limits, health checks
5. Calls `Backend.Apply()` to reconcile the running state
6. Logs the result (services started, updated, removed)

The `GET /plan` endpoint runs steps 1-4 and then calls `Backend.Diff()` instead of `Apply()`, showing what *would* change without touching the runtime.
