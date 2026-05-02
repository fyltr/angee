# Angee

Angee is agentic infrastructure as code. One config file, one command, a fully operational platform with AI agents that can manage themselves.

## The problem

You want to run a platform — a web app, a database, a task queue, a reverse proxy — and you want AI agents alongside it that can deploy, debug, and develop your code. Today you'd glue together Docker Compose, a bunch of env vars, MCP server configs, agent runtimes, credential plumbing, and a deployment pipeline. Every project reinvents this wiring.

## The idea

Angee extends Docker Compose to make AI agents first-class citizens alongside your services. You define everything in a single `angee.yaml` — apps, databases, workers, MCP servers, agents, secrets — and Angee compiles it into running infrastructure with all the wiring done.

```yaml
services:        # platform workloads (web, DB, Redis, workers)
mcp_servers:     # tool servers that agents connect to
agents:          # AI workloads (admin, developer, researcher, ...)
secrets:         # credentials that interconnect everything
repositories:    # source code repos agents work with
```

```sh
angee init       # scaffold the config, generate secrets
angee up         # start everything
angee admin      # talk to your platform admin agent
```

## Two modes

The `angee` binary serves **two complementary modes** (full detail in [`ARCHITECTURE.md` §12](./ARCHITECTURE.md#12-project-mode)):

- **Compose mode** (the original) — operator state for an agent stack. `angee init`, `angee up`, `angee deploy`, `angee chat` all live here. Everything below in this doc describes compose mode unless noted otherwise.
- **Project mode** *(new)* — drop-in for consumers of an Angee runtime framework (e.g. `django-angee`). The binary walks parents from CWD, finds `.angee/project.yaml`, and acts as a polyglot dispatcher (`angee build`/`migrate`/`doctor`/`fixtures` exec the framework) plus a dev orchestrator (`angee dev` spawns build watcher + dev server + frontend + extras with line-prefixed or pane-mode output).

Both modes coexist in the same shell — `angee dev` in a project tree and `angee up` against `~/.angee/` are orthogonal. Disambiguation is by manifest filename: `.angee/project.yaml` ⇒ project-mode; `.angee/angee.yaml` ⇒ compose-mode. See [`RUNTIMES.md`](./RUNTIMES.md) for the adapter-author guide.

## What goes in `angee.yaml`

### Services

Standard containers — your web app, database, cache, task queue, reverse proxy. Each service has a **lifecycle** that controls its behavior:

| Lifecycle | Restart | Routing | Example |
|-----------|---------|---------|---------|
| `platform` | unless-stopped | Traefik labels + domain | web app, API |
| `sidecar` | unless-stopped | internal only | Postgres, Redis |
| `worker` | unless-stopped | internal only | Celery worker |
| `system` | always | varies | Traefik, operator |
| `job` | no (one-shot) | none | migration, seed |

Services can declare domains, health checks, resource limits, volumes, replicas, build contexts, and environment variables — everything you'd put in a compose file, but in a cleaner schema.

### MCP servers

MCP (Model Context Protocol) servers are the tool layer. They give agents the ability to interact with the platform and external services. Angee wires them up automatically — agents declare which MCP servers they use, and Angee injects the connection URLs and credentials at deploy time.

```yaml
mcp_servers:
  angee-operator:
    transport: streamable-http
    url: http://operator:9000/mcp

  github:
    transport: sse
    url: https://api.githubcopilot.com/mcp/
    credentials:
      source: connect.account
      account_type: github

  filesystem:
    transport: stdio
    image: ghcr.io/fyltr/angee-filesystem-mcp:latest
    command: [node, /usr/local/lib/mcp-filesystem/dist/index.js]
    args: [/workspace]
```

Three transport types:
- **streamable-http / sse** — remote MCP servers, accessed via URL
- **stdio** — local MCP servers, run as a sidecar process inside the agent container

### Agents

AI agents are containers that run agentic coding tools (Claude Code, opencode, aider, or any terminal-based agent). Each agent gets:

- A persistent workspace directory
- MCP server connections (injected as environment variables)
- Config files rendered from templates (e.g., `opencode.json`)
- A role (`operator` for platform management, `user` for development)
- Resource limits (CPU, memory)
- API keys via secret references

```yaml
agents:
  admin:
    image: ghcr.io/anomalyco/opencode:latest
    lifecycle: system
    role: operator
    mcp_servers: [angee-operator, filesystem, django-mcp]
    workspace:
      persistent: true
    env:
      ANTHROPIC_API_KEY: "${secret:anthropic-api-key}"

  developer:
    image: ghcr.io/anomalyco/opencode:latest
    lifecycle: system
    role: user
    mcp_servers: [github, filesystem]
    workspace:
      repository: base
      branch: main
      persistent: true
    resources:
      cpu: "2.0"
      memory: "4Gi"
```

Agents are runtime-agnostic — the container image determines which agentic tool runs inside. Swap the image to switch between agent runtimes.

### Skills

Skills are reusable capability bundles that agents can reference. A skill packages MCP servers and system prompt additions into a named unit:

```yaml
skills:
  deploy:
    description: "Deployment capability"
    mcp_servers: [angee-operator]
    system_prompt: "You can deploy and manage the platform."

  code-review:
    description: "Code review capability"
    mcp_servers: [github]
    system_prompt: "You can review PRs and suggest improvements."

agents:
  admin:
    skills: [deploy]          # gets operator MCP + deploy prompt
  developer:
    skills: [deploy, code-review]  # gets both
```

When an agent references a skill, the compiler merges the skill's MCP servers into the agent's server list (deduplicating) and injects the skill's system prompt as an environment variable. This avoids repeating the same MCP server lists across agents and makes capability bundles composable.

### Agent templates

Templates bootstrap a complete platform from a single command. A template contains:

- `angee.yaml.tmpl` — Go template for the main config
- `.angee-template.yaml` — metadata: parameters, secrets, descriptions
- `*.tmpl` — config file templates (opencode.json, claude config, etc.)
- `agents/*/` — workspace scaffolding files for each agent

```sh
angee init --template https://github.com/fyltr/angee#templates/default
angee init --template ./my-custom-template
```

Templates handle secret generation (random passwords, derived connection strings), so `angee init` produces a ready-to-run platform with zero manual credential setup.

### Secrets

Secrets are declared in `angee.yaml` and resolved at init time. They live in `.env` (gitignored, never committed) and are injected into services and agents via `${secret:name}` references that the compiler translates into Docker Compose environment variable interpolation.

```yaml
secrets:
  - name: django-secret-key
    required: true
    generated: true          # auto-generated random string

  - name: db-password
    required: true
    generated: true

  - name: db-url
    required: true
    derived: "postgresql://angee:${db-password}@postgres:5432/${project}"

  - name: anthropic-api-key
    required: true            # must be supplied by user
```

Secret types:
- **generated** — random string with configurable length and charset
- **derived** — computed from other secrets (e.g., a DATABASE_URL built from a password)
- **supplied** — provided by the user via `--secret key=value` or interactive prompt

### Repositories

Source code repositories linked to the project. Agents with `workspace.repository` get these repos cloned into their workspace.

```yaml
repositories:
  base:
    url: https://github.com/org/app
    branch: main
    role: base
```

## How it works

### The compile loop

Angee doesn't talk to Docker directly. It compiles `angee.yaml` into a `docker-compose.yaml` (or, in v2, Kubernetes manifests) and delegates to the runtime.

```
angee.yaml  →  compiler  →  docker-compose.yaml  →  docker compose up
                   │
                   ├── Sets restart policies based on lifecycle
                   ├── Generates Traefik labels for platform services
                   ├── Injects MCP server URLs into agent env
                   ├── Resolves ${secret:name} → ${ENV_NAME}
                   ├── Renders agent config files from templates
                   └── Configures volumes, resources, health checks
```

### GitOps state management

ANGEE_ROOT (`~/.angee/`) is a git repository. Every deploy is a commit. Every rollback is a revert. The git log is a complete audit trail.

```
~/.angee/
├── angee.yaml                # source of truth (committed)
├── docker-compose.yaml       # generated, committed
├── operator.yaml             # local runtime config (gitignored)
├── .env                      # secrets (gitignored)
├── opencode.json.tmpl        # agent config templates (committed)
└── agents/
    ├── admin/workspace/      # admin agent's persistent workspace
    └── developer/workspace/  # developer agent's persistent workspace
```

### The operator

The angee-operator is a Go daemon that owns ANGEE_ROOT. It compiles, deploys, and monitors the platform. Two interfaces:

- **HTTP API** — for the CLI, dashboards, and apps
- **MCP endpoint** — for AI agents to manage the platform programmatically

The admin agent connects to the operator via MCP and can read config, deploy, rollback, check status, tail logs, and scale services — all through tool calls.

See [OPERATOR.md](OPERATOR.md), [API.md](API.md), and [MCP.md](MCP.md) for details.

### The CLI

```
angee init [--template url] [--secret k=v]    Scaffold ANGEE_ROOT
angee up                                       Start operator + deploy
angee down                                     Stop everything
angee deploy [-m message]                      Deploy angee.yaml
angee rollback <sha|HEAD~N>                    Roll back
angee plan                                     Dry-run deploy
angee pull                                     Pull latest container images
angee restart                                  Stop and restart all services
angee ls                                       List services and agents
angee status                                   Platform status
angee logs <service> [--follow]                Tail logs
angee chat [agent]                             Interactive agent session
angee admin                                    → angee chat admin
angee develop                                  → angee chat developer
angee ask "message" [--agent name]             One-shot message
angee update template                          Re-fetch and re-render template
```

## Runtime backends

The operator is runtime-agnostic. A `RuntimeBackend` interface abstracts all runtime interaction. Agents and apps talk to the operator API — they never know which runtime is underneath.

| Version | Backend | Compiles to | Status |
|---------|---------|-------------|--------|
| v1 | Docker Compose | `docker-compose.yaml` | Implemented |
| v2 | Kubernetes | Helm charts / manifests | Planned |

Switch backends with one line in `operator.yaml`:

```yaml
runtime: docker-compose   # or: kubernetes
```

Same `angee.yaml`, same API, same MCP tools, different runtime.

## What a real stack looks like

The default template produces a stack with the operator, Traefik, OpenBao, and an admin agent. You add application services via components:

| Service | Lifecycle | What it is |
|---------|-----------|------------|
| `operator` | system | angee-operator (HTTP API + MCP) |
| `traefik` | system | Reverse proxy, TLS termination |
| `openbao` | system | Secrets vault (KV v2) |

| Agent | Role | MCP servers | What it does |
|-------|------|-------------|--------------|
| `admin` | operator | operator, filesystem | Manages deployment, config, secrets |

Add more services and agents by editing `angee.yaml` or using `angee add`:

```sh
angee add postgres     # adds postgres service + db-password secret
angee add redis        # adds redis cache
```

## What Angee is not

- **Not a generic Docker management tool.** It manages a specific platform defined in `angee.yaml`, not arbitrary containers.
- **Not a PaaS.** You own the infrastructure. Angee is a config compiler and lifecycle manager.
- **Not an agent framework.** Angee doesn't build agents — it runs them. Bring any terminal-based agentic tool as a container image.
- **Not opinionated about your stack.** The default template is a minimal infrastructure base (operator + Traefik + OpenBao), but `angee.yaml` can describe any set of services. Templates are pluggable.
