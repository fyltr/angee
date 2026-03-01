# Angee Configuration Specification

Complete reference for `angee.yaml` — the declarative source of truth for an Angee-managed stack.

## Overview

Angee is an AI-native platform orchestrator. Users define services, MCP servers, and AI agents in a single `angee.yaml`. The operator compiles it to `docker-compose.yaml` and manages the full runtime lifecycle.

```
angee.yaml  →  operator  →  docker-compose.yaml  →  docker runtime
                  ↑
            agents (via MCP)
```

---

## Top-Level Structure

```yaml
name: my-project                    # required — docker compose project name
version: "1.0"                      # optional — stack version for tracking

# P0: Secrets backend configuration
secrets_backend:
  type: openbao                     # openbao | env (default: env)
  openbao:
    address: http://openbao:8200
    auth:
      method: token                 # token | approle
      token_env: BAO_TOKEN          # env var containing the token
    prefix: angee                   # KV path prefix

# P0: Environment
environment: dev                    # dev | staging | prod (default: dev)

repositories:    { ... }            # source code repos
services:        { ... }            # platform services (containers)
mcp_servers:     { ... }            # MCP server definitions
agents:          { ... }            # AI agent definitions
skills:          { ... }            # reusable agent capabilities
secrets:         [ ... ]            # secret declarations (init-time)

# P1: Agent/MCP registry (future)
# registry:
#   source: https://github.com/fyltr/angee-registry
#   local: .angee/registry
```

---

## `name` (required)

```yaml
name: my-project
```

Used as the docker compose project name. Must be lowercase, alphanumeric + hyphens.

## `version` (optional)

```yaml
version: "1.0"
```

Stack version for tracking. Not used by the compiler today.

---

## `secrets_backend` (P0 — planned)

Configures how secrets are resolved at compile and runtime. Replaces flat `.env` file for production use.

```yaml
secrets_backend:
  type: openbao

  openbao:
    address: http://openbao:8200
    auth:
      method: token
      token_env: BAO_TOKEN          # reads token from this env var
    prefix: angee                   # path prefix in KV store
```

### Resolution

References in the form `${secret:name}` are resolved differently per backend:

| Backend | Resolution |
|---------|-----------|
| `env` (default) | `${secret:db-password}` → `${DB_PASSWORD}` (docker compose .env interpolation) |
| `openbao` | `${secret:db-password}` → operator fetches `secret/data/{prefix}/{env}/db-password` from OpenBao, injects value |

### Environment-aware paths

When `type: openbao`, the operator auto-prefixes the environment:

```
secret/data/angee/dev/db-password        # environment: dev
secret/data/angee/staging/db-password    # environment: staging
secret/data/angee/prod/db-password       # environment: prod
```

### Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `env` | Backend type: `env` or `openbao` |
| `openbao.address` | string | — | OpenBao server URL |
| `openbao.auth.method` | string | `token` | Auth method: `token` or `approle` |
| `openbao.auth.token_env` | string | `BAO_TOKEN` | Env var containing auth token |
| `openbao.auth.role_id_env` | string | — | Env var for AppRole role_id |
| `openbao.auth.secret_id_env` | string | — | Env var for AppRole secret_id |
| `openbao.prefix` | string | `angee` | KV v2 mount path prefix |

---

## `environment` (P0 — planned)

```yaml
environment: dev
```

Controls which environment overlay is applied and how secrets are scoped.

| Value | Description |
|-------|-------------|
| `dev` | Local development (default) |
| `staging` | Pre-production testing |
| `prod` | Production |

### Environment overlays

When the operator loads config, it merges:

```
angee.yaml  +  environments/{env}.yaml  →  final config
```

Overlay files deep-merge into the base config. Example:

**`environments/prod.yaml`:**
```yaml
services:
  django:
    replicas: 3
    env:
      DEBUG: "false"
agents:
  angee-developer:
    lifecycle: on-demand   # don't run developer agent in prod
```

---

## `repositories`

Source code repos cloned into `src/` during `angee init`.

```yaml
repositories:
  base:
    url: https://github.com/fyltr/angee-django
    branch: main
    role: base
    path: src/base          # optional, default: src/<name>
```

### Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | — | Git clone URL |
| `branch` | string | default branch | Branch to checkout |
| `role` | string | — | `base`, `custom`, or `dependency` |
| `path` | string | `src/<name>` | Clone destination relative to ANGEE_ROOT |

---

## `services`

Platform services — containers managed by docker compose.

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg17
    lifecycle: sidecar
    env:
      POSTGRES_USER: angee
      POSTGRES_PASSWORD: "${secret:db-password}"
    volumes:
      - name: pgdata
        path: /var/lib/postgresql/data
        persistent: true
        size: 20Gi
    health:
      path: /health
      port: 5432
      interval: 30s
      timeout: 5s
```

### Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `image` | string | — | Container image |
| `build` | object | — | Build from source (context, dockerfile, target) |
| `command` | string | — | Override container command |
| `lifecycle` | string | — | Lifecycle type (see below) |
| `domains` | list | — | Traefik routing rules (platform services only) |
| `resources` | object | — | CPU/memory limits |
| `env` | map | — | Environment variables. Supports `${secret:name}` |
| `volumes` | list | — | Named/persistent volumes |
| `ports` | list | — | Host:container port mappings |
| `raw_volumes` | list | — | Raw docker volume strings |
| `health` | object | — | HTTP health check (operator probes externally) |
| `replicas` | int | 1 | Replica count |
| `depends_on` | list | — | Service startup dependencies |

### Lifecycle values

| Lifecycle | Restart Policy | Traefik | Use Case |
|-----------|---------------|---------|----------|
| `platform` | unless-stopped | Yes (with domains) | Web-facing services |
| `sidecar` | unless-stopped | No | Databases, caches |
| `worker` | unless-stopped | No | Background processing |
| `system` | always | No | Operator, infrastructure |
| `agent` | unless-stopped | No | AI agents |
| `job` | unless-stopped | No | One-shot tasks |

### Domain spec

```yaml
domains:
  - host: myapp.io
    port: 8000          # default: 8000
    tls: true           # enables Let's Encrypt via Traefik
```

### Build spec

```yaml
build:
  context: src/base
  dockerfile: Dockerfile
  target: production    # multi-stage build target
```

### Volume spec

```yaml
volumes:
  - name: pgdata             # creates named docker volume
    path: /var/lib/postgresql/data
    persistent: true
    size: 20Gi               # informational, not enforced by docker
```

### Health spec

```yaml
health:
  path: /health
  port: 8000
  interval: 30s
  timeout: 5s
```

Health checks are **operator-managed HTTP probes** (Kubernetes-style), not Docker HEALTHCHECK directives. This avoids requiring curl/wget inside containers.

### Resource spec

```yaml
resources:
  cpu: "1.0"
  memory: "1Gi"    # Kubernetes-style: 1Gi, 256Mi, 512Ki (auto-converted to docker format)
```

---

## `mcp_servers`

MCP (Model Context Protocol) servers that agents can connect to.

```yaml
mcp_servers:
  angee-operator:
    transport: streamable-http
    url: http://operator:9000/mcp
    credentials:
      source: service_account
      scopes:
        - config.read
        - config.write
        - deploy

  angee-files:
    transport: stdio
    command:
      - node
      - /usr/local/lib/mcp-filesystem/dist/index.js
    args:
      - /workspace
    credentials:
      source: none
```

### Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `transport` | string | — | `stdio`, `sse`, or `streamable-http` |
| `url` | string | — | Server URL (for `sse` / `streamable-http`) |
| `image` | string | — | Container image (for stdio sidecar MCPs) |
| `command` | list | — | Command (for `stdio` transport) |
| `args` | list | — | Arguments appended to command |
| `credentials` | object | — | Credential sourcing config |

### Transport types

| Transport | Agent receives | Description |
|-----------|---------------|-------------|
| `stdio` | `command + args` via env | Agent runs MCP server as subprocess |
| `sse` | URL via `ANGEE_MCP_<NAME>_URL` | Server-Sent Events endpoint |
| `streamable-http` | URL via `ANGEE_MCP_<NAME>_URL` | HTTP streaming endpoint |

### Credential spec (current)

```yaml
credentials:
  source: service_account     # connect.account | service_account | sso | none
  account_type: admin         # optional
  run_as: operator            # optional
  scopes:                     # optional
    - config.read
    - deploy
```

> **Note**: The `credentials` field is declared in config but not yet enforced at runtime. It becomes functional with P1 (agent identity + JWT auth).

### Credential spec (P1 — planned)

```yaml
credentials:
  internal:
    type: jwt                 # operator-issued JWT
  external:
    type: oauth_token
    credential: secret://oauth/github/token
    client: secret://oauth/github/client
```

---

## `agents`

AI agents — first-class infrastructure entities that run as containers.

```yaml
agents:
  angee-admin:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system
    role: operator
    description: "Platform admin — manages deployment and config"

    mcp_servers:
      - angee-operator
      - angee-files

    skills:
      - deploy

    files:
      - template: opencode.json.tmpl
        mount: /root/.config/opencode/opencode.json

    workspace:
      persistent: true

    resources:
      cpu: "1.0"
      memory: "2Gi"

    env:
      ANTHROPIC_API_KEY: "${secret:anthropic-api-key}"

    system_prompt: "You are the platform admin..."
```

### Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `image` | string | `ghcr.io/fyltr/angee-agent:latest` | Container image |
| `command` | string | — | Override container command |
| `template` | string | — | Template reference (reserved for registry) |
| `version` | string | — | Image version pin |
| `lifecycle` | string | — | `system` (always-on) or `on-demand` |
| `role` | string | — | `operator` (gets API key) or `user` |
| `mcp_servers` | list | — | MCP servers this agent can access |
| `skills` | list | — | Skills from `skills:` section |
| `files` | list | — | Config files to mount into container |
| `run_as` | string | — | User identity (reserved for P1) |
| `workspace` | object | — | Workspace configuration |
| `resources` | object | — | CPU/memory limits |
| `env` | map | — | Environment variables |
| `system_prompt` | string | — | Injected as `ANGEE_SYSTEM_PROMPT` |
| `description` | string | — | Human-readable description |
| `credential_bindings` | list | — | Credential names this agent can access (resolved at compile time) |
| `permissions` | list | — | Operator-enforced capability permissions |

### Credential bindings

Agents declare which credentials they need via `credential_bindings`. The compiler resolves these at compile time by looking up credential output definitions from installed components:

```yaml
agents:
  angee-developer:
    credential_bindings:
      - github-oauth          # → GITHUB_TOKEN env var + config file mount
      - anthropic-api-key     # → ANTHROPIC_API_KEY env var
```

Each credential binding name is matched against installed credential components. The credential's `outputs` determine how the credential reaches the agent:

- **`type: env`** — Injects an environment variable (e.g., `GITHUB_TOKEN=${GITHUB_OAUTH}`)
- **`type: file`** — Renders a template file and mounts it at the specified container path

### Agent fields (P1 — planned)

```yaml
agents:
  angee-admin:
    ref: registry://agents/angee-admin@v1      # registry reference
```

| Field | Type | Description |
|-------|------|-------------|
| `ref` | string | Registry URI: `registry://agents/<name>@<version>` |

### Workspace spec

```yaml
workspace:
  path: /custom/path          # explicit host path
  repository: base            # from repositories: section
  branch: main
  persistent: true
```

Priority order for workspace resolution:
1. `path` set → bind-mount that path to `/workspace`
2. `repository` set → bind-mount `src/<repo>` to `/workspace`
3. Neither → bind-mount `./agents/<name>/workspace` to `/workspace`

### File mount spec

```yaml
files:
  - template: opencode.json.tmpl     # rendered at deploy time
    mount: /root/.config/opencode/opencode.json

  - source: ~/.ssh/config            # host file bind-mount
    mount: /root/.ssh/config
    optional: true                    # skip if source doesn't exist
```

Exactly one of `template` or `source` must be set. `mount` is always required.

### Container naming

Agent `angee-admin` becomes compose service `agent-angee-admin`.

### Injected environment variables

The compiler auto-injects these into every agent container:

| Variable | Value |
|----------|-------|
| `ANGEE_AGENT_NAME` | Agent name |
| `ANGEE_SYSTEM_PROMPT` | System prompt (if set) |
| `ANGEE_MCP_SERVERS` | Comma-separated list of MCP server names |
| `ANGEE_MCP_<NAME>_URL` | URL for each remote MCP server |
| `ANGEE_OPERATOR_API_KEY` | Operator API key (only for `role: operator`) |
| `ANGEE_SKILL_PROMPT_<NAME>` | Per-skill system prompt additions |

---

## `skills`

Reusable agent capabilities — a named bundle of MCP servers and/or system prompt.

```yaml
skills:
  deploy:
    description: "Deploy and manage the platform"
    mcp_servers:
      - angee-operator
    system_prompt: "You can deploy, rollback, and scale services."

  code-review:
    description: "Review code changes"
    mcp_servers:
      - angee-files
    system_prompt: "You can review code and suggest improvements."
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Human-readable description |
| `mcp_servers` | list | MCP servers provided by this skill |
| `system_prompt` | string | Prompt additions injected when skill is assigned |

When an agent has `skills: [deploy]`, the compiler:
1. Merges the skill's `mcp_servers` into the agent's list (dedup)
2. Injects `ANGEE_SKILL_PROMPT_DEPLOY=<prompt>` as env var

---

## `secrets`

Secret declarations — processed during `angee init` to populate `.env`.

```yaml
secrets:
  - name: db-password
    description: "Postgres password"
    required: true
    generated: true
    length: 32
    charset: "abcdefghijklmnopqrstuvwxyz0123456789"

  - name: db-url
    description: "Full DATABASE_URL"
    required: true
    derived: "postgresql://angee:${db-password}@postgres:5432/${project}"

  - name: anthropic-api-key
    description: "Anthropic API key"
    required: true
```

### Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | — | Secret name (becomes ENV_NAME via uppercasing + hyphen→underscore) |
| `description` | string | — | Human-readable description |
| `required` | bool | false | Error if missing and not generated/derived |
| `generated` | bool | false | Auto-generate random value |
| `derived` | string | — | Expression: `${other-secret}` and `${project}` substitution |
| `length` | int | 50 | Generated value length |
| `charset` | string | alphanumeric + symbols | Character set for generation |

### Resolution order

1. `--secret name=value` flag → wins unconditionally
2. `generated: true` → crypto/rand random value
3. `required: true` → interactive prompt (or error with `--yes`)
4. `derived: "..."` → interpolated from resolved secrets + project name

### Secret references in env values

```yaml
env:
  DATABASE_URL: "${secret:db-url}"
```

The compiler translates `${secret:db-url}` → `${DB_URL}` for Docker Compose interpolation from `.env`.

---

## ANGEE_ROOT Filesystem

```
.angee/                            # ANGEE_ROOT (git repo)
├── angee.yaml                     # source of truth
├── docker-compose.yaml            # generated — never edit manually
├── .env                           # secrets — gitignored
├── operator.yaml                  # local runtime config — gitignored
├── .gitignore
├── opencode.json.tmpl             # agent config template
├── .angee/
│   ├── components/                # installed component records
│   │   ├── angee-postgres.yaml
│   │   └── angee-oauth-github.yaml
│   └── credential-outputs/        # cached credential output definitions
│       └── github-oauth.yaml
├── agents/
│   ├── angee-admin/
│   │   ├── .env                   # per-agent secrets — gitignored
│   │   └── workspace/
│   │       └── AGENTS.md          # agent instructions
│   └── angee-developer/
│       ├── .env
│       └── workspace/
│           └── AGENTS.md
├── src/
│   └── base/                      # cloned repository
├── environments/                  # P0: env overlays
│   ├── dev.yaml
│   ├── staging.yaml
│   └── prod.yaml
└── registry/                      # P1: local registry
    ├── agents/
    └── mcp/
```

---

## Component Model

Components are self-contained units that can be added to a stack via `angee add <namespace>/<name>`. Each component is a git repository (or local directory) with an `angee-component.yaml` at its root.

### Component types

| Type | Description | Example |
|------|-------------|---------|
| `service` | Infrastructure service (DB, cache, vault) | `angee/postgres`, `angee/redis` |
| `agent` | AI agent with workspace and MCP access | `fyltr/health-agent` |
| `application` | Full application with services + agents | `fyltr/fyltr-django` |
| `module` | Extension to an existing application | `fyltr/django-billing` |
| `credential` | OAuth/API key credential definition | `angee/oauth-github` |

### `angee-component.yaml` format

```yaml
name: angee/postgres
type: service
version: "1.0.0"
description: "PostgreSQL with pgvector"

requires:
  - angee/openbao        # dependencies that must exist

parameters:
  - name: DBSize
    default: "20Gi"

# Stack fragments — merged into angee.yaml on install
services:
  postgres:
    image: pgvector/pgvector:pg17
    lifecycle: sidecar
    env:
      POSTGRES_USER: angee
      POSTGRES_PASSWORD: "${secret:db-password}"

secrets:
  - name: db-password
    required: true
    generated: true
```

### Credential components

Credential components define how external auth tokens reach agent containers:

```yaml
name: angee/oauth-github
type: credential
version: "1.0.0"

credential:
  name: github-oauth
  type: oauth_client
  provider:
    name: github
    auth_url: https://github.com/login/oauth/authorize
    token_url: https://github.com/login/oauth/access_token
    scopes: [repo, read:org]
  outputs:
    - type: env
      key: GITHUB_TOKEN
      value_path: access_token
    - type: file
      template: templates/github_auth.json.tmpl
      mount: /root/.config/opencode/providers/github.json
```

### Credential outputs

| Output type | Description |
|-------------|-------------|
| `env` | Injects a secret value as an environment variable |
| `file` | Renders a template and mounts it at a container path |

Outputs are resolved by the compiler when an agent declares `credential_bindings`:

```yaml
agents:
  angee-developer:
    credential_bindings:
      - github-oauth    # compiler resolves → env var + file mount
```

### Module components

Modules extend an existing application (declared via `extends`):

```yaml
name: fyltr/django-billing
type: module
extends: fyltr/fyltr-django

django:
  app: billing
  migrations: true
  urls: billing/

services:
  stripe-webhook:
    lifecycle: worker

secrets:
  - name: stripe-api-key
    required: true
```

### Installation lifecycle

```
angee add angee/postgres
  │
  ├── Fetch component (git clone or local path)
  ├── Load angee-component.yaml
  ├── Check dependencies (requires:)
  ├── Resolve parameters (--param or defaults)
  ├── Render component.yaml as Go template
  ├── Validate no naming conflicts
  ├── Merge fragments into angee.yaml
  ├── Clone repositories (if any)
  ├── Process file manifest
  └── Write install record to .angee/components/
```

Install records track what was added for clean removal via `angee remove`.

### CLI commands

```sh
angee add angee/postgres             # install from GitHub shorthand
angee add ./local/component          # install from local path
angee add angee/postgres --param DBSize=50Gi
angee remove angee/postgres          # remove component and clean up
angee components                     # list installed components
angee credential list                # list stored credentials
angee credential set db-password secret123
angee credential get db-password     # shows masked value
angee credential delete old-secret
```

---

## Operator HTTP API

All mutations go through the operator. The CLI never touches containers directly.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness check |
| `GET` | `/config` | Read angee.yaml as JSON |
| `POST` | `/config` | Write angee.yaml, validate, commit, optionally deploy |
| `POST` | `/deploy` | Compile + apply |
| `GET` | `/plan` | Dry-run diff |
| `POST` | `/rollback` | Git revert to SHA, redeploy |
| `GET` | `/status` | Runtime status of all services + agents |
| `GET` | `/logs/{service}` | Service log stream |
| `POST` | `/scale/{service}` | Scale service replicas |
| `POST` | `/down` | Bring stack down |
| `GET` | `/agents` | List agents with status |
| `POST` | `/agents/{name}/start` | Start agent |
| `POST` | `/agents/{name}/stop` | Stop agent |
| `GET` | `/agents/{name}/logs` | Agent logs |
| `GET` | `/history` | Git log of config changes |
| `POST` | `/mcp` | JSON-RPC 2.0 MCP endpoint |

### Operator API (P0 — planned additions)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/credentials` | List credential names (not values) |
| `GET` | `/credentials/{name}` | Get credential metadata |
| `POST` | `/credentials/{name}` | Set credential value |
| `DELETE` | `/credentials/{name}` | Delete credential |

### Operator API (P1 — planned additions)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/agent/token` | Issue JWT identity token for agent |
| `GET` | `/.well-known/jwks.json` | JWKS endpoint for MCP servers |
| `GET` | `/registry/agents` | List registered agents |
| `GET` | `/registry/agents/{name}` | Get agent registry entry |

---

## Compile Pipeline

```
angee.yaml
    │
    ├── [P0] Load environment overlay (environments/{env}.yaml)
    ├── [P0] Resolve secrets via backend (env or openbao)
    │
    ├── Validate config (lifecycles, MCP refs, skill refs, file mounts)
    │
    ├── [P1] Resolve registry refs (registry://agents/... → image + defaults)
    │
    ├── RenderAgentFiles (deploy-time .tmpl rendering)
    │
    ├── Compile services → compose services
    │   ├── lifecycle → restart policy
    │   ├── domains → Traefik labels
    │   ├── ${secret:name} → ${ENV_NAME}
    │   └── volumes, ports, resources, health, depends_on
    │
    ├── Compile agents → compose services (prefixed agent-)
    │   ├── image, command, stdin_open, tty
    │   ├── workspace volume mount
    │   ├── per-agent .env file
    │   ├── MCP server env injection
    │   ├── skill resolution (merge MCP servers + prompts)
    │   ├── credential_bindings → resolve credential outputs (env + file mounts)
    │   └── file mount volumes
    │
    └── Write docker-compose.yaml
```

---

## Full Example

```yaml
name: my-project
version: "1.0"

secrets_backend:
  type: openbao
  openbao:
    address: http://openbao:8200
    auth:
      method: token
      token_env: BAO_TOKEN
    prefix: angee

environment: dev

services:
  operator:
    image: ghcr.io/fyltr/angee-operator:latest
    lifecycle: system
    ports:
      - "9000:9000"
    raw_volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - .:/angee-root
    env:
      ANGEE_ROOT: /angee-root
    health:
      path: /health
      port: 9000

  postgres:
    image: pgvector/pgvector:pg17
    lifecycle: sidecar
    env:
      POSTGRES_USER: angee
      POSTGRES_PASSWORD: "${secret:db-password}"
      POSTGRES_DB: "my-project"
    volumes:
      - name: pgdata
        path: /var/lib/postgresql/data
        persistent: true

  redis:
    image: redis:7-alpine
    lifecycle: sidecar
    command: redis-server --appendonly yes

  openbao:
    image: openbao/openbao:2
    lifecycle: system
    ports:
      - "8200:8200"
    env:
      BAO_LOCAL_CONFIG: '{"storage":{"raft":{"path":"/openbao/data","node_id":"node1"}},"listener":{"tcp":{"address":"0.0.0.0:8200","tls_disable":true}},"api_addr":"http://openbao:8200","cluster_addr":"http://openbao:8201"}'
    command: server -dev -dev-root-token-id=dev-root-token
    volumes:
      - name: bao-data
        path: /openbao/data
        persistent: true

mcp_servers:
  angee-operator:
    transport: streamable-http
    url: http://operator:9000/mcp
  angee-files:
    transport: stdio
    command: [node, /usr/local/lib/mcp-filesystem/dist/index.js]
    args: [/workspace]

agents:
  angee-admin:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system
    role: operator
    mcp_servers: [angee-operator, angee-files]
    files:
      - template: opencode.json.tmpl
        mount: /root/.config/opencode/opencode.json
    workspace:
      persistent: true
    env:
      ANTHROPIC_API_KEY: "${secret:anthropic-api-key}"

secrets:
  - name: db-password
    required: true
    generated: true
    length: 32
  - name: anthropic-api-key
    required: true
```
