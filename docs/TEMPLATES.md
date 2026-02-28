# Templates

An Angee template is a directory that bootstraps an entire AI-powered platform from `angee init`. Templates define the complete stack — services, agents, MCP servers, secrets — and `angee init` handles all the wiring.

## Two-phase template system

Angee templates render at **two different times** with different data:

### Phase 1: Init time (`angee init`)

When you run `angee init`, this happens in order:

1. **Detect template source** — check for `.angee-template/` in the current directory, or use `--template` flag
2. **Fetch** — if git URL, shallow-clone to temp dir (supports `repo#subdir` fragments). If local path, use directly
3. **Load metadata** — read `.angee-template.yaml` for parameter and secret definitions
4. **Gather parameters** — prompt interactively or accept defaults with `--yes`
5. **Resolve secrets** — in priority order:
   - `--secret name=value` flags (always win)
   - `generated: true` → random cryptographic string
   - Interactive prompt (if required and not supplied, skipped with `--yes`)
   - `derived` → second pass, substitutes `${other-secret}` and `${project}` references
6. **Create ANGEE_ROOT** — initialize directory with git repo, `.gitignore`, subdirectories
7. **Render `angee.yaml.tmpl`** → `angee.yaml` using Go `text/template` with `TemplateParams`
8. **Copy `*.tmpl` files** — other template files (like `opencode.json.tmpl`) copied as-is to ANGEE_ROOT for deploy-time rendering
9. **Write `.env`** — resolved secrets (mode 0600, gitignored, never committed)
10. **Write `operator.yaml`** — local runtime config (gitignored)
11. **Create agent directories** — with stub `.env` files
12. **Copy agent workspace scaffolding** — `AGENTS.md` and other files from template `agents/` directory
13. **Compile** — `angee.yaml` → `docker-compose.yaml` so `angee up` works immediately
14. **Initial git commit**

After init, `angee up` starts everything. Zero manual setup.

### Phase 2: Deploy time (every `angee up`, `deploy`, `pull`, `restart`)

Before every deploy, agent config file templates are re-rendered with fresh, agent-specific data:

```
For each agent:
  1. Filter MCP servers to only those the agent references
  2. Read the agent's files[].template from ANGEE_ROOT
  3. Render with AgentFileData (agent name, config, filtered MCP servers)
  4. Write to agents/<name>/ directory
```

This means MCP server connections are always current — if you change an MCP server URL in `angee.yaml`, the next deploy automatically updates every agent's config.

## `.angee-template/` folder convention

The simplest way to add angee to an existing project: create a `.angee-template/` folder in your repository root.

```
my-project/
├── .angee-template/           # Template blueprint (committed to your repo)
│   ├── .angee-template.yaml   # Metadata: parameters, secrets
│   ├── angee.yaml.tmpl        # Go template → angee.yaml
│   ├── opencode.json.tmpl     # Agent config template (rendered at deploy time)
│   └── agents/
│       ├── admin/
│       │   └── AGENTS.md      # Admin agent workspace scaffolding
│       └── developer/
│           └── AGENTS.md      # Developer agent workspace scaffolding
├── src/                       # Your application code
├── Dockerfile
└── README.md
```

**How it works:**

```sh
cd my-project
angee init                  # detects .angee-template/, uses it
angee up                    # starts everything
```

When `angee init` runs without `--template`:

1. Checks if `.angee-template/` exists in the current directory
2. If found, uses it as the template source
3. Sets ANGEE_ROOT to `.angee/` in the current directory (not `~/.angee/`)

This keeps everything local to the project. The `.angee/` directory is the working copy — it has its own git repo, secrets, and runtime state.

```sh
angee init --template https://github.com/org/project
```

When pointing `--template` at a git repo, angee clones it and looks for `.angee-template/` inside. If found, uses that subdirectory. Otherwise uses the repo root.

| | `.angee-template/` | `.angee/` (ANGEE_ROOT) |
|---|---|---|
| **Purpose** | Blueprint — committed to your project repo | Working copy — managed by angee |
| **Created by** | You (the template author) | `angee init` |
| **Contains** | `.tmpl` files, metadata, scaffolding | Rendered configs, runtime state, secrets |
| **Committed** | Yes, to your project's git repo | Has its own git repo |
| **Editable** | Yes, update and re-render with `angee update template` | Yes, edited by users and agents |

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

**`*.tmpl`** (other than `angee.yaml.tmpl`) — Config file templates for agent runtimes. These are copied into ANGEE_ROOT at init time and rendered at deploy time with agent-specific data.

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

## Secrets

This is where Angee diverges most from Docker Compose. Instead of requiring users to manually create `.env`, templates declare what secrets the stack needs, and `angee init` resolves them automatically.

### Secret types

**Generated** — Random cryptographic string. Angee produces it during init. The user never sees or thinks about it.

```yaml
- name: db-password
  generated: true
  length: 32
  charset: "abcdefghijklmnopqrstuvwxyz0123456789"
```

**Derived** — Computed from other secrets. Use `${other-secret}` to reference another secret's value, and `${project}` for the project name. Resolved in a second pass after all generated/supplied secrets are available.

```yaml
- name: db-url
  derived: "postgresql://app:${db-password}@postgres:5432/${project}"
```

**Supplied** — Must be provided by the user. Either via `--secret` flag or interactive prompt during init.

```yaml
- name: anthropic-api-key
  required: true
  # no `generated` or `derived` — user must provide it
```

**Optional** — Not required. If not supplied and not generated, omitted from `.env`.

```yaml
- name: sentry-dsn
  description: "Sentry error tracking DSN"
  required: false
```

### Secret resolution order

1. **`--secret key=value` flag** — always wins
2. **`generated: true`** — auto-generate random value
3. **Interactive prompt** — ask the user (skipped with `--yes`)
4. **`derived`** — compute from other resolved secrets (second pass)
5. **Error** if `required: true` and nothing resolved it

### Secret fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | — | Secret name, also used as env var key (uppercased, hyphens → underscores) |
| `description` | string | — | Shown during interactive prompt |
| `required` | bool | false | Error if missing |
| `generated` | bool | false | Auto-generate random value |
| `derived` | string | — | Expression: `${other-secret}`, `${project}` |
| `length` | int | 50 | Length of generated value |
| `charset` | string | `a-z0-9!@#%^&*(-_=+)` | Characters to use for generation |

### How secrets flow into containers

```
angee.yaml:       DATABASE_URL: "${secret:db-url}"
     ↓ compiler
compose.yaml:     DATABASE_URL: "${DB_URL}"
     ↓ docker compose reads .env
runtime:          DATABASE_URL=postgresql://angee:abc123@postgres:5432/myapp
```

1. `angee init` resolves secrets → writes `.env` with `KEY=value` lines
2. `angee.yaml` references secrets as `${secret:name}` in env values
3. The compiler translates `${secret:db-password}` → `${DB_PASSWORD}`
4. Docker Compose interpolates `${DB_PASSWORD}` from `.env` at runtime

Secret values never appear in `angee.yaml` or `docker-compose.yaml` — both are committed to git. Only `.env` has the actual values, and it's gitignored.

## Init-time template: `angee.yaml.tmpl`

A standard Go `text/template`. Receives a `TemplateParams` struct:

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

Example:

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

## Deploy-time templates: agent config files

Templates can include config files that are rendered at **deploy time** (not init time) with agent-specific data. These use a different data context than `angee.yaml.tmpl`.

Each agent config template receives:

```go
type AgentFileData struct {
    AgentName  string                         // e.g. "admin"
    Agent      AgentSpec                      // full agent config from angee.yaml
    MCPServers map[string]MCPServerSpec       // only this agent's MCP servers
}
```

Template functions available:

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

### How file mounts work

The `mount` path determines where the rendered file ends up:

- **Mount under `/workspace/`** — written to `agents/<name>/workspace/`, included in the workspace bind mount (no separate volume needed)
- **Mount elsewhere** (e.g., `/root/.config/app.json`) — written to `agents/<name>/`, gets a separate read-only volume mount added by the compiler

At deploy time, the compiler renders the template and injects the appropriate volume mount into the agent's docker-compose service definition.

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

**A `.angee-template/` folder in the current directory** (auto-detected):
```sh
cd my-project    # has .angee-template/ in it
angee init       # uses .angee-template/, creates .angee/ here
```

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

Git templates are shallow-cloned into a temp directory, used for init, then cleaned up. When cloning a git repo, angee also checks for `.angee-template/` inside it and uses that subdirectory if found.

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

See `templates/default/` in this repo for a production template with Django, Postgres, Redis, Celery, two agents, three MCP servers, and four secrets.

## Template design guidelines

1. **Generate what you can.** Database passwords, secret keys, tokens — if it doesn't require external credentials, make it `generated: true`.

2. **Derive compound secrets.** If a service needs a connection string and you already generate the password, use `derived` to compose the full URL.

3. **Only prompt for external credentials.** API keys (Anthropic, OpenAI, GitHub), OAuth tokens, external service URLs — these are the only things users should have to provide.

4. **Include agent instructions.** An `AGENTS.md` in the workspace scaffolding tells the agent what it can do and how to behave.

5. **Use config file templates.** Different agent runtimes (opencode, Claude Code, aider) need different config formats. Template them so the MCP wiring is automatic.

6. **Keep parameters minimal.** Project name and domain are usually enough. Don't ask users to configure things they don't care about at init time — they can edit `angee.yaml` later.
