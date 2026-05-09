# Operator API

Run the standalone operator:

```sh
angee operator --root . --bind 127.0.0.1 --port 9000
```

Non-loopback binds require `--token`. Protected endpoints use:

```http
Authorization: Bearer <token>
```

## REST

Health:

```http
GET /healthz
```

Stack:

```http
GET  /stack/status
POST /stack/init
POST /stack/update
POST /stack/prepare
POST /stack/build
POST /stack/up
POST /stack/dev
POST /stack/down
POST /stack/destroy?purge=true
GET  /stack/logs?service=name
```

Services:

```http
GET   /services
POST  /services
PATCH /services/{name}
POST  /services/{name}/start
POST  /services/{name}/stop
POST  /services/{name}/restart
POST  /services/{name}/destroy
GET   /services/{name}/logs
```

Jobs:

```http
GET  /jobs
POST /jobs/{name}/run
```

Job output is returned by `POST /jobs/{name}/run`.

Sources:

```http
GET  /sources
GET  /sources/{name}/status
POST /sources/{name}/fetch
POST /sources/{name}/pull
POST /sources/{name}/push
```

Workspaces:

```http
GET   /workspaces
POST  /workspaces
GET   /workspaces/{name}
PATCH /workspaces/{name}
GET   /workspaces/{name}/status
GET   /workspaces/{name}/logs
POST  /workspaces/{name}/start
POST  /workspaces/{name}/stop
POST  /workspaces/{name}/restart
POST  /workspaces/{name}/destroy?purge=true
GET   /workspaces/{name}/git
POST  /workspaces/{name}/push
```

Events and MCP descriptor:

```http
GET /events
GET /mcp
```

`/events` currently emits a single `ready` SSE event. `/mcp` currently returns a
static descriptor; it is not a JSON-RPC MCP server.

## GraphQL

GraphQL is available at:

```http
POST /graphql
Content-Type: application/json
```

Example:

```sh
curl -s http://127.0.0.1:9000/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ stackStatus { name root services { name runtime status } } }"}'
```

The GraphQL schema exposes stack, service, job, source, workspace, log snapshot,
and mutation fields corresponding to the REST operations.
