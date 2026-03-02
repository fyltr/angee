# Example: Fyltr Unibox on Angee

This walkthrough shows how a real application — **Fyltr Unibox** (a unified messaging platform with contact intelligence) — is deployed, managed, and extended using angee. It demonstrates the full lifecycle: from `angee init` to self-managing AI agents that use the application's own data.

---

## What Fyltr Unibox Does

Fyltr is a Django application with two core modules:

**Unibox** — aggregates messages from every communication channel (email via IMAP, WhatsApp via live Neonize bridge, SMS, Telegram, Slack, Discord, Twitter/X) into a single database with unified threading, identity mapping, fragment deduplication, and pgvector semantic search.

**Nexus** — the contact graph. Maps identities across platforms to unified contacts, tracks relationship strength (gravity scores based on message frequency, recency, and platform breadth), and organizes contacts into hierarchical groups with topic-based conversation tracking.

Together: Unibox knows what was said, Nexus knows who said it and how they relate to each other.

---

## The Stack

```yaml
# angee.yaml — fyltr-unibox

name: fyltr-unibox

# ─── Connectors ───────────────────────────────────────────────────────────────
# External services shared across agents and the Django app.
# Agents and services declare which connectors they need.
# Angee injects credentials at deploy time.

connectors:
  google:
    provider: google
    type: oauth
    oauth:
      client_id: "${secret:google-client-id}"
      client_secret: "${secret:google-client-secret}"
      scopes:
        - https://mail.google.com/                    # IMAP access
        - https://www.googleapis.com/auth/gmail.send  # SMTP send
        - https://www.googleapis.com/auth/contacts.readonly
    env:
      GOOGLE_ACCESS_TOKEN: oauth_token
    description: "Google account (Gmail, Contacts)"
    required: true

  whatsapp:
    provider: custom
    type: setup_command
    setup_command:
      command: [whatsapp-bridge, auth]
      prompt: "Scan the QR code with WhatsApp on your phone"
      parse: stdout
    env:
      WA_SESSION_TOKEN: session_token
    description: "WhatsApp (live bridge via Neonize)"

  github:
    provider: github
    type: oauth
    oauth:
      client_id: "${secret:github-client-id}"
      client_secret: "${secret:github-client-secret}"
      scopes: [repo, read:org]
    env:
      GH_TOKEN: oauth_token
    description: "GitHub for code access"

  anthropic:
    provider: anthropic
    type: api_key
    env:
      ANTHROPIC_API_KEY: api_key
    description: "Anthropic API key for AI agents"
    required: true

# ─── Services ─────────────────────────────────────────────────────────────────

services:
  django:
    build:
      context: src/fyltr-django
      dockerfile: Dockerfile
    lifecycle: platform
    domains:
      - host: app.fyltr.local
        port: 8000
    connectors: [google, whatsapp]
    env:
      DATABASE_URL: "${secret:db-url}"
      REDIS_URL: "redis://redis:6379/0"
      SECRET_KEY: "${secret:django-secret-key}"
    health:
      path: /health/
      port: 8000
      interval: 15s
    depends_on: [postgres, redis]

  celery-worker:
    build:
      context: src/fyltr-django
      dockerfile: Dockerfile
    command: celery -A config worker -l info
    lifecycle: worker
    connectors: [google]
    env:
      DATABASE_URL: "${secret:db-url}"
      REDIS_URL: "redis://redis:6379/0"
    depends_on: [postgres, redis]

  celery-beat:
    build:
      context: src/fyltr-django
      dockerfile: Dockerfile
    command: celery -A config beat -l info
    lifecycle: worker
    env:
      DATABASE_URL: "${secret:db-url}"
      REDIS_URL: "redis://redis:6379/0"
    depends_on: [postgres, redis]

  celery-whatsapp:
    build:
      context: src/fyltr-django
      dockerfile: Dockerfile
    command: celery -A config worker -l info -Q whatsapp -c 4
    lifecycle: worker
    connectors: [whatsapp]
    env:
      DATABASE_URL: "${secret:db-url}"
      REDIS_URL: "redis://redis:6379/0"
    depends_on: [postgres, redis]

  postgres:
    image: pgvector/pgvector:pg17
    lifecycle: sidecar
    env:
      POSTGRES_PASSWORD: "${secret:db-password}"
      POSTGRES_DB: fyltr
    volumes:
      - name: pgdata
        path: /var/lib/postgresql/data
        persistent: true

  redis:
    image: redis:7-alpine
    lifecycle: sidecar
    volumes:
      - name: redis-data
        path: /data
        persistent: true

# ─── MCP Servers ──────────────────────────────────────────────────────────────
# Tool providers that agents connect to.

mcp_servers:
  angee-operator:
    transport: streamable-http
    url: http://operator:9000/mcp
    credentials:
      source: service_account
      scopes: [config.read, config.write, deploy, rollback, status, logs, scale, secrets.list, secrets.write]

  angee-files:
    transport: stdio
    image: ghcr.io/fyltr/angee-filesystem-mcp:latest
    command: [node, /usr/local/lib/mcp-filesystem/dist/index.js]
    args: [/workspace]

  fyltr-mcp:
    transport: streamable-http
    url: http://django:8000/mcp/
    credentials:
      source: service_account

# ─── Skills ───────────────────────────────────────────────────────────────────

skills:
  platform-ops:
    description: "Deploy, rollback, scale, and manage the angee platform"
    mcp_servers: [angee-operator]

  code-editing:
    description: "Read and edit source code in the workspace"
    mcp_servers: [angee-files]

  fyltr-data:
    description: "Query unibox messages, nexus contacts, and communication history"
    mcp_servers: [fyltr-mcp]

# ─── Agents ───────────────────────────────────────────────────────────────────

agents:
  angee-admin:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system
    role: operator
    description: "Platform admin — deploys, scales, manages infrastructure and agents"
    skills: [platform-ops]
    mcp_servers: [angee-operator]
    connectors: [anthropic]
    files:
      - template: opencode.json.tmpl
        mount: /root/.config/opencode/opencode.json
    workspace:
      persistent: true

  angee-developer:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system
    role: user
    description: "Developer — writes code, runs tests, improves the fyltr-django app"
    skills: [code-editing, platform-ops]
    mcp_servers: [angee-files, angee-operator]
    connectors: [anthropic, github]
    workspace:
      repository: fyltr-django
      persistent: true

  angee-researcher:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system
    role: user
    description: "Research assistant — queries communication history and contact network to prepare briefings"
    skills: [fyltr-data]
    mcp_servers: [fyltr-mcp]
    connectors: [anthropic]
    workspace:
      persistent: true

# ─── Repositories ─────────────────────────────────────────────────────────────

repositories:
  fyltr-django:
    url: https://github.com/fyltr/fyltr-django
    branch: main
    path: src/fyltr-django

# ─── Secrets ──────────────────────────────────────────────────────────────────

secrets:
  - name: django-secret-key
    generated: true
    length: 50

  - name: db-password
    generated: true
    length: 32

  - name: db-url
    derived: "postgresql://postgres:${db-password}@postgres:5432/fyltr"

  - name: google-client-id
    required: true
    description: "Google OAuth client ID (console.cloud.google.com)"

  - name: google-client-secret
    required: true

  - name: github-client-id
    required: true

  - name: github-client-secret
    required: true

secrets_backend:
  type: openbao
  openbao:
    address: http://openbao:8200
    auth:
      method: token
      token_env: BAO_TOKEN
    prefix: angee
```

---

## How Everything Fits Together

### Step 1: Initialize the stack

```bash
angee init --repo https://github.com/fyltr/fyltr-django
```

Angee scaffolds ANGEE_ROOT, generates `db-password` and `django-secret-key`, prompts for `google-client-id`, `google-client-secret`, `github-client-id`, `github-client-secret`, and writes everything to `.env` (and seeds OpenBao if running). The fyltr-django repo is cloned into `src/fyltr-django/`.

### Step 2: Connect external accounts

```bash
angee connect google       # opens browser → Google OAuth → grants Gmail + Contacts
angee connect whatsapp     # runs whatsapp-bridge auth → scan QR code
angee connect github       # opens browser → GitHub OAuth
angee connect anthropic    # prompts for API key
```

Each connector stores its credential in OpenBao (or `.env`). These are stack-level shared resources. When the stack deploys, the compiler looks at which services and agents declared `connectors: [google, whatsapp]` and injects the right env vars.

The Django app receives `GOOGLE_ACCESS_TOKEN` and `WA_SESSION_TOKEN` because it declared `connectors: [google, whatsapp]`. The celery-whatsapp worker gets `WA_SESSION_TOKEN`. The developer agent gets `GH_TOKEN` and `ANTHROPIC_API_KEY`. The researcher agent gets `ANTHROPIC_API_KEY`. Nobody gets credentials they didn't ask for.

### Step 3: Deploy

```bash
angee up
```

The compiler reads `angee.yaml`, resolves all secret references and connector credentials, generates `docker-compose.yaml` with Traefik labels, and brings everything up:

```
Infrastructure:
  operator      — REST API + MCP server (port 9000)
  traefik       — reverse proxy (ports 80/443)
  openbao       — secrets vault (port 8200)

Application:
  django        — web app at app.fyltr.local (port 8000)
  celery-worker — background tasks (IMAP sync, topic extraction)
  celery-beat   — scheduled jobs (periodic email polling)
  celery-whatsapp — dedicated WhatsApp worker (live Neonize bridge)
  postgres      — pgvector database
  redis         — cache + message broker

Agents:
  agent-angee-admin      — platform ops (port 4096)
  agent-angee-developer  — code editing (port 4096)
  agent-angee-researcher — data queries (port 4096)
```

Once running, the Django app connects to Gmail via IMAP (using the Google OAuth token from the `google` connector), starts the WhatsApp live bridge (using the session from the `whatsapp` connector), and begins syncing messages into the unified database. Celery beat schedules periodic IMAP polls. The WhatsApp worker maintains a persistent connection.

### Step 4: Use angee-developer to improve the app

```bash
angee develop
```

This attaches to the developer agent's terminal. The developer agent has:

- **angee-files MCP** — can read and edit all source code in `src/fyltr-django/`
- **angee-operator MCP** — can deploy changes, check logs, restart services
- **github connector** — can push branches, create PRs
- **anthropic connector** — powers the AI model

A typical interaction:

```
You: Add a REST endpoint to search messages by semantic similarity using pgvector.

Developer agent:
  → reads src/fyltr-django/src/fyltr/unibox/drf/views.py (via angee-files)
  → reads src/fyltr-django/src/fyltr/unibox/models.py (MessageEmbedding model)
  → writes new SearchViewSet with cosine similarity query
  → writes serializer and registers URL
  → calls deploy tool (via angee-operator) to rebuild django container
  → checks logs to verify no errors
  → reports: "Added GET /api/unibox/messages/search/?q=... endpoint"
```

The developer agent works in a persistent workspace with the full repo checked out. It can iterate: write code, deploy, check logs, fix issues, deploy again. When satisfied, it can push a branch and create a PR using the GitHub connector.

### Step 5: Manage connectors via the angee UI (or admin agent)

The angee-admin agent can manage the entire platform including connectors:

```bash
angee admin
```

```
You: Add a Telegram connector so we can sync Telegram messages too.

Admin agent:
  → calls config_get (via angee-operator MCP) to read current angee.yaml
  → adds a new connector entry for Telegram with type: token
  → adds connectors: [telegram] to the django service and celery-worker
  → calls config_set with the updated YAML
  → calls deploy to apply the change
  → reports: "Added telegram connector. Run 'angee connect telegram' to set the bot token."
```

The admin agent can also:

- Scale workers: `"Scale celery-whatsapp to 8 workers"` → calls `service_scale`
- Check health: `"Is everything running?"` → calls `platform_status`
- Rollback: `"The last deploy broke email sync, roll back"` → calls `deploy_rollback`
- Add agents: `"Spin up a new research agent focused on sales contacts"` → edits angee.yaml and deploys
- Rotate credentials: `"Refresh the Google OAuth token"` → manages credentials via MCP

### Step 6: Use angee-researcher for intelligence

This is where connectors and MCP meet. The researcher agent has access to `fyltr-mcp` — the Django app's own MCP server that exposes tools for querying the database.

```bash
angee chat angee-researcher
```

```
You: I have a meeting with Sarah Chen tomorrow. Prepare a briefing.

Researcher agent:
  → calls contact_search("Sarah Chen") via fyltr-mcp
  → gets back: Contact(name="Sarah Chen", identities=[email:sarah@acme.com, whatsapp:+1...])
  → calls message_search(contact_id="con_abc123", limit=50) via fyltr-mcp
  → gets back: last 50 messages across email + WhatsApp threads
  → calls relationship_get(contact_id="con_abc123") via fyltr-mcp
  → gets back: gravity=0.82, platforms=[email, whatsapp], last_interaction=2d ago
  → calls contact_graph(contact_id="con_abc123", depth=1) via fyltr-mcp
  → gets back: mutual contacts (her team, shared threads)
  → synthesizes a briefing:

  "Sarah Chen — VP Engineering at Acme Corp
   Relationship: Strong (gravity 0.82), communicate via email and WhatsApp.
   Last contact: 2 days ago about the API integration timeline.

   Key topics from recent conversations:
   - API v2 migration (she raised concerns about backwards compat, Mar 1)
   - Joint webinar planning (confirmed for March 15, needs slide deck)
   - Hiring: she's looking for a senior Go developer (you offered to share the posting)

   Open threads:
   - WhatsApp: awaiting her response on the slide deck outline
   - Email: invoice #1847 sent Feb 28, not yet acknowledged

   People in her orbit you also know:
   - James Park (her CTO, you met at the conference)
   - Maria Lopez (shared Slack channel #api-partners)"
```

The researcher doesn't have direct database access or raw SQL. It queries through the fyltr-mcp server, which exposes high-level tools (search contacts, get messages, query relationships). The MCP server handles authorization, pagination, and data formatting.

### Step 7: Admin creates a new agent dynamically

The admin agent can create new agents on the fly by editing `angee.yaml`:

```
You: Create an agent called "angee-outreach" that can draft follow-up emails
     using the contact history and send them via Gmail.

Admin agent:
  → reads current angee.yaml via config_get
  → adds a new agent:
      angee-outreach:
        image: ghcr.io/anomalyco/opencode:latest
        lifecycle: system
        role: user
        description: "Drafts and sends follow-up emails using communication history"
        skills: [fyltr-data]
        mcp_servers: [fyltr-mcp]
        connectors: [anthropic, google]
        workspace:
          persistent: true
  → calls config_set with updated YAML + deploy: true
  → new agent container starts with:
      - fyltr-mcp access (query contacts, messages, relationships)
      - google connector (GOOGLE_ACCESS_TOKEN for sending email)
      - anthropic connector (ANTHROPIC_API_KEY for LLM)
  → reports: "angee-outreach is running. Use 'angee chat angee-outreach' to interact."
```

The new agent can now:
1. Query Nexus for contacts with stale relationships (`is_fading: true`)
2. Read recent message history for context
3. Draft personalized follow-up emails
4. Send via Gmail using the shared Google connector

All without any code changes to the Django app — just a YAML edit and deploy.

---

## How Connectors Flow Through the System

```
angee connect google
  │
  ├─ OAuth flow via operator (/connectors/google/start → Google → /connectors/callback)
  ├─ Token stored in OpenBao as connector-google
  │
  ▼
angee up / angee deploy
  │
  ├─ Compiler reads angee.yaml
  ├─ For each service/agent with connectors: [google]:
  │   ├─ Looks up connector "google" spec
  │   ├─ Reads env mapping: GOOGLE_ACCESS_TOKEN: oauth_token
  │   ├─ Injects: GOOGLE_ACCESS_TOKEN=${CONNECTOR_GOOGLE} into docker-compose env
  │   └─ .env has: CONNECTOR_GOOGLE=ya29.a0AfH6SM...
  │
  ▼
Runtime
  │
  ├─ django container: has GOOGLE_ACCESS_TOKEN → uses for IMAP + SMTP
  ├─ celery-worker: has GOOGLE_ACCESS_TOKEN → uses for email sync
  ├─ celery-whatsapp: has WA_SESSION_TOKEN → uses for Neonize bridge
  ├─ agent-angee-developer: has GH_TOKEN → uses for GitHub ops
  ├─ agent-angee-researcher: has ANTHROPIC_API_KEY → uses for LLM
  └─ agent-angee-outreach: has GOOGLE_ACCESS_TOKEN + ANTHROPIC_API_KEY
```

Key principle: **connectors are declared once, shared everywhere, and injected automatically**. No service or agent manually configures credentials. The angee compiler handles all the wiring.

---

## Summary

| Layer | What | How |
|-------|------|-----|
| **Infrastructure** | operator + traefik + openbao | Always-on, managed by angee |
| **Application** | django + celery workers + postgres + redis | Built from source, deployed via angee |
| **Connectors** | google, whatsapp, github, anthropic | OAuth/API keys stored in vault, shared across stack |
| **Data** | Unibox (messages) + Nexus (contacts) | Django app manages, exposes via REST + MCP |
| **Admin agent** | Platform management | Edits angee.yaml, deploys, scales, creates agents |
| **Developer agent** | Code improvements | Edits source code, deploys, tests, pushes to GitHub |
| **Researcher agent** | Intelligence | Queries messages and contacts via fyltr-mcp |
| **Outreach agent** | Automated actions | Reads history + sends emails via shared connectors |

Everything is one file (`angee.yaml`), one command (`angee up`), and one git repo. Agents can modify the stack, the stack self-deploys, and connectors flow credentials to wherever they're needed.
