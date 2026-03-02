# Example: Odoo 18 on Angee

This walkthrough shows how **Odoo 18** — a full-featured ERP — is deployed on angee with email connectors, the [odoo-llm](https://github.com/apexive/odoo-llm) MCP server, and three AI agents: an **admin** for platform ops, a **webmaster** for website management, and a **bookkeeper** for accounting.

---

## What This Stack Does

Odoo is an ERP that covers CRM, accounting, inventory, website, email marketing, and more. By deploying it on angee we get:

1. **GitOps deployment** — the entire Odoo stack defined in one `angee.yaml`, versioned in git
2. **Email connectors** — Google and custom SMTP/IMAP accounts shared across Odoo and agents
3. **AI agents that manage Odoo** — via the odoo-llm MCP server, agents can read/write any Odoo model, manage accounting, edit the website, and call any Odoo method
4. **Self-managing infrastructure** — the admin agent can scale workers, deploy config changes, and manage secrets

---

## The Stack

```yaml
# angee.yaml — odoo-erp

name: odoo-erp

# ─── Connectors ───────────────────────────────────────────────────────────────
# Email accounts shared between Odoo (for sending/receiving) and agents.

connectors:
  google:
    provider: google
    type: oauth
    oauth:
      client_id: "${secret:google-client-id}"
      client_secret: "${secret:google-client-secret}"
      scopes:
        - https://mail.google.com/
        - https://www.googleapis.com/auth/gmail.send
        - https://www.googleapis.com/auth/contacts.readonly
    env:
      GOOGLE_ACCESS_TOKEN: oauth_token
    description: "Google account (Gmail for Odoo outbound/inbound email)"
    required: true

  smtp:
    provider: custom
    type: api_key
    env:
      SMTP_PASSWORD: api_key
    description: "SMTP relay password (Mailgun, SendGrid, or direct)"

  anthropic:
    provider: anthropic
    type: api_key
    env:
      ANTHROPIC_API_KEY: api_key
    description: "Anthropic API key for AI agents"
    required: true

  github:
    provider: github
    type: oauth
    oauth:
      client_id: "${secret:github-client-id}"
      client_secret: "${secret:github-client-secret}"
      scopes: [repo]
    env:
      GH_TOKEN: oauth_token
    description: "GitHub for custom addon development"

# ─── Services ─────────────────────────────────────────────────────────────────

services:
  odoo:
    image: odoo:18.0
    lifecycle: platform
    domains:
      - host: erp.company.local
        port: 8069
        tls: true
    env:
      HOST: postgres
      PORT: "5432"
      USER: odoo
      PASSWORD: "${secret:db-password}"
    volumes:
      - name: odoo-data
        path: /var/lib/odoo
        persistent: true
      - name: odoo-addons
        path: /mnt/extra-addons
        persistent: true
    raw_volumes:
      - ./config/odoo.conf:/etc/odoo/odoo.conf:ro
    health:
      path: /web/health
      port: 8069
      interval: 30s
      timeout: 10s
    depends_on: [postgres, redis]

  postgres:
    image: postgres:15
    lifecycle: sidecar
    env:
      POSTGRES_USER: odoo
      POSTGRES_PASSWORD: "${secret:db-password}"
      POSTGRES_DB: postgres
      PGDATA: /var/lib/postgresql/data/pgdata
    volumes:
      - name: pgdata
        path: /var/lib/postgresql/data/pgdata
        persistent: true

  redis:
    image: redis:7-alpine
    lifecycle: sidecar
    volumes:
      - name: redis-data
        path: /data
        persistent: true

# ─── MCP Servers ──────────────────────────────────────────────────────────────

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
        - rollback
        - status
        - logs
        - scale
        - secrets.list
        - secrets.write

  angee-files:
    transport: stdio
    image: ghcr.io/fyltr/angee-filesystem-mcp:latest
    command: [node, /usr/local/lib/mcp-filesystem/dist/index.js]
    args: [/workspace]

  odoo-llm:
    transport: streamable-http
    url: http://odoo:8069/mcp
    credentials:
      source: service_account
      scopes: [all]

# ─── Skills ───────────────────────────────────────────────────────────────────

skills:
  platform-ops:
    description: "Deploy, rollback, scale, and manage the angee platform"
    mcp_servers: [angee-operator]

  odoo-admin:
    description: "Full Odoo access — read/write any model, call any method, manage configuration"
    mcp_servers: [odoo-llm]

  odoo-accounting:
    description: "Accounting operations — journal entries, invoices, payments, reports, reconciliation, period close"
    mcp_servers: [odoo-llm]
    system_prompt: |
      You are a bookkeeper. Use the Odoo accounting tools to manage
      journal entries, invoices, bills, payments, and reports.
      Always verify entries before posting. Never delete posted entries
      — reverse them instead. Follow double-entry accounting principles.

  odoo-website:
    description: "Website management — pages, menus, media, SEO, redirects, analytics"
    mcp_servers: [odoo-llm]
    system_prompt: |
      You are a webmaster managing an Odoo website. Use the website
      tools to create and edit pages, manage navigation menus, upload
      media, optimize SEO metadata, and set up URL redirects.
      Always preview changes before publishing.

  code-editing:
    description: "Read and edit source code and config files"
    mcp_servers: [angee-files]

# ─── Agents ───────────────────────────────────────────────────────────────────

agents:
  angee-admin:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system
    role: operator
    description: "Platform admin — deploys, scales, manages infrastructure, installs Odoo modules"
    skills: [platform-ops, odoo-admin]
    mcp_servers: [angee-operator, odoo-llm]
    connectors: [anthropic]
    files:
      - template: opencode.json.tmpl
        mount: /root/.config/opencode/opencode.json
    workspace:
      persistent: true

  webmaster:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system
    role: user
    description: "Webmaster — manages Odoo website pages, content, SEO, and navigation"
    skills: [odoo-website]
    mcp_servers: [odoo-llm]
    connectors: [anthropic]
    workspace:
      persistent: true

  bookkeeper:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system
    role: user
    description: "Bookkeeper — manages invoices, payments, journal entries, reconciliation, and financial reports"
    skills: [odoo-accounting]
    mcp_servers: [odoo-llm]
    connectors: [anthropic]
    workspace:
      persistent: true

# ─── Repositories ─────────────────────────────────────────────────────────────

repositories:
  odoo-llm:
    url: https://github.com/apexive/odoo-llm
    branch: "18.0"
    path: src/odoo-llm

  custom-addons:
    url: https://github.com/company/odoo-custom-addons
    branch: main
    path: src/custom-addons

# ─── Secrets ──────────────────────────────────────────────────────────────────

secrets:
  - name: db-password
    generated: true
    length: 32

  - name: odoo-admin-password
    generated: true
    length: 24
    description: "Odoo master/admin password"

  - name: odoo-mcp-key
    required: true
    description: "Odoo MCP API key (generate from Odoo: My Profile → Account Security → New MCP Key)"

  - name: google-client-id
    required: true
    description: "Google OAuth client ID"

  - name: google-client-secret
    required: true

  - name: github-client-id
    required: true

  - name: github-client-secret
    required: true

  - name: smtp-relay-password
    description: "SMTP relay password (optional — skip if using Gmail directly)"

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

### Step 1: Initialize

```bash
angee init --repo https://github.com/company/odoo-custom-addons
```

Angee generates `db-password` and `odoo-admin-password`, prompts for `google-client-id`, `google-client-secret`, `github-client-id`, `github-client-secret`, and clones repositories. The odoo-llm addons are cloned into `src/odoo-llm/`.

### Step 2: Configure Odoo

Before deploying, create the Odoo config file that the container will mount:

```ini
# config/odoo.conf
[options]
addons_path = /mnt/extra-addons,/usr/lib/python3/dist-packages/odoo/addons
admin_passwd = ${ODOO_ADMIN_PASSWORD}
db_host = postgres
db_port = 5432
db_user = odoo
db_password = ${DB_PASSWORD}
smtp_server = smtp.gmail.com
smtp_port = 465
smtp_ssl = True
smtp_user = you@gmail.com
smtp_password = ${GOOGLE_ACCESS_TOKEN}
email_from = you@gmail.com
proxy_mode = True
```

The SMTP credentials come from the `google` connector — same OAuth token that grants Gmail access.

### Step 3: Connect accounts

```bash
angee connect google       # OAuth → Gmail access for Odoo email
angee connect anthropic    # API key for AI agents
angee connect github       # OAuth → repo access for addon development
angee connect smtp         # optional: SMTP relay password (if not using Gmail)
```

### Step 4: Deploy

```bash
angee up
```

What comes up:

```
Infrastructure:
  operator      — angee REST API + MCP (port 9000)
  traefik       — reverse proxy (ports 80/443)
  openbao       — secrets vault (port 8200)

Application:
  odoo          — Odoo 18 at erp.company.local (port 8069)
  postgres      — PostgreSQL 15
  redis         — session cache

Agents:
  agent-angee-admin  — platform ops + Odoo admin
  agent-webmaster    — website management
  agent-bookkeeper   — accounting
```

### Step 5: Install odoo-llm modules

After Odoo starts, install the MCP server modules. The admin agent can do this:

```bash
angee admin
```

```
You: Install the odoo-llm MCP server and the accounting and website tool modules.

Admin agent:
  → calls odoo_model_method_executor via odoo-llm MCP:
      model: ir.module.module
      method: button_immediate_install
      domain: [("name", "in", ["llm_mcp_server", "llm_tool_account", "llm_tool_website"])]
  → Odoo installs llm_mcp_server (which auto-installs llm, llm_tool, web_json_editor)
  → Odoo installs llm_tool_account (18 accounting tools)
  → Odoo installs llm_tool_website (31 website tools)
  → reports: "Installed 3 modules. MCP server is running at /mcp with 55+ tools."
```

Then generate an MCP API key:

```
You: Generate an MCP API key for the agents.

Admin agent:
  → calls odoo_model_method_executor:
      model: res.users
      method: _mcp_create_key (or via UI instructions)
  → stores the key: angee credential set odoo-mcp-key <key>
  → reports: "MCP key stored. All agents can now authenticate with Odoo."
```

### Step 6: Use the bookkeeper

```bash
angee chat bookkeeper
```

The bookkeeper agent connects to Odoo via odoo-llm MCP. It has access to 18 accounting tools plus the 6 core CRUD tools. Everything it does runs with Odoo's ACL — the MCP key determines permissions.

```
You: Create an invoice for Acme Corp for $5,000 consulting services, due in 30 days.

Bookkeeper agent:
  → calls odoo_record_retriever:
      model: res.partner
      domain: [("name", "ilike", "Acme")]
      fields: ["id", "name", "property_payment_term_id"]
  → finds partner_id = 42

  → calls create_invoice (llm_tool_account):
      partner_id: 42
      move_type: out_invoice
      invoice_date: 2026-03-02
      lines: [{name: "Consulting services", price_unit: 5000, quantity: 1}]
      payment_term: 30 days
  → invoice created: INV/2026/0047 (draft)

  → reports: "Created draft invoice INV/2026/0047 for Acme Corp — $5,000.
              Review at erp.company.local/web#id=47&model=account.move
              Say 'post it' to confirm."
```

```
You: Post it and send to the customer.

Bookkeeper:
  → calls post_move: move_id = 47
  → calls odoo_model_method_executor:
      model: account.move
      record_ids: [47]
      method: action_send_and_print
  → Odoo sends the invoice via Gmail (using the google connector's SMTP credentials)
  → reports: "Invoice INV/2026/0047 posted and emailed to sarah@acme.com"
```

Other bookkeeper tasks:

```
You: Show me the trial balance for Q1 2026.

Bookkeeper:
  → calls trial_balance tool (llm_tool_account)
  → returns formatted trial balance with debit/credit columns

You: Reconcile the bank statement from today.

Bookkeeper:
  → calls list_unreconciled_items → shows unmatched debits/credits
  → calls suggest_reconciliation → proposes matching pairs
  → calls reconcile_journal_items with user approval

You: Run the month-end close checklist for February.

Bookkeeper:
  → calls pre_close_checklist(period="2026-02")
  → reports: "3 unposted draft entries, 5 unreconciled bank items,
              revenue recognition complete, depreciation posted."
```

### Step 7: Use the webmaster

```bash
angee chat webmaster
```

The webmaster has 31 website tools for full CMS control:

```
You: Create a landing page for our new consulting service.

Webmaster:
  → calls find_pages to check existing pages
  → calls create_page:
      name: "Consulting Services"
      url: /consulting
      content: (generates HTML with service description, pricing, CTA)
  → calls update_seo_metadata:
      title: "Expert Consulting Services | Company Name"
      description: "Transform your business with our expert consulting..."
      keywords: ["consulting", "business strategy", "digital transformation"]
  → calls update_menu to add "Consulting" to the main navigation
  → reports: "Page created at /consulting with SEO metadata.
              Preview: erp.company.local/consulting (unpublished).
              Say 'publish' to go live."
```

```
You: Upload our new logo and update the header.

Webmaster:
  → calls upload_media with the logo file
  → calls update_page_content to replace the header logo HTML
  → calls find_pages to check all pages for the old logo reference
  → reports: "Logo updated across 12 pages."
```

```
You: Set up a redirect from /old-services to /consulting.

Webmaster:
  → calls create_redirect:
      url_from: /old-services
      url_to: /consulting
      type: 301
  → reports: "301 redirect created. /old-services → /consulting"
```

### Step 8: Admin creates new agents on the fly

The admin agent can extend the stack dynamically:

```
You: Create a sales agent that can manage CRM leads and send quotes.

Admin agent:
  → reads current angee.yaml via config_get
  → adds a new skill:
      odoo-sales:
        description: "CRM and sales — manage leads, opportunities, quotations"
        mcp_servers: [odoo-llm]
        system_prompt: "You are a sales assistant. Use Odoo to manage leads,
          create quotations, and track the sales pipeline."
  → adds a new agent:
      sales-agent:
        image: ghcr.io/anomalyco/opencode:latest
        command: serve --hostname 0.0.0.0 --port 4096
        lifecycle: system
        role: user
        description: "Sales assistant — manages CRM leads, quotations, and pipeline"
        skills: [odoo-sales]
        mcp_servers: [odoo-llm]
        connectors: [anthropic, google]
  → calls config_set + deploy
  → reports: "sales-agent is running. Use 'angee chat sales-agent'."
```

The sales agent inherits the `google` connector, so it can send quotes via email directly from Odoo.

---

## How Connectors Flow

```
angee connect google
  │
  ├─ OAuth → token stored in OpenBao as connector-google
  │
  ▼
angee deploy
  │
  ├─ Compiler reads connectors from angee.yaml
  ├─ Odoo doesn't declare connectors directly — email config is in odoo.conf
  │   └─ odoo.conf references ${GOOGLE_ACCESS_TOKEN} for SMTP
  ├─ Agents with connectors: [anthropic] get ANTHROPIC_API_KEY injected
  ├─ Agents with connectors: [google] get GOOGLE_ACCESS_TOKEN injected
  │
  ▼
Runtime
  ├─ odoo: SMTP uses Gmail OAuth token (from odoo.conf)
  ├─ agent-angee-admin: has ANTHROPIC_API_KEY
  ├─ agent-webmaster: has ANTHROPIC_API_KEY
  ├─ agent-bookkeeper: has ANTHROPIC_API_KEY
  └─ agent-sales-agent: has ANTHROPIC_API_KEY + GOOGLE_ACCESS_TOKEN
```

---

## Comparison with Fyltr-Unibox Example

Both examples follow the same angee pattern, but with different applications:

| Aspect | Fyltr Unibox | Odoo |
|--------|-------------|------|
| **Application** | Custom Django app (messaging + contacts) | Off-the-shelf ERP |
| **MCP server** | Custom fyltr-mcp (built into Django) | odoo-llm addon (100+ tools) |
| **Build** | Built from source (`build: context:`) | Official Docker image (`image: odoo:18.0`) |
| **Email** | IMAP sync into Unibox database | Odoo's built-in email (SMTP/IMAP) |
| **Connectors** | google, whatsapp, github, anthropic | google, smtp, github, anthropic |
| **Agent pattern** | Researcher queries the app's data | Bookkeeper/webmaster operate the ERP |
| **Data access** | Agents query via custom MCP tools | Agents use generic CRUD + domain tools |
| **Dynamic agents** | Admin creates outreach agent | Admin creates sales agent |

The key similarity: **connectors are shared stack-level resources**, agents connect to the application via MCP, and the admin agent can reconfigure the entire stack by editing `angee.yaml`.

---

## What the odoo-llm MCP Server Provides

The [apexive/odoo-llm](https://github.com/apexive/odoo-llm) project runs as an Odoo addon, exposing a streamable-http MCP endpoint at `/mcp`. Agents authenticate with a Bearer token (Odoo API key) and all operations respect Odoo's standard ACL and record rules.

### Core tools (6) — universal Odoo access

| Tool | What it does |
|------|-------------|
| `odoo_model_inspector` | Introspect any model: fields, methods, inheritance |
| `odoo_record_retriever` | Search and read records from any model |
| `odoo_record_creator` | Create records in any model |
| `odoo_record_updater` | Update records by domain filter |
| `odoo_record_unlinker` | Delete records by domain filter |
| `odoo_model_method_executor` | Call any method on any model (confirm orders, validate pickings, etc.) |

### Accounting tools (18) — `llm_tool_account`

Journal entries, invoices, payments, trial balance, P&L, reconciliation, period close.

### Website tools (31) — `llm_tool_website`

Pages, content, media, menus, SEO, redirects, visitor analytics.

### Financial reporting tools (44) — `llm_tool_mis_builder`

MIS Builder reports, KPIs, period comparisons, variance analysis, drill-down.

### Additional modules

- `llm_tool_knowledge` — RAG/semantic search over Odoo knowledge base
- `llm_tool_ocr_mistral` — OCR for invoices, receipts, documents

With the 6 core CRUD tools alone, an agent can operate on any Odoo model — `sale.order`, `stock.picking`, `hr.employee`, `project.task` — making it possible to build agents for any Odoo workflow without writing custom tools.
