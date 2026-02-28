# Templates

An Angee template is a directory that bootstraps an entire AI-powered platform from `angee init`. If you've built an application and want others (or your own agents) to spin it up with one command, you create a template.

## Why templates exist

Docker Compose requires you to write a `docker-compose.yaml` and manually create a `.env` with all the secrets your stack needs. Miss one variable and the stack fails to start. Add AI agents to the mix — each with their own workspace, MCP server connections, config files, and API keys — and the manual setup becomes painful.

Angee templates solve this. A template defines the complete stack — services, agents, MCP servers, secrets — and `angee init` handles all the wiring:

- Renders `angee.yaml` with project-specific values
- **Generates secrets automatically** (passwords, keys) and writes them to `.env`
- Derives compound secrets from simple ones (e.g., `DATABASE_URL` from `db-password`)
- Creates per-agent workspace directories with scaffolding files
- Copies config file templates for agent runtimes
- Initializes a git repository for gitops state management
- Compiles the first `docker-compose.yaml` so `angee up` works immediately

The result is a ready-to-run `.angee/` directory. No manual env setup, no credential copying, no config file authoring.

## How templates differ from Docker Compose

| | Docker Compose | Angee template |
|---|---|---|
| Output directory | Current directory | `~/.angee/` (or custom path) — a git repo |
| Secrets | You create `.env` manually | Template generates and manages `.env` |
| Secret types | None — all manual | Generated, derived, prompted, supplied |
| AI agents | Not a concept | First-class: workspaces, MCP wiring, config templates |
| State | Stateless files | Git-tracked: every deploy is a commit, rollback = revert |
| Config files | Static | Go templates with project-specific parameters |
| Workspaces | None | Per-agent persistent directories with scaffolding |
| Init experience | Write files by hand | `angee init --template url` → running platform |

The core difference: Docker Compose describes containers. Angee templates describe a **platform** — containers, agents, their tool connections, their credentials, their workspaces, and the secret graph that wires it all together.

## Template structure

```
my-template/
├── .angee-template.yaml      # metadata: parameters, secrets, description
├── angee.yaml.tmpl            # Go template → rendered to angee.yaml
├── opencode.json.tmpl         # agent config template (optional, any name)
├── claude-config.json.tmpl    # another agent config template (optional)
└── agents/                    # agent workspace scaffolding (optional)
    ├── my-admin/
    │   └── AGENTS.md           # instructions for the admin agent
    └── my-developer/
        └── AGENTS.md           # instructions for the developer agent
```

### Required files

**`.angee-template.yaml`** — Template metadata. Declares parameters users can customize and secrets the template needs.

**`angee.yaml.tmpl`** — Go template that renders into the project's `angee.yaml`. This is the source of truth for the entire platform.

### Optional files

**`*.tmpl`** (other than `angee.yaml.tmpl`) — Config file templates for agent runtimes. These are copied into ANGEE_ROOT and rendered at deploy time with agent-specific data (MCP server connections, agent name, etc.).

**`agents/<name>/`** — Workspace scaffolding. Files here are copied into `agents/<name>/workspace/` during init. Use this for agent instructions (`AGENTS.md`), starter configs, or seed data.

## `.angee-template.yaml` reference

```yaml
name: my-stack-template
description: |
  My application stack with Postgres, Redis, and AI agents.
version: "1.0"
author: your-org

# Parameters — customizable values injected into angee.yaml.tmpl
parameters:
  - name: ProjectName
    description: "Project name (lowercase, hyphens)"
    required: true
    validate: "^[a-z][a-z0-9-]+$"

  - name: Domain
    description: "Primary domain"
    default: "localhost"

  - name: DBSize
    description: "Postgres volume size"
    default: "20Gi"

# Informational — which services and agents the template creates
services:
  - web
  - postgres
  - redis

agents:
  - admin
  - developer

# Secrets — processed by `angee init`
secrets:
  - name: django-secret-key
    description: "Application secret key"
    required: true
    generated: true
    length: 50
    charset: "abcdefghijklmnopqrstuvwxyz0123456789!@#%^&*(-_=+)"

  - name: db-password
    description: "Database password"
    required: true
    generated: true
    length: 32
    charset: "abcdefghijklmnopqrstuvwxyz0123456789"

  - name: db-url
    description: "Full DATABASE_URL"
    required: true
    derived: "postgresql://app:${db-password}@postgres:5432/${project}"

  - name: anthropic-api-key
    description: "Anthropic API key for AI agents"
    required: true
```

### Parameters

Parameters are injected into `angee.yaml.tmpl` during `angee init`. Users can customize them interactively or accept defaults with `--yes`.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Go template field name (PascalCase) |
| `description` | string | Shown to user during interactive init |
| `default` | string | Value used when user accepts defaults |
| `required` | bool | Error if not provided and no default |
| `validate` | string | Regex pattern for validation |

Parameters are available in `angee.yaml.tmpl` as `{{ .FieldName }}`. The built-in parameters are:

- `{{ .ProjectName }}` — derived from directory name or prompted
- `{{ .Domain }}` — primary domain, defaults to `localhost`

Templates can add any custom parameters (e.g., `DBSize`, `DjangoWorkers`, `CeleryCPU`).

### Secrets

This is where Angee diverges most from Docker Compose. Instead of requiring users to manually create `.env`, templates declare what secrets the stack needs, and `angee init` resolves them automatically.

#### Secret types

**Generated** — Random cryptographic string. Angee produces it during init. The user never has to think about it.

```yaml
- name: db-password
  generated: true
  length: 32
  charset: "abcdefghijklmnopqrstuvwxyz0123456789"
```

**Derived** — Computed from other secrets. Use `${other-secret}` to reference another secret's value, and `${project}` for the project name.

```yaml
- name: db-url
  derived: "postgresql://app:${db-password}@postgres:5432/${project}"
```

Derived secrets are resolved in a second pass, after all generated/supplied secrets are available. This lets you compose connection strings, DSNs, and compound credentials from their parts.

**Supplied** — Must be provided by the user. Either via `--secret` flag or interactive prompt during init.

```yaml
- name: anthropic-api-key
  required: true
  # no `generated` or `derived` — user must provide it
```

**Optional** — Not required. If not supplied and not generated, it's simply omitted from `.env`.

```yaml
- name: sentry-dsn
  description: "Sentry error tracking DSN"
  required: false
```

#### Secret resolution order

When `angee init` runs, each secret is resolved in this order:

1. **`--secret key=value` flag** — always wins, overrides everything
2. **`generated: true`** — auto-generate a random value
3. **Interactive prompt** — ask the user (skipped with `--yes`)
4. **`derived`** — compute from other resolved secrets (second pass)
5. **Error** if `required: true` and nothing resolved it

#### Secret fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | — | Secret name, also used as env var key (uppercased, hyphens → underscores) |
| `description` | string | — | Shown during interactive prompt |
| `required` | bool | false | Error if missing |
| `generated` | bool | false | Auto-generate random value |
| `derived` | string | — | Expression: `${other-secret}`, `${project}` |
| `length` | int | 50 | Length of generated value |
| `charset` | string | `a-z0-9!@#%^&*(-_=+)` | Characters to use for generation |

#### How secrets flow into containers

1. `angee init` resolves secrets → writes `.env` with `KEY=value` lines
2. `angee.yaml` references secrets as `${secret:name}` in env values
3. The compiler translates `${secret:db-password}` → `${DB_PASSWORD}`
4. Docker Compose interpolates `${DB_PASSWORD}` from `.env` at runtime

The secret values never appear in `angee.yaml` or `docker-compose.yaml` — both are committed to git. Only `.env` has the actual values, and it's gitignored.

## `angee.yaml.tmpl`

A standard Go `text/template`. The template receives a `TemplateParams` struct:

```go
type TemplateParams struct {
    ProjectName   string
    Domain        string
    DBSize        string
    RedisSize     string
    MediaSize     string
    DjangoWorkers string
    DjangoMemory  string
    DjangoCPU     string
    CeleryWorkers string
    CeleryMemory  string
    CeleryCPU     string
}
```

Example usage:

```yaml
name: {{ .ProjectName }}

services:
  web:
    image: myapp:latest
    lifecycle: platform
    domains:
      - host: {{ .Domain }}
        port: 8000
        tls: {{ if eq .Domain "localhost" }}false{{ else }}true{{ end }}
    env:
      DATABASE_URL: "${secret:db-url}"
      SECRET_KEY: "${secret:app-secret-key}"

  postgres:
    image: postgres:17
    lifecycle: sidecar
    env:
      POSTGRES_PASSWORD: "${secret:db-password}"
    volumes:
      - name: pgdata
        path: /var/lib/postgresql/data
        persistent: true
        size: {{ .DBSize }}

agents:
  admin:
    image: ghcr.io/anomalyco/opencode:latest
    lifecycle: system
    role: operator
    mcp_servers: [angee-operator]
    env:
      ANTHROPIC_API_KEY: "${secret:anthropic-api-key}"
```

The rendered output becomes the project's `angee.yaml` — committed to git, editable by users and agents after init.

## Agent config templates

Templates can include config files that are rendered at **deploy time** (not init time) with agent-specific data. These use a different data context than `angee.yaml.tmpl`.

Each agent config template receives:

```go
type AgentFileData struct {
    AgentName  string                         // e.g. "admin"
    Agent      AgentSpec                      // full agent config from angee.yaml
    MCPServers map[string]MCPServerSpec       // only this agent's MCP servers
}
```

And has access to template functions:

| Function | Description |
|----------|-------------|
| `opencodeMCP .MCPServers` | Generates a complete `opencode.json` config from MCP server specs |
| `toJSON .Value` | Marshals any value to indented JSON |

Example — `opencode.json.tmpl`:

```
{{ opencodeMCP .MCPServers }}
```

This produces a valid `opencode.json` with all MCP server connections configured for that specific agent. The agent references this in `angee.yaml`:

```yaml
agents:
  admin:
    files:
      - template: opencode.json.tmpl
        mount: /root/.config/opencode/opencode.json
```

At deploy time, the operator renders the template and either bind-mounts the result into the container or places it in the agent's workspace directory (if the mount path is under `/workspace/`).

## Agent workspace scaffolding

The `agents/` directory in a template contains per-agent workspace files. During `angee init`, these are copied into `agents/<name>/workspace/` in ANGEE_ROOT.

Use this for:

- **`AGENTS.md`** — Instructions that tell the agent its role, what tools it has, and how to behave
- Starter configs, seed data, or README files
- Anything the agent should find in its workspace on first run

Example — `agents/my-admin/AGENTS.md`:

```markdown
# Admin Agent

You are the platform admin. You manage deployments, config, and infrastructure.

## MCP servers available
- **angee-operator** — deploy, rollback, scale, status, logs
- **angee-files** — filesystem access

## Guidelines
- Explain actions before taking them
- Confirm destructive operations with the user
- Never expose secret values
```

Files are only copied if they don't already exist — re-running init won't overwrite customized workspace content.

## Hosting templates

Templates can be:

**A local directory:**
```sh
angee init --template ./my-template
```

**A Git repository:**
```sh
angee init --template https://github.com/org/my-template
```

**A subdirectory in a Git repository** (using `#fragment`):
```sh
angee init --template https://github.com/org/repo#templates/my-stack
```

Git templates are shallow-cloned into a temp directory, used for init, then cleaned up.

## What `angee init` does with a template

1. Fetch the template (clone if Git URL, read if local path)
2. Load `.angee-template.yaml` for parameter and secret definitions
3. Prompt for parameters (or use `--yes` for defaults)
4. Resolve secrets: `--secret` flags → generate → prompt → derive
5. Create ANGEE_ROOT directory with git repo, `.gitignore`, subdirectories
6. Render `angee.yaml.tmpl` → `angee.yaml`
7. Copy `*.tmpl` agent config templates into ANGEE_ROOT
8. Write `.env` with resolved secrets (mode 0600, gitignored)
9. Write `operator.yaml` with runtime defaults (gitignored)
10. Create per-agent directories with stub `.env` files
11. Copy agent workspace scaffolding from `agents/`
12. Compile `angee.yaml` → `docker-compose.yaml`
13. Create initial git commit

After init, `angee up` starts the operator and deploys the stack. Zero manual setup between init and a running platform.

## Writing a template from scratch

### Minimal template

The simplest possible template — one service, one agent, one secret:

**`.angee-template.yaml`:**
```yaml
name: minimal
description: "Minimal template — one service, one agent"
version: "1.0"

parameters:
  - name: ProjectName
    required: true

secrets:
  - name: api-key
    description: "API key for the agent"
    required: true
```

**`angee.yaml.tmpl`:**
```yaml
name: {{ .ProjectName }}

services:
  app:
    image: nginx:alpine
    lifecycle: platform
    domains:
      - host: localhost
        port: 80

mcp_servers:
  angee-operator:
    transport: streamable-http
    url: http://operator:9000/mcp

agents:
  admin:
    image: ghcr.io/anomalyco/opencode:latest
    lifecycle: system
    role: operator
    mcp_servers: [angee-operator]
    env:
      ANTHROPIC_API_KEY: "${secret:api-key}"
```

That's it. `angee init --template ./minimal --secret api-key=sk-...` produces a working platform.

### Full template

See `templates/default/` in this repo for a production template with Django, Postgres, Redis, Celery, two agents, three MCP servers, and four secrets (two generated, one derived, one user-supplied).

## Template design guidelines

1. **Generate what you can.** Database passwords, secret keys, tokens — if it doesn't require external credentials, make it `generated: true`. Users shouldn't have to invent passwords.

2. **Derive compound secrets.** If a service needs a connection string and you already generate the password, use `derived` to compose the full URL. One generated value feeds many.

3. **Only prompt for external credentials.** API keys (Anthropic, OpenAI, GitHub), OAuth tokens, external service URLs — these are the only things users should have to provide.

4. **Include agent instructions.** An `AGENTS.md` in the workspace scaffolding tells the agent what it can do and how to behave. Without it, the agent starts cold.

5. **Use config file templates.** Different agent runtimes (opencode, Claude Code, aider) need different config formats. Template them so the MCP wiring is automatic.

6. **Keep parameters minimal.** Project name and domain are usually enough. Resource limits and volume sizes can have sensible defaults. Don't ask users to configure things they don't care about at init time — they can edit `angee.yaml` later.
