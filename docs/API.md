# HTTP API Reference

The operator listens on port 9000 (configurable in `operator.yaml`). All endpoints return JSON. Errors use the envelope `{"error": "message"}`.

## Authentication

When `ANGEE_API_KEY` is set (environment variable or `api_key` in `operator.yaml`), all endpoints except `/health` require:

```
Authorization: Bearer <api-key>
```

Unauthenticated requests receive `401 {"error": "unauthorized"}`.

## Endpoints

### Health

#### `GET /health`

Returns operator liveness. Always bypasses auth.

**Response:**
```json
{
  "status": "ok",
  "root": "/home/user/.angee",
  "runtime": "docker-compose"
}
```

---

### Config

#### `GET /config`

Returns the current `angee.yaml` as parsed JSON.

**Response:** The full `AngeeConfig` object — `name`, `version`, `services`, `agents`, `mcp_servers`, `repositories`, `secrets`.

#### `POST /config`

Writes new `angee.yaml` content, validates it, and commits to git. Optionally deploys immediately.

**Request:**
```json
{
  "content": "name: my-platform\nservices: ...",
  "message": "add redis cache",
  "deploy": true
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content` | string | yes | Raw YAML content for angee.yaml |
| `message` | string | no | Git commit message (default: `"angee-agent: update config"`) |
| `deploy` | bool | no | If true, deploy immediately after committing |

**Response:**
```json
{
  "sha": "a1b2c3d",
  "message": "add redis cache",
  "deploy": {
    "services_started": ["redis"],
    "services_updated": [],
    "services_removed": []
  }
}
```

The `deploy` field is only present when `deploy: true` was requested.

---

### Deployment lifecycle

#### `POST /deploy`

Compiles `angee.yaml` and applies it to the runtime. This is the primary deploy mechanism.

**Request:**
```json
{
  "message": "optional deploy note"
}
```

**Response:**
```json
{
  "services_started": ["web", "db", "agent-developer"],
  "services_updated": ["worker"],
  "services_removed": []
}
```

#### `GET /plan`

Dry-run deploy. Shows what would change without applying.

**Response:**
```json
{
  "add": ["redis"],
  "update": ["web"],
  "remove": ["old-worker"]
}
```

#### `POST /rollback`

Reverts to a previous git commit and redeploys.

**Request:**
```json
{
  "sha": "a1b2c3d"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sha` | string | yes | Git commit SHA or ref to roll back to |

**Response:**
```json
{
  "rolled_back_to": "a1b2c3d",
  "deploy": {
    "services_started": [],
    "services_updated": ["web", "worker"],
    "services_removed": ["redis"]
  }
}
```

---

### Service management

#### `GET /status`

Returns the runtime state of all services and agents.

**Response:**
```json
[
  {
    "name": "web",
    "type": "service",
    "status": "running",
    "health": "healthy",
    "replicas_running": 2,
    "replicas_desired": 2,
    "domains": ["app.example.com"]
  },
  {
    "name": "agent-developer",
    "type": "agent",
    "status": "running",
    "health": "unknown",
    "replicas_running": 1,
    "replicas_desired": 1
  }
]
```

#### `GET /logs/{service}`

Returns logs for a specific service.

**Query parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `lines` | int | 100 | Number of log lines to return |
| `follow` | bool | false | Stream logs (keep connection open) |
| `since` | string | — | Only logs after this timestamp |

**Response:** `text/plain` log output.

#### `GET /logs`

Returns logs for all services (combined). Same query parameters as above.

#### `POST /scale/{service}`

Adjusts replica count for a service.

**Request:**
```json
{
  "replicas": 3
}
```

**Response:**
```json
{
  "service": "web",
  "replicas": 3
}
```

#### `POST /down`

Brings the entire stack down. Stops and removes all containers.

**Response:**
```json
{
  "status": "down"
}
```

#### `POST /restart/{service}` *(planned)*

Restart a single service without a full redeploy.

#### `POST /exec/{service}` *(planned)*

Execute a command inside a running container. For migrations, management commands, scripts.

**Planned request:**
```json
{
  "command": ["python", "manage.py", "migrate"]
}
```

---

### Agent management

#### `GET /agents`

Lists all agents defined in `angee.yaml` with their current runtime status.

**Response:**
```json
[
  {
    "name": "developer",
    "lifecycle": "system",
    "role": "operator",
    "status": "running",
    "health": "unknown"
  },
  {
    "name": "reviewer",
    "lifecycle": "agent",
    "role": "user",
    "status": "stopped",
    "health": "unknown"
  }
]
```

#### `POST /agents/{name}/start`

Starts a stopped agent. Recompiles the compose file and applies to ensure the agent's config is current.

**Response:** Same as deploy — `ApplyResult` with `services_started`, `services_updated`, `services_removed`.

#### `POST /agents/{name}/stop`

Stops a running agent.

**Response:**
```json
{
  "status": "stopped",
  "agent": "developer"
}
```

#### `GET /agents/{name}/logs`

Returns logs for a specific agent.

**Query parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `follow` | bool | false | Stream logs |

**Response:** `text/plain` log output (up to 200 lines by default).

---

### Credential management

#### `GET /credentials`

Lists all credential names and the active backend type.

**Response:**
```json
{
  "names": ["ANTHROPIC_API_KEY", "DB_URL", "DJANGO_SECRET_KEY"],
  "backend": "env"
}
```

#### `GET /credentials/{name}`

Returns metadata about a credential — existence and value length. The raw value is never exposed via the API.

**Response:**
```json
{
  "name": "db-url",
  "exists": true,
  "length": 42
}
```

Returns `404` if the credential does not exist.

#### `POST /credentials/{name}`

Stores a credential value.

**Request:**
```json
{
  "value": "postgres://user:pass@db:5432/mydb"
}
```

**Response:**
```json
{
  "status": "ok",
  "name": "db-url"
}
```

#### `DELETE /credentials/{name}`

Removes a credential.

**Response:**
```json
{
  "status": "ok",
  "name": "db-url"
}
```

---

### Repository management *(planned)*

These endpoints manage source repositories defined in `angee.yaml` under `repositories:`.

#### `GET /repos` *(planned)*

List all configured repositories and their local status.

#### `POST /repos/{name}/clone` *(planned)*

Clone a repository into the workspace.

#### `POST /repos/{name}/pull` *(planned)*

Pull latest changes from the remote.

#### `POST /repos/{name}/checkout` *(planned)*

Switch to a different branch.

**Planned request:**
```json
{
  "branch": "feature/new-api"
}
```

#### `GET /repos/{name}/status` *(planned)*

Git status of the repository (modified files, branch, clean/dirty).

#### `GET /repos/{name}/log` *(planned)*

Recent git log for the repository.

---

### Git history

#### `GET /history`

Returns recent git commits from ANGEE_ROOT (the config repo, not source repos).

**Query parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `n` | int | 20 | Number of commits to return |

**Response:**
```json
[
  {
    "sha": "a1b2c3d",
    "message": "deploy: add redis cache",
    "author": "angee-operator",
    "date": "2026-02-27"
  }
]
```

---

### MCP endpoint

#### `POST /mcp`

JSON-RPC 2.0 endpoint for MCP (Model Context Protocol) tool calls. AI agents connect here to manage the platform programmatically.

**Protocol:** JSON-RPC 2.0 over HTTP POST (streamable HTTP transport).

**Supported methods:**

| Method | Description |
|--------|-------------|
| `initialize` | Handshake — returns server info and capabilities |
| `tools/list` | Lists all available MCP tools |
| `tools/call` | Invokes a tool by name with arguments |

**Example — list tools:**
```json
{
  "jsonrpc": "2.0",
  "method": "tools/list",
  "id": 1
}
```

**Example — call a tool:**
```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "platform_status",
    "arguments": {}
  },
  "id": 2
}
```

**Response format:**
```json
{
  "jsonrpc": "2.0",
  "result": {
    "content": [{"type": "text", "text": "[{\"name\":\"web\",\"status\":\"running\",...}]"}]
  },
  "id": 2
}
```

See [MCP.md](MCP.md) for the complete tool reference.

---

### OpenAPI schema

#### `GET /openapi.json`

Returns the OpenAPI 3.1 schema for all HTTP endpoints. Always bypasses auth.

---

### `angee update` (CLI only)

Update platform components after initial setup:

```sh
angee update template   # Re-fetch and re-render the template
angee update agents     # Pull latest agent images
angee update skills     # Update skill definitions (placeholder)
```

---

## Error responses

All errors return a JSON object with an `error` field:

```json
{"error": "agent \"foo\" not found in angee.yaml"}
```

Common status codes:

| Code | Meaning |
|------|---------|
| 400 | Bad request — invalid body, missing required fields, invalid config |
| 401 | Unauthorized — missing or invalid API key |
| 404 | Not found — agent or service doesn't exist |
| 500 | Internal error — runtime failure, git error, compile error |
