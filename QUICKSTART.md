# Angee — Quick Start

## Install

```sh
# macOS / Linux (one-liner)
curl https://angee.ai/install.sh | sh

# Homebrew
brew install angee
```

## Start in 2 commands

```sh
angee init
```
```
  ✔ Created ANGEE_ROOT at ~/.angee
  ✔ Created .gitignore
  ✔ Created angee.yaml (default template)
  ✔ Created operator.yaml (local runtime config)
  ✔ Initial git commit: "angee: initialize project"

  Next steps:
  → angee up          Start the platform
  → angee ls          View running agents and services
  → angee admin       Chat with the admin agent
```

```sh
angee up
```
```
  ✔ Compiled angee.yaml → docker-compose.yaml
  → Starting stack...
  ✔ Operator started (port 9000)
  ✔ Services deployed

  Platform ready:
  → UI        →  http://localhost:3333
  → API        →  http://localhost:8000/api
  → Operator  →  http://localhost:9000

  → angee ls          View agents and services
  → angee admin       Chat with admin agent
  → angee develop     Chat with developer agent
```

---

## What you get

```sh
angee ls
```
```
SERVICES
  NAME       STATUS          HEALTH    REPLICAS  DOMAINS
  web        ● running       healthy   1/1       localhost
  postgres   ● running       healthy   1/1
  redis      ● running       healthy   1/1

AGENTS
  NAME       STATUS          HEALTH
  admin      ● running       healthy
  developer  ● running       healthy
```

---

## Talk to your agents

```sh
angee admin           # interactive chat
angee develop         # shortcut for developer agent
angee chat my-agent   # any agent by name
```

```
  Connected to admin agent
  (type your message and press Enter — /exit to quit)

you: scale web to 3 replicas

admin: I'll scale the web service to 3 replicas.

  Current replicas: 1
  Updating angee.yaml → web.replicas: 3
  Committing: "angee-agent: scale web to 3 replicas"
  Deploying...

  ✔ web scaled to 3/3 replicas.

you: add a staging environment

admin: I'll create a staging branch in ANGEE_ROOT...
```

```sh
# One-shot (no interactive session)
angee ask "why did the last deploy fail?"
angee ask "add GitHub MCP access to the developer agent"
angee ask --agent developer "add a health check to the web service"
```

---

## Manage your deployment

```sh
angee plan          # preview what deploy would change
angee deploy        # apply angee.yaml to the runtime
angee rollback HEAD~1    # roll back to the previous deploy
angee logs web      # tail service logs
angee logs admin --follow  # live agent logs
angee status        # full platform status
```

---

## Edit angee.yaml directly

The `angee.yaml` is a regular file — you can edit it, or ask an agent to:

```yaml
# ~/.angee/angee.yaml
name: my-project

services:
  web:
    image: ghcr.io/org/myapp:latest
    lifecycle: platform
    domains:
      - host: myapp.io
    resources:
      cpu: "1.0"
      memory: "1Gi"

  postgres:
    image: pgvector/pgvector:pg17    # pgvector built-in
    lifecycle: sidecar
    volumes:
      - name: pgdata
        path: /var/lib/postgresql/data
        persistent: true

  redis:
    image: redis:7-alpine
    lifecycle: sidecar

  celery:
    build:
      context: src/base
    command: celery -A config worker -l info
    lifecycle: worker

mcp_servers:
  github:
    transport: sse
    url: https://api.githubcopilot.com/mcp/
    credentials:
      source: connect.account
      account_type: github
      run_as: requester

agents:
  admin:                              # always running, manages the platform
    image: ghcr.io/fyltr/angee-admin-agent:latest
    lifecycle: system
    role: operator
    mcp_servers: [angee-operator, angee-files]

  developer:                          # always running, helps you build
    image: ghcr.io/fyltr/angee-developer-agent:latest
    lifecycle: system
    mcp_servers: [github, angee-files]
    workspace:
      persistent: true
```

After editing: `angee deploy`

---

## Default template (angee-django)

The official Django template includes:

| Service | Image | Purpose |
|---------|-------|---------|
| `django` | your app | Web + API |
| `postgres` | `pgvector/pgvector:pg17` | Database with vector search |
| `redis` | `redis:7-alpine` | Cache + message broker |
| `celery` | your app | Async task workers |
| `celery-beat` | your app | Scheduled tasks |

```sh
angee init --template https://github.com/fyltr/angee-django-template --repo https://github.com/org/myapp
```

---

## Next: link a source repo

```sh
angee ask "link my repo at https://github.com/org/myapp as the base source"
```

Or edit `angee.yaml` directly:

```yaml
repositories:
  base:
    url: https://github.com/org/myapp
    branch: main
    role: base
```

Then `angee deploy` — your source is cloned into `~/.angee/src/base/`.

---

## Full CLI reference

```
angee init [--template url|path] [--repo url] [--dir path]
angee up
angee down
angee ls
angee status
angee plan
angee deploy [-m message]
angee rollback <sha|HEAD~N>
angee logs <service> [-f] [-n lines]
angee chat [agent]
angee admin
angee develop
angee ask <message> [--agent name]
```

---

**UI → http://localhost:3333 · Docs → https://angee.ai/docs**
