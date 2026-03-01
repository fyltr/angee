# Angee Component Specification

Components are the unit of composition in Angee. Everything that can be added to a stack — services, agents, applications, credentials — is a component.

## Overview

```
angee add angee/postgres
angee add fyltr/fyltr-django
angee add fyltr/health-agent
angee add angee/oauth-github
angee add angee/claude-max
```

Each component lives in a git repo with an `angee-component.yaml` at the root. This file declares what the component adds to a stack: services, agents, MCP servers, secrets, config files, and credential outputs.

---

## Component Types

| Type | What it adds | Example |
|------|-------------|---------|
| `service` | `services:` entries | `angee/postgres`, `angee/redis`, `angee/openbao` |
| `agent` | `agents:` + optional `mcp_servers:` | `fyltr/health-agent`, `fyltr/finance-agent` |
| `application` | `services:` + `mcp_servers:` + `repositories:` + workers | `fyltr/fyltr-django` |
| `module` | Extends an existing application | `fyltr/django-billing` |
| `credential` | OAuth/auth setup + config templates | `angee/oauth-github`, `angee/claude-max` |

The operator doesn't care about types — it just sees the final merged `angee.yaml`. Types are a packaging concern for `angee add` and the admin agent.

---

## Naming Convention

```
<namespace>/<component-name>
```

- `angee/` — official infrastructure components
- `fyltr/` — your org's components
- Any GitHub `<org>/<repo>` with `angee-component.yaml` at root

Resolution:
```
angee add fyltr/fyltr-django
  → git clone https://github.com/fyltr/fyltr-django
  → read angee-component.yaml
```

Override with full URL:
```
angee add https://github.com/custom-org/my-component
angee add ./local/path/to/component
```

---

## `angee-component.yaml` Format

### Identity

```yaml
name: fyltr/fyltr-django              # required — namespace/name
type: application                      # service | agent | application | module | credential
version: "1.0.0"                       # semver
description: |
  Django application framework with Celery workers.
  Requires: postgres, redis.
```

### Dependencies

```yaml
requires:                              # components that must exist in the stack
  - angee/postgres
  - angee/redis

extends: fyltr/fyltr-django            # for modules: parent component (must be installed)
```

When `angee add` finds missing `requires`, it prompts:
```
fyltr/fyltr-django requires angee/postgres (not installed). Add it? [Y/n]
```

### Parameters

```yaml
parameters:
  - name: Domain
    description: "Primary domain for Django"
    default: "localhost"
    required: false

  - name: DjangoWorkers
    description: "Number of gunicorn workers"
    default: "3"
```

Parameters are resolved at install time via `--param` flags or interactive prompt. Template expressions `{{ .ParamName }}` in the component YAML are replaced with resolved values before merging.

### Stack Fragments

These sections mirror `angee.yaml` and are merged into it:

```yaml
repositories:
  base:
    url: https://github.com/fyltr/fyltr-django
    branch: main
    role: base

services:
  django:
    build:
      context: src/base
      dockerfile: Dockerfile
    lifecycle: platform
    domains:
      - host: "{{ .Domain }}"
        port: 8000
    env:
      DATABASE_URL: "${secret:db-url}"
      SECRET_KEY: "${secret:django-secret-key}"
    depends_on:
      - postgres
      - redis

  celery-worker:
    build:
      context: src/base
      dockerfile: Dockerfile
    command: "celery -A config worker --loglevel=info --concurrency={{ .CeleryWorkers }}"
    lifecycle: worker
    depends_on:
      - postgres
      - redis

mcp_servers:
  django-mcp:
    transport: streamable-http
    url: http://django:8000/mcp/

agents:
  angee-developer:
    image: ghcr.io/anomalyco/opencode:latest
    command: serve --hostname 0.0.0.0 --port 4096
    lifecycle: system
    role: user
    mcp_servers:
      - angee-files
      - django-mcp
    workspace:
      repository: base
      persistent: true
    env:
      ANTHROPIC_API_KEY: "${secret:anthropic-api-key}"

secrets:
  - name: django-secret-key
    description: "Django SECRET_KEY"
    required: true
    generated: true
    length: 50
```

### Template Syntax Rules

Two substitution syntaxes with different lifetimes:

| Syntax | Resolved when | By what | Survives into angee.yaml? |
|--------|--------------|---------|--------------------------|
| `{{ .Param }}` | Install time (`angee add`) | Go text/template | No — replaced with value |
| `${secret:name}` | Compile time (`angee deploy`) | Compiler + secrets backend | Yes — lives in angee.yaml |

After `angee add`, `angee.yaml` contains only `${secret:...}` references, never `{{ }}`.

---

## File Manifest

Components declare files that go to different places, rendered at different phases:

```yaml
files:
  # Workspace files — copied into agent workspace at install time
  - source: agents/angee-developer/AGENTS.md
    target: workspace
    agent: angee-developer
    phase: install

  # Root templates — copied to ANGEE_ROOT, re-rendered every deploy
  - source: templates/opencode.json.tmpl
    target: root
    phase: deploy

  # Agent config files — rendered + mounted at specific container path
  - source: templates/auth_creds.json.tmpl
    target: config
    mount: /root/.config/opencode/auth_creds.json
    credential: claude-session
    phase: deploy

  # Service config files — rendered + mounted into a service container
  - source: templates/nginx.conf.tmpl
    target: service-config
    mount: /etc/nginx/conf.d/default.conf
    service: nginx
    phase: deploy

  # Static files — copied as-is, no templating
  - source: scripts/migrate.sh
    target: root
    phase: install
    executable: true
```

### File Targets

| Target | Where it lands | When rendered |
|--------|---------------|--------------|
| `workspace` | `agents/<agent>/workspace/<filename>` | Install (copied once) |
| `root` | `ANGEE_ROOT/<filename>` | Install (static) or deploy (.tmpl files) |
| `config` | `agents/<agent>/<filename>` + volume mount | Deploy (every time) |
| `service-config` | Generated + volume mount into service | Deploy (every time) |

### Template Data Available at Each Phase

**Install time** (`phase: install`):
- Component parameters (ProjectName, Domain, etc.)
- No runtime data

**Deploy time** (`phase: deploy`, `target: root`):
- `AgentFileData` — same as existing `.tmpl` rendering:
  - `.AgentName` — agent name
  - `.Agent` — full AgentSpec
  - `.MCPServers` — resolved MCP servers for this agent

**Deploy time** (`phase: deploy`, `target: config` with `credential:`):
- `.Credential` — resolved credential data from vault
- `.AgentName` — agent receiving this file
- `.Agent` — full AgentSpec

---

## Credential Components

Credential components package authentication setup: OAuth flows, session tokens, API keys.

### Credential Declaration

```yaml
name: angee/oauth-github
type: credential

credential:
  name: github-oauth                   # credential identifier
  type: oauth_client                   # oauth_client | session_token | api_key

  provider:
    name: github
    auth_url: https://github.com/login/oauth/authorize
    token_url: https://github.com/login/oauth/access_token
    scopes:
      - repo
      - read:org
      - read:user

  oauth:
    callback_path: /oauth/callback/github
    consent_path: /oauth/connect/github
    token_path: /oauth/token/github

  vault_paths:
    client: secret/oauth/github/client
    tokens: secret/oauth/github/tokens/{agent}

  # How the credential reaches agent containers
  outputs:
    - type: env
      key: GITHUB_TOKEN
      value_path: access_token

    - type: file
      template: templates/github_auth.json.tmpl
      mount: /root/.config/opencode/providers/github.json

secrets:
  - name: github-client-id
    description: "GitHub OAuth App client ID (from github.com/settings/developers)"
    required: true
  - name: github-client-secret
    description: "GitHub OAuth App client secret"
    required: true

files:
  - source: templates/github_auth.json.tmpl
    target: root
    phase: install
```

### Credential Outputs

Outputs define how credential data is delivered to agent containers:

| Output type | What happens | Example |
|------------|-------------|---------|
| `env` | Value injected as env var into `agents/<name>/.env` | `GITHUB_TOKEN=gho_xxx` |
| `file` | Template rendered with credential data, mounted into container | `auth_creds.json` at specific path |

An agent opts in via `credential_bindings`:

```yaml
agents:
  health-agent:
    credential_bindings:
      - github-oauth         # gets GITHUB_TOKEN env + github_auth.json file
      - claude-session       # gets auth_creds.json file
```

### Credential Template Data

Templates declared as credential outputs receive:

```
.Credential        — map of credential fields from vault
.Credential.access_token
.Credential.token_type
.Credential.scope
.Credential.session_key
.Credential.expires_at
...

.AgentName         — agent receiving this file
.Agent             — full AgentSpec
```

Example template:

```json
{
  "provider": "github",
  "access_token": "{{ .Credential.access_token }}",
  "token_type": "{{ .Credential.token_type }}",
  "scope": "{{ .Credential.scope }}"
}
```

### Credential Types

**`oauth_client`** — Standard OAuth 2.0 with authorization code flow:
```
angee add angee/oauth-github     → stores client_id/secret
angee auth login github          → browser consent → stores refresh token
Deploy time                      → operator refreshes access token → injects
```

**`session_token`** — Browser session capture:
```
angee add angee/claude-max       → registers credential
angee auth login claude          → browser login → captures session
Deploy time                      → operator injects session into agent config
```

**`api_key`** — Simple secret with structured output:
```
angee add angee/openai-key       → prompts for API key
Deploy time                      → injects as env var
```

---

## Module Components

Modules extend an existing application component:

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
    build:
      context: src/base
      dockerfile: Dockerfile
    command: python manage.py run_stripe_webhook
    lifecycle: worker
    env:
      STRIPE_WEBHOOK_SECRET: "${secret:stripe-webhook-secret}"

secrets:
  - name: stripe-api-key
    required: true
  - name: stripe-webhook-secret
    required: true
```

The `extends` field means: "I require `fyltr/fyltr-django` to be installed. I add functionality to it."

The `django` section is application-specific metadata. The operator doesn't interpret it — the application's deploy hooks or migration scripts use it.

---

## Lifecycle Hooks

```yaml
hooks:
  post_install: scripts/post-install.sh
  pre_deploy: scripts/pre-deploy.sh
  post_deploy: scripts/post-deploy.sh
  pre_remove: scripts/pre-remove.sh
```

Hooks are shell scripts bundled in the component repo, copied to `ANGEE_ROOT/hooks/<component>/` at install time.

| Hook | When | Use case |
|------|------|----------|
| `post_install` | After `angee add` merges config | Print instructions, seed data |
| `pre_deploy` | Before each `angee deploy` | Run migrations, build assets |
| `post_deploy` | After each `angee deploy` | Health checks, notifications |
| `pre_remove` | Before `angee remove` | Cleanup, data export warnings |

---

## `angee add` Flow

```
angee add fyltr/fyltr-django --param Domain=myapp.io
│
├── 1. Resolve source
│      fyltr/fyltr-django → https://github.com/fyltr/fyltr-django
│      git clone --depth 1 → /tmp/angee-component-xxx/
│
├── 2. Read angee-component.yaml
│      Parse identity, type, requires, parameters
│
├── 3. Check dependencies
│      requires: [angee/postgres, angee/redis]
│      ├── angee/postgres in angee.yaml services? ✓
│      └── angee/redis in angee.yaml services? ✓
│      (If missing: prompt to auto-add)
│
├── 4. Resolve parameters
│      Domain = "myapp.io" (--param flag)
│      DjangoWorkers = "3" (default)
│      DjangoCPU = "1.0" (default)
│
├── 5. Render angee-component.yaml as Go template
│      {{ .Domain }} → "myapp.io"
│      ${secret:...} → preserved as-is
│
├── 6. Parse rendered YAML, extract fragments
│      repositories: {base: ...}
│      services: {django: ..., celery-worker: ..., celery-beat: ...}
│      mcp_servers: {django-mcp: ...}
│      secrets: [django-secret-key]
│
├── 7. Validate — no conflicts
│      Service names unique? Port conflicts? Domain conflicts?
│
├── 8. Merge into angee.yaml
│      Deep-merge: repositories, services, mcp_servers, agents, skills
│      Append (dedup): secrets
│      Write angee.yaml
│
├── 9. Clone repositories
│      base → src/base/
│
├── 10. Process files manifest
│       ├── workspace files → agents/<name>/workspace/
│       ├── root files → ANGEE_ROOT/
│       ├── config templates → ANGEE_ROOT/ (rendered at deploy time)
│       └── static files → ANGEE_ROOT/
│
├── 11. Generate secrets
│       django-secret-key → generated → .env (or vault)
│
├── 12. Record installation
│       Write .angee/components/fyltr-fyltr-django.yaml
│       (tracks: version, source, installed_at, parameters used)
│
├── 13. Run post_install hook (if exists)
│
├── 14. Git commit "angee: add fyltr/fyltr-django"
│
└── 15. Deploy (if --deploy or prompt)
```

## `angee remove` Flow

```
angee remove fyltr/fyltr-django
│
├── 1. Read .angee/components/fyltr-fyltr-django.yaml
│      Know what was added: services, mcp_servers, secrets, etc.
│
├── 2. Check dependents
│      Any other component requires fyltr/fyltr-django?
│      Any module extends it?
│
├── 3. Run pre_remove hook (if exists)
│
├── 4. Remove from angee.yaml
│      Remove services: django, celery-worker, celery-beat
│      Remove mcp_servers: django-mcp
│      Remove secrets: django-secret-key
│      Remove repositories: base (if no other component uses it)
│
├── 5. Clean up files
│      Remove component-specific templates, workspace files
│      Remove hooks
│
├── 6. Delete component record
│      Remove .angee/components/fyltr-fyltr-django.yaml
│
├── 7. Git commit "angee: remove fyltr/fyltr-django"
│
└── 8. Deploy (if --deploy or prompt)
```

---

## Installation Record

When a component is installed, a record is written to track what was added:

```yaml
# .angee/components/fyltr-fyltr-django.yaml
name: fyltr/fyltr-django
version: "1.0.0"
type: application
source: https://github.com/fyltr/fyltr-django
installed_at: "2026-03-01T10:30:00Z"
parameters:
  Domain: myapp.io
  DjangoWorkers: "3"
added:
  repositories: [base]
  services: [django, celery-worker, celery-beat]
  mcp_servers: [django-mcp]
  agents: [angee-developer]
  secrets: [django-secret-key]
  files:
    - agents/angee-developer/workspace/AGENTS.md
    - opencode.json.tmpl
```

This record enables clean removal and `angee list` to show installed components.

---

## Deploy-Time Credential Resolution

At every deploy, the operator processes `credential_bindings` for each agent:

```
For agent "health-agent" with credential_bindings: [github-oauth, claude-session]:

1. Load credential component metadata
   github-oauth → outputs: [{env: GITHUB_TOKEN}, {file: github_auth.json.tmpl}]
   claude-session → outputs: [{file: auth_creds.json.tmpl}]

2. Fetch credential data from vault
   github-oauth → secret/oauth/github/tokens/health-agent
   claude-session → secret/claude/session

3. Process outputs
   github-oauth/env → write GITHUB_TOKEN=gho_xxx to agents/health-agent/.env
   github-oauth/file → render github_auth.json.tmpl → agents/health-agent/github_auth.json
   claude-session/file → render auth_creds.json.tmpl → agents/health-agent/auth_creds.json

4. Add to compile output
   env_file: [./agents/health-agent/.env]
   volumes:
     - ./agents/health-agent/github_auth.json:/root/.config/opencode/providers/github.json:ro
     - ./agents/health-agent/auth_creds.json:/root/.config/opencode/auth_creds.json:ro
```

Credential files are:
- Gitignored (contain secrets)
- Regenerated every deploy (short-lived tokens)
- Read-only mounts (agents can't modify)

---

## CLI Commands

```
angee add <component> [--param Key=Value ...] [--deploy]
angee remove <component> [--deploy] [--force]
angee list                                      # list installed components
angee update <component>                        # pull new version, re-merge
angee auth login <provider>                     # run OAuth consent flow
angee auth logout <provider>                    # revoke tokens
angee auth status                               # show credential status
angee credential list                           # list all credentials
angee credential set <name> <value>             # set a secret
angee credential get <name>                     # get secret metadata (not value)
```

---

## Agent Directory After Deploy

```
agents/health-agent/
├── .env                    # GITHUB_TOKEN=gho_xxx (gitignored, regenerated)
├── opencode.json           # rendered from opencode.json.tmpl
├── github_auth.json        # rendered from credential template (gitignored)
├── auth_creds.json         # rendered from credential template (gitignored)
└── workspace/
    └── AGENTS.md           # copied at install time
```
