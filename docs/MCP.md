# MCP Tools Reference

The angee-operator exposes an MCP (Model Context Protocol) server that AI agents use to manage the platform. This document is written for agent authors — it describes what your agent can do through the operator.

## Connection

| | |
|---|---|
| Transport | Streamable HTTP |
| URL | `http://operator:9000/mcp` |
| Protocol | JSON-RPC 2.0 over HTTP POST |
| Auth | `Bearer <ANGEE_API_KEY>` in the Authorization header |

In `angee.yaml`, configure the operator as an MCP server for your agent:

```yaml
mcp_servers:
  operator:
    transport: streamable-http
    url: http://operator:9000/mcp

agents:
  my-agent:
    mcp_servers: [operator]
```

> **Status**: The MCP endpoint is **implemented** at `POST /mcp`. All tools listed below are functional.

## Tools

### Platform status

#### `platform_health`

Check if the operator is running.

**Parameters:** none

**Returns:**
```json
{
  "status": "ok",
  "root": "/home/user/.angee",
  "runtime": "docker-compose"
}
```

#### `platform_status`

Get the runtime state of all services and agents.

**Parameters:** none

**Returns:**
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
  }
]
```

---

### Configuration

#### `config_get`

Read the current `angee.yaml` configuration.

**Parameters:** none

**Returns:** The full platform configuration as a JSON object.

#### `config_set`

Write a new `angee.yaml`, validate it, and commit to git.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `content` | string | yes | Raw YAML content |
| `message` | string | no | Git commit message |
| `deploy` | bool | no | Deploy immediately after saving |

**Example call:**
```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "config_set",
    "arguments": {
      "content": "name: my-platform\nservices:\n  web:\n    image: nginx\n    lifecycle: platform\n",
      "message": "add nginx service",
      "deploy": true
    }
  },
  "id": 1
}
```

**Returns:**
```json
{
  "sha": "a1b2c3d",
  "message": "add nginx service",
  "deploy": {
    "services_started": ["web"],
    "services_updated": [],
    "services_removed": []
  }
}
```

---

### Deployment

#### `deploy`

Compile the current `angee.yaml` and apply it to the runtime.

**Parameters:** none (or optional `message` string)

**Returns:**
```json
{
  "services_started": ["web", "db"],
  "services_updated": [],
  "services_removed": []
}
```

#### `deploy_plan`

Dry-run: see what would change without actually deploying.

**Parameters:** none

**Returns:**
```json
{
  "add": ["redis"],
  "update": ["web"],
  "remove": []
}
```

#### `deploy_rollback`

Roll back to a previous configuration and redeploy.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `sha` | string | yes | Git commit SHA to roll back to |

**Returns:**
```json
{
  "rolled_back_to": "a1b2c3d",
  "deploy": {
    "services_started": [],
    "services_updated": ["web"],
    "services_removed": ["redis"]
  }
}
```

---

### Service management

#### `service_logs`

Get logs for a service.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `service` | string | no | Service name. Omit for all services |
| `lines` | int | no | Number of lines (default: 100) |
| `since` | string | no | Only logs after this timestamp |

**Returns:** Log text as a string.

#### `service_scale`

Scale a service to a given replica count.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `service` | string | yes | Service name |
| `replicas` | int | yes | Desired replica count |

**Returns:**
```json
{
  "service": "web",
  "replicas": 3
}
```

#### `service_restart` *(planned)*

Restart a service without a full redeploy.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `service` | string | yes | Service name |

#### `service_exec` *(planned)*

Run a command inside a running service container.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `service` | string | yes | Service name |
| `command` | string[] | yes | Command and arguments |

**Example:**
```json
{
  "name": "service_exec",
  "arguments": {
    "service": "web",
    "command": ["python", "manage.py", "migrate"]
  }
}
```

#### `platform_down`

Bring the entire stack down.

**Parameters:** none

---

### Agent management

#### `agent_list`

List all agents and their status.

**Parameters:** none

**Returns:**
```json
[
  {
    "name": "developer",
    "lifecycle": "system",
    "role": "operator",
    "status": "running",
    "health": "unknown"
  }
]
```

#### `agent_start`

Start a stopped agent.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | yes | Agent name |

#### `agent_stop`

Stop a running agent.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | yes | Agent name |

#### `agent_logs`

Get logs for an agent.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | yes | Agent name |
| `follow` | bool | no | Stream logs continuously |

---

### Repository management *(planned)*

#### `repo_list` *(planned)*

List configured source repositories.

#### `repo_clone` *(planned)*

Clone a repository into the workspace.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | yes | Repository name from angee.yaml |

#### `repo_pull` *(planned)*

Pull latest changes.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | yes | Repository name |

#### `repo_checkout` *(planned)*

Switch branch.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | yes | Repository name |
| `branch` | string | yes | Branch to check out |

#### `repo_status` *(planned)*

Git status of a repository.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | yes | Repository name |

#### `repo_log` *(planned)*

Git log for a repository.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `name` | string | yes | Repository name |
| `n` | int | no | Number of commits (default: 20) |

---

### Git history

#### `history`

Get the config change history (commits in ANGEE_ROOT).

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `n` | int | no | Number of commits (default: 20) |

**Returns:**
```json
[
  {
    "sha": "a1b2c3d",
    "message": "deploy: add redis",
    "author": "angee-operator",
    "date": "2026-02-27"
  }
]
```

---

## Common workflows

### Deploy a config change

```
1. config_get          → read current config
2. config_set          → write updated config (with deploy: false)
3. deploy_plan         → check what would change
4. deploy              → apply it
```

### Debug a failing service

```
1. platform_status     → find which service is unhealthy
2. service_logs        → read its logs
3. config_get          → check its configuration
4. service_restart     → restart it (planned)
```

### Scale under load

```
1. platform_status     → check current replica counts
2. service_scale       → increase replicas for the service
3. platform_status     → verify new replicas are running
```

### Roll back a bad deploy

```
1. history             → find the last good commit SHA
2. deploy_rollback     → revert to it and redeploy
3. platform_status     → verify everything recovered
```

### Manage agent lifecycle

```
1. agent_list          → see which agents are running
2. agent_stop          → stop an agent
3. agent_start         → start it back up
4. agent_logs          → check what it's been doing
```

## Tool status summary

| Tool | Status |
|------|--------|
| `platform_health` | maps to implemented endpoint |
| `platform_status` | maps to implemented endpoint |
| `config_get` | maps to implemented endpoint |
| `config_set` | maps to implemented endpoint |
| `deploy` | maps to implemented endpoint |
| `deploy_plan` | maps to implemented endpoint |
| `deploy_rollback` | maps to implemented endpoint |
| `service_logs` | maps to implemented endpoint |
| `service_scale` | maps to implemented endpoint |
| `service_restart` | planned |
| `service_exec` | planned |
| `platform_down` | maps to implemented endpoint |
| `agent_list` | maps to implemented endpoint |
| `agent_start` | maps to implemented endpoint |
| `agent_stop` | maps to implemented endpoint |
| `agent_logs` | maps to implemented endpoint |
| `repo_list` | planned |
| `repo_clone` | planned |
| `repo_pull` | planned |
| `repo_checkout` | planned |
| `repo_status` | planned |
| `repo_log` | planned |
| `history` | maps to implemented endpoint |
| **MCP endpoint itself** | **implemented** (`POST /mcp`) |
