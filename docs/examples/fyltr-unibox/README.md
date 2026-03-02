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

## Dynamic Connectors: Adding Email Accounts at Runtime

The example above shows static connectors — declared once in `angee.yaml` at init time. But Unibox is a multi-account messaging platform. Users add and remove email accounts, WhatsApp numbers, and other sources through the Django UI at runtime. A redeploy for every new Gmail account is wrong.

### The Problem

A user has three Gmail accounts (personal, work, client) and two IMAP accounts (company server, shared inbox). They add these through the Unibox UI over days and weeks. Each needs OAuth tokens or passwords stored securely, and all must survive a full stack rebuild from scratch.

### The Solution: Two Tiers of Connectors

**Tier 1 — Platform connectors** (static, declared in angee.yaml at init):

These are infrastructure-level accounts that the stack itself needs to function. They're injected as env vars at deploy time. Examples: the Anthropic API key for agents, the GitHub OAuth for the developer agent, the initial Google account.

**Tier 2 — Application connectors** (dynamic, managed at runtime via operator API):

These are accounts that the application manages on behalf of users. They're declared in `angee.yaml` for gitops (so the stack can be rebuilt), but the app reads credentials from OpenBao directly at runtime — not from env vars. No redeploy needed.

### How It Works

```yaml
# angee.yaml — connectors section with multiple email accounts

connectors:
  # Tier 1: platform connector (static, injected as env var)
  anthropic:
    provider: anthropic
    type: api_key
    env:
      ANTHROPIC_API_KEY: api_key
    required: true

  # Tier 2: application connectors (dynamic, read from OpenBao at runtime)
  gmail-personal:
    provider: google
    type: oauth
    oauth:
      client_id: "${secret:google-client-id}"
      client_secret: "${secret:google-client-secret}"
      scopes: [https://mail.google.com/, https://www.googleapis.com/auth/gmail.send]
    description: "Personal Gmail"
    tags: [email, imap]

  gmail-work:
    provider: google
    type: oauth
    oauth:
      client_id: "${secret:google-client-id}"
      client_secret: "${secret:google-client-secret}"
      scopes: [https://mail.google.com/, https://www.googleapis.com/auth/gmail.send]
    description: "Work Gmail"
    tags: [email, imap]

  company-imap:
    provider: custom
    type: api_key
    description: "Company IMAP server (mail.company.com)"
    tags: [email, imap]

  shared-inbox:
    provider: custom
    type: api_key
    description: "Shared support inbox (support@company.com)"
    tags: [email, imap]

  whatsapp-personal:
    provider: custom
    type: setup_command
    setup_command:
      command: [whatsapp-bridge, auth]
      prompt: "Scan QR code with your personal WhatsApp"
      parse: stdout
    tags: [messaging, whatsapp]
```

#### The flow when a user adds a new email account in the Django UI:

```
User clicks "Add Email Account" in Unibox settings
  │
  ├─ For OAuth (Gmail):
  │   │
  │   ├─ Django redirects to operator: GET /connectors/gmail-client3/start
  │   ├─ Operator redirects to Google OAuth consent screen
  │   ├─ Google calls back → operator exchanges code for token
  │   ├─ Token stored in OpenBao as: connector-gmail-client3
  │   │
  │   └─ Operator updates angee.yaml (via config_set):
  │       connectors:
  │         gmail-client3:
  │           provider: google
  │           type: oauth
  │           ...
  │       → git commit "add connector: gmail-client3"
  │
  ├─ For IMAP (custom server):
  │   │
  │   ├─ Django collects: host, port, username, password via form
  │   ├─ Django calls operator: POST /credentials (stores password in OpenBao)
  │   ├─ Django calls operator: POST /config (adds connector to angee.yaml)
  │   │   connectors:
  │   │     imap-support:
  │   │       provider: custom
  │   │       type: api_key
  │   │       description: "support@company.com on mail.company.com:993"
  │   │       tags: [email, imap]
  │   │       metadata:
  │   │         host: mail.company.com
  │   │         port: 993
  │   │         username: support@company.com
  │   │         ssl: true
  │   └─ → git commit "add connector: imap-support"
  │
  ▼
Django reads credentials at runtime (NO redeploy needed)
  │
  ├─ On startup / periodically:
  │   ├─ GET /connectors?tags=email → list all email connectors
  │   ├─ For each connector:
  │   │   ├─ GET /connectors/{name}/status → is it connected?
  │   │   ├─ GET /credentials/{name} → get the credential from OpenBao
  │   │   └─ Start IMAP sync / configure SMTP sender
  │   └─ Django's connector registry now has all email accounts
  │
  ▼
angee.yaml is always the source of truth
  │
  ├─ If the stack is rebuilt from scratch:
  │   ├─ angee.yaml declares all connectors (from git)
  │   ├─ OpenBao has all credentials (persistent volume)
  │   ├─ Django reads connectors + credentials on startup
  │   └─ All email accounts are restored automatically
  │
  └─ If OpenBao is lost:
      ├─ angee.yaml still declares the connectors
      ├─ angee connect gmail-personal → re-authenticate
      ├─ angee connect gmail-work → re-authenticate
      └─ Credentials restored, Django picks them up
```

#### What Django sees at runtime:

Django doesn't get individual env vars per email account. Instead it gets:

```
ANGEE_OPERATOR_URL=http://operator:9000    # injected at deploy time
ANGEE_API_KEY=sk-...                       # injected at deploy time
```

And uses the operator API to discover and read connectors:

```python
# Django Unibox connector discovery (pseudocode)

import httpx

OPERATOR = os.environ["ANGEE_OPERATOR_URL"]
API_KEY = os.environ["ANGEE_API_KEY"]

def get_email_connectors():
    """Discover all email connectors from angee."""
    resp = httpx.get(
        f"{OPERATOR}/connectors",
        params={"tags": "email"},
        headers={"Authorization": f"Bearer {API_KEY}"},
    )
    return resp.json()  # [{name, provider, type, description, metadata, connected}]

def get_credential(connector_name):
    """Read a connector's credential from OpenBao via operator."""
    resp = httpx.get(
        f"{OPERATOR}/credentials/connector-{connector_name}",
        headers={"Authorization": f"Bearer {API_KEY}"},
    )
    return resp.json()["value"]

def add_email_account(name, provider, host, port, username, password):
    """Register a new email connector from the Django UI."""
    # 1. Store credential
    httpx.post(
        f"{OPERATOR}/credentials/connector-{name}",
        json={"value": password},
        headers={"Authorization": f"Bearer {API_KEY}"},
    )
    # 2. Add connector to angee.yaml
    config = httpx.get(f"{OPERATOR}/config", headers=...).json()
    config["connectors"][name] = {
        "provider": provider,
        "type": "api_key",
        "description": f"{username} on {host}:{port}",
        "tags": ["email", "imap"],
        "metadata": {"host": host, "port": port, "username": username, "ssl": True},
    }
    httpx.post(
        f"{OPERATOR}/config",
        json={"content": yaml.dump(config), "message": f"add connector: {name}"},
        headers={"Authorization": f"Bearer {API_KEY}"},
    )
    # 3. No redeploy needed — connector is immediately available
```

#### What the operator API provides:

| Endpoint | Purpose |
|----------|---------|
| `GET /connectors` | List all connectors (filterable by `tags`) |
| `GET /connectors?tags=email` | List only email connectors |
| `GET /connectors/{name}/status` | Is this connector authenticated? |
| `GET /credentials/connector-{name}` | Read credential from OpenBao |
| `POST /credentials/connector-{name}` | Store credential in OpenBao |
| `POST /config` | Update angee.yaml (adds connector declaration) |

#### Connector metadata for IMAP/SMTP:

The `metadata` field on a connector holds connection details that aren't secrets:

```yaml
connectors:
  imap-support:
    provider: custom
    type: api_key
    tags: [email, imap]
    metadata:
      host: mail.company.com
      port: 993
      username: support@company.com
      ssl: true
      smtp_host: smtp.company.com
      smtp_port: 587
```

The credential (password/token) is in OpenBao. The connection details (host, port, username) are in angee.yaml. Both are needed; neither duplicates the other.

### The Two-Tier Pattern

```
┌─────────────────────────────────────────────────────────┐
│                    angee.yaml (git)                      │
│                                                         │
│  Tier 1: Platform connectors         Tier 2: App connectors
│  ┌──────────────┐                    ┌──────────────────┐
│  │ anthropic     │ ← env var inject  │ gmail-personal   │
│  │ github        │   at deploy time  │ gmail-work       │
│  └──────────────┘                    │ company-imap     │
│                                      │ shared-inbox     │
│                                      │ whatsapp-personal│
│                                      └──────────────────┘
│                                        ↑ read at runtime
│                                        │ via operator API
└─────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│                   OpenBao (vault)                        │
│                                                         │
│  connector-anthropic = sk-ant-...                       │
│  connector-github = gho_...                             │
│  connector-gmail-personal = ya29.a0AfH6SM...            │
│  connector-gmail-work = ya29.b1BgTN...                  │
│  connector-company-imap = p@ssw0rd                      │
│  connector-shared-inbox = inbox-secret                  │
│  connector-whatsapp-personal = wa-session-xyz           │
└─────────────────────────────────────────────────────────┘
```

**Tier 1** (agents): Credentials injected as env vars at deploy time. Simple. Agent containers get `ANTHROPIC_API_KEY=sk-ant-...` in their environment.

**Tier 2** (app): Django queries the operator API at runtime to discover connectors and read credentials from OpenBao. No redeploy when accounts are added or removed. New accounts appear instantly.

**Both tiers** are declared in angee.yaml and versioned in git. If the stack is rebuilt from scratch, everything is restored: the connector declarations from git, the credentials from OpenBao.

### What happens when a user removes an account?

```
User clicks "Remove" on gmail-work in Unibox settings
  │
  ├─ Django stops IMAP sync for that account
  ├─ Django calls DELETE /credentials/connector-gmail-work
  ├─ Django calls POST /config → removes gmail-work from connectors:
  └─ → git commit "remove connector: gmail-work"
```

Clean. The credential is gone from OpenBao, the connector is gone from angee.yaml, and the git history records when and why it was removed.

---

## How Connectors Flow Through the System (Updated)

```
Static flow (deploy time):
  angee connect anthropic → stored in OpenBao
  angee deploy → compiler injects ANTHROPIC_API_KEY into agent containers

Dynamic flow (runtime):
  User adds Gmail account in Django UI
    → Django calls operator API: store credential + update angee.yaml
    → Django reads credential from OpenBao via operator API
    → IMAP sync starts immediately (no redeploy)
    → angee.yaml committed to git (rebuild-safe)
```

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
