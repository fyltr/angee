# MCP API

The operator exposes an MCP server so agents can inspect, provision, and operate Angee resources.

The MCP tools call the same operator services as the CLI and HTTP API.

## Connection

| Field | Value |
|---|---|
| Transport | Streamable HTTP |
| URL | `http://operator:9000/mcp` |
| Auth | `Bearer <ANGEE_API_KEY>` when configured |

Example manifest snippet:

```yaml
mcp_servers:
  operator:
    transport: streamable-http
    url: http://operator:9000/mcp

agents:
  developer:
    mcp_servers: [operator]
```

## Tool Groups

| Group | Examples |
|---|---|
| Stack | Read/write manifest, stack init, stack update, reconcile, deploy, rollback. |
| Sources | List, inspect, sync, materialize. |
| Workspaces | Init, update, inspect, destroy. |
| Agents | Init, update, start, stop, logs, inspect. |
| Services | List, start, stop, restart, logs. |
| Jobs | Run, list, inspect, cancel, logs. |
| Workflows | Start, inspect, cancel, logs. |
| Secrets | List metadata and rotate values without exposing secret values. |
| Port leases | List allocations and release stale leases. |

## Important Rules

- MCP tools must not expose secret values.
- Agents should operate on Angee resource names: sources, services, jobs, workflows, workspaces, agents, MCP servers, and port leases.
- Provisioning tools must call the operator provisioning path, not duplicate init logic.
- Destructive operations should require confirmation or explicit tool parameters.

## Example Agent Workflow

1. Inspect the stack manifest.
2. Sync a source to the requested ref and tree.
3. Run a job such as `test` or `migrate`.
4. Tail service or job logs.
5. If a manifest change is needed, update `angee.yaml` and ask the operator to reconcile.
