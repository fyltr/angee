# angee

**The easiest way to spin up, maintain, and develop your AI stack. (docker-compose for AI project).**

Define your services, agents, and MCP servers in one file. Run one command. Get a fully operational platform with AI agents that can deploy, debug, and build alongside you.

```sh
angee init                    # scaffold your AI stack, generate secrets
angee up                      # start everything
angee admin                   # talk to your platform sysadmin agent
```

## What it looks like

```yaml
# angee.yaml — your entire stack in one file

services:
  web:
    image: myapp:latest
    lifecycle: platform
    domains: [{ host: myapp.io, port: 8000, tls: true }]

  postgres:
    image: pgvector/pgvector:pg17
    lifecycle: sidecar
    env:
      POSTGRES_PASSWORD: "${secret:db-password}"

mcp_servers:
  operator:
    transport: streamable-http
    url: http://operator:9000/mcp

skills:
  deploy:
    description: "Deploy, rollback, and manage the platform lifecycle"
    mcp_servers: [operator]
  code-review:
    description: "Review code changes and suggest improvements"

agents:
  admin:
    image: ghcr.io/anomalyco/opencode:latest
    lifecycle: system
    role: operator
    skills: [deploy]
    mcp_servers: [operator]
    env:
      ANTHROPIC_API_KEY: "${secret:anthropic-api-key}"

  developer:
    image: ghcr.io/anomalyco/opencode:latest
    lifecycle: system
    role: user
    skills: [code-review]

secrets:
  - name: db-password
    generated: true
  - name: anthropic-api-key
    required: true
```

That's services, agents, skills, MCP tool connections, and secrets — all in one place. Attach skills to agents to give them capabilities. `angee init` generates the passwords, prompts for API keys, and writes a ready-to-run `.env`. No manual credential setup.

## Install

```sh
# From source
go install github.com/fyltr/angee/cmd/angee@latest

# Or build locally
git clone https://github.com/fyltr/angee && cd angee
make build        # → dist/angee, dist/angee-operator
```

Requirements: Go 1.25+, Docker

## Quick start

### From a template

```sh
angee init                                  # default stack (Django + Postgres + Redis + agents)
angee init --template https://github.com/org/my-template   # custom template
angee up                                    # compile, start, done
```

### Add angee to an existing project

```sh
angee init --dir .angee --repo https://github.com/you/your-app
# edit .angee/angee.yaml to define your services and agents
angee up
```

`angee init` creates a `.angee/` directory (or `~/.angee/`) — a git-tracked workspace where your config, compiled manifests, agent workspaces, and secrets live. This is the key difference from Docker Compose: the AI stack is stateful. Agents have persistent workspaces. Every deploy is a git commit. Rollback is a revert.

## How it works

```
angee.yaml  →  compiler  →  docker-compose.yaml  →  docker compose up
                  │
                  ├── restart policies from lifecycle
                  ├── Traefik routing labels for platform services
                  ├── MCP server URLs injected into agent env
                  ├── ${secret:name} → .env interpolation
                  ├── agent config files rendered from templates
                  └── volumes, resources, health checks
```

Angee compiles your `angee.yaml` into `docker-compose.yaml` and manages the runtime. You never edit the compose file — it's generated. The angee-operator daemon handles compilation, deployment, and exposes an HTTP API + MCP endpoint so agents can manage the platform too.

## Two binaries

**`angee`** — CLI. What you type.

**`angee-operator`** — Daemon (port 9000). Owns the config repo, compiles, deploys, exposes the API that both the CLI and agents talk to.

```
you  →  angee CLI  →  angee-operator (:9000)  →  docker compose
                            ↑
               admin agent via MCP
```

## CLI

```
angee init [--template url]          Scaffold ANGEE_ROOT, generate secrets
angee up                             Compile and start the platform
angee down                           Stop everything
angee ls                             List services and agents
angee deploy [-m message]            Deploy current config
angee plan                           Dry-run — what would change
angee rollback <sha|HEAD~N>          Revert to a previous deploy
angee logs <service> [-f]            Tail logs
angee status                         Platform status

angee admin                          Chat with the admin agent
angee develop                        Chat with the developer agent
angee chat <agent>                   Chat with any agent
angee ask "do something"             One-shot message to an agent
```

## What goes in `angee.yaml`

| Section | What it defines |
|---------|----------------|
| `services` | Containers: web apps, databases, caches, workers, proxies |
| `agents` | AI agent containers with workspace, MCP connections, skills, config files |
| `mcp_servers` | MCP tool servers that agents connect to (stdio, SSE, streamable-http) |
| `skills` | Reusable agent capabilities — attach to any agent to give it new abilities |
| `secrets` | Credentials — generated, derived, or user-supplied. Written to `.env` |
| `repositories` | Source code repos linked to agent workspaces |

### Lifecycles

Every service and agent has a lifecycle that controls restart policy and routing:

| Lifecycle | Restart | Routing | Use for |
|-----------|---------|---------|---------|
| `platform` | unless-stopped | Traefik labels + domain | Web apps, APIs |
| `sidecar` | unless-stopped | internal only | Postgres, Redis |
| `worker` | unless-stopped | internal only | Celery, background jobs |
| `system` | always | varies | Traefik, operator, always-on agents |
| `agent` | unless-stopped | none | On-demand AI agents |
| `job` | no | none | One-shot tasks |

### Secrets

Docker Compose needs you to write `.env` by hand. Angee generates it.

Templates declare secrets with types — `generated` (random password), `derived` (built from other secrets), or required (user provides). `angee init` resolves them all and writes `.env` automatically.

```yaml
secrets:
  - name: db-password
    generated: true           # auto-generated 32-char password
    length: 32

  - name: db-url
    derived: "postgresql://app:${db-password}@postgres:5432/${project}"

  - name: anthropic-api-key
    required: true            # prompts user during init
```

In your services, reference them as `${secret:db-password}` — the compiler translates this to Docker Compose env interpolation. Secret values never appear in committed files.

### Skills

Skills are reusable capabilities you attach to agents. Instead of wiring each agent's MCP servers, prompts, and tools individually, define a skill once and attach it to any agent that needs it.

```yaml
skills:
  deploy:
    description: "Deploy, rollback, and manage the platform lifecycle"
    mcp_servers: [operator]
    system_prompt: "You can deploy and manage the platform..."

  code-review:
    description: "Review PRs and suggest improvements"
    mcp_servers: [github]

agents:
  admin:
    skills: [deploy]          # admin can deploy
  developer:
    skills: [code-review]     # developer can review code
  lead:
    skills: [deploy, code-review]  # lead can do both
```

Skills keep agent configs DRY — define the capability once, attach it wherever needed.

## Templates

A template bootstraps a complete AI stack: services, agents, MCP wiring, secrets, agent workspace files — from one command. Add a template to your project repo and anyone can spin up the full stack with `angee init`.

See [docs/TEMPLATES.md](docs/TEMPLATES.md) for the full template authoring guide.

## Runtime backends

The operator is runtime-agnostic. Same `angee.yaml`, same API, different backend:

| Version | Backend | Status |
|---------|---------|--------|
| v1 | Docker Compose | Implemented |
| v2 | Kubernetes | Planned |

```yaml
# operator.yaml
runtime: docker-compose   # or: kubernetes
```

## Documentation

- [docs/OVERVIEW.md](docs/OVERVIEW.md) — What Angee is and how it works
- [docs/OPERATOR.md](docs/OPERATOR.md) — The operator: architecture and design
- [docs/API.md](docs/API.md) — HTTP API reference
- [docs/MCP.md](docs/MCP.md) — MCP tools reference for agent authors
- [docs/TEMPLATES.md](docs/TEMPLATES.md) — Template authoring guide

## Development

```sh
make build            # Build operator + CLI → dist/
make test             # go test -v -race ./...
make lint             # golangci-lint
make check            # fmt + vet + lint + test
make run-operator     # Run operator against ~/.angee
make dev ARGS="init"  # Build CLI and run with args
```

Two dependencies: `cobra` (CLI) and `yaml.v3` (config parsing).

## License

Apache 2.0
