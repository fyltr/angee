# Clean Refactor Implementation Plan

## Goal

Rewrite angee-go from scratch following strict clean architecture. Both the CLI and the operator are thin adapters on top of a single shared core. Zero business logic duplication.

---

## The Problem Today

1. **handlers.go and mcp.go duplicate ~200 lines** of identical business logic (deploy, rollback, config, agents, status — all implemented twice)
2. **CLI commands duplicate the compile sequence** (up.go, restart.go, pull.go all repeat the same 15-line load → validate → render → compile → write block)
3. **CLI commands duplicate HTTP client patterns** (doRequest → ReadAll → check status → Unmarshal — repeated in every command)
4. **api.ApplyResult mirrors runtime.ApplyResult** with a manual converter function
5. **No connector CRUD API** — connectors need full REST + MCP endpoints

---

## Clean Architecture: The Layer Diagram

```
┌─────────────────────────────────────────────────────────┐
│                    Delivery Layer                         │
│                                                          │
│   cmd/angee/        cmd/operator/                        │
│       │                  │                               │
│       ▼                  ▼                               │
│    cli/             internal/operator/                   │
│  (cobra cmds)       (HTTP + MCP adapters)               │
│       │                  │                               │
│       │    ┌─────────────┘                               │
│       │    │                                             │
├───────┼────┼─────────────────────────────────────────────┤
│       ▼    ▼                                             │
│   ┌──────────────────────┐                               │
│   │  internal/service/   │  ← Business Logic Core        │
│   │                      │     (THE source of truth)     │
│   │  Platform struct     │     All operations live here  │
│   │  - Deploy()          │     Returns (result, error)   │
│   │  - Rollback()        │     Knows nothing about HTTP  │
│   │  - ConfigGet/Set()   │     Knows nothing about MCP   │
│   │  - AgentList/Start() │     Knows nothing about CLI   │
│   │  - ConnectorCRUD()   │                               │
│   │  - ServiceLogs()     │                               │
│   │  - CredentialCRUD()  │                               │
│   └──────────┬───────────┘                               │
│              │                                           │
├──────────────┼───────────────────────────────────────────┤
│              ▼                                           │
│   ┌──────────────────────┐                               │
│   │  Infrastructure      │                               │
│   │                      │                               │
│   │  internal/config/    │  YAML types + validation      │
│   │  internal/compiler/  │  angee.yaml → compose.yaml    │
│   │  internal/runtime/   │  RuntimeBackend interface     │
│   │  internal/creds/     │  CredentialBackend interface  │
│   │  internal/root/      │  ANGEE_ROOT filesystem        │
│   │  internal/git/       │  git operations               │
│   │  api/                │  shared request/response types│
│   └──────────────────────┘                               │
└─────────────────────────────────────────────────────────┘
```

**The rule:** Dependencies point inward. The service layer imports infrastructure. Delivery adapters import the service layer. Nothing imports delivery.

---

## Package Design

### `api/` — Shared Types

Request/response structs used by both CLI and operator. No logic.

```
api/
└── types.go
```

Types (all have json + yaml tags):
- `HealthResponse`
- `ApplyResult`, `ChangeSet` — canonical versions (runtime returns these directly)
- `ConfigSetRequest`, `ConfigSetResponse`
- `DeployRequest`, `RollbackRequest`, `RollbackResponse`
- `ScaleRequest`, `ScaleResponse`
- `ServiceStatus`, `AgentInfo`
- `CommitInfo`, `DownResponse`, `AgentActionResponse`
- `ConnectorSpec`, `ConnectorCreateRequest`, `ConnectorUpdateRequest` — **new**
- `CredentialSetRequest` — **new**
- `ErrorResponse`

### `internal/config/` — YAML Config Types

```
internal/config/
├── angee.go       # AngeeConfig + all sub-specs (add ConnectorSpec)
├── operator.go    # OperatorConfig
├── validate.go    # structural validation
└── overlay.go     # environment overlay merging
```

Key addition to `AngeeConfig`:

```go
type AngeeConfig struct {
    Name            string                       `yaml:"name"`
    Version         string                       `yaml:"version,omitempty"`
    Connectors      map[string]ConnectorSpec     `yaml:"connectors,omitempty"`
    Services        map[string]ServiceSpec       `yaml:"services,omitempty"`
    MCPServers      map[string]MCPServerSpec     `yaml:"mcp_servers,omitempty"`
    Agents          map[string]AgentSpec         `yaml:"agents,omitempty"`
    Skills          map[string]SkillSpec         `yaml:"skills,omitempty"`
    Repositories    map[string]RepositorySpec    `yaml:"repositories,omitempty"`
    Secrets         []SecretRef                  `yaml:"secrets,omitempty"`
    SecretsBackend  *SecretsBackendConfig        `yaml:"secrets_backend,omitempty"`
}

type ConnectorSpec struct {
    Provider     string            `yaml:"provider"`
    Type         string            `yaml:"type"`                    // oauth | api_key | token | setup_command
    Description  string            `yaml:"description,omitempty"`
    Required     bool              `yaml:"required,omitempty"`
    Tags         []string          `yaml:"tags,omitempty"`
    Metadata     map[string]any    `yaml:"metadata,omitempty"`
    Env          map[string]string `yaml:"env,omitempty"`
    OAuth        *OAuthConfig      `yaml:"oauth,omitempty"`
    SetupCommand *SetupCommandSpec `yaml:"setup_command,omitempty"`
}
```

Services and agents reference connectors:

```go
type ServiceSpec struct {
    // ... existing fields ...
    Connectors []string `yaml:"connectors,omitempty"`
}

type AgentSpec struct {
    // ... existing fields ...
    Connectors []string `yaml:"connectors,omitempty"`
}
```

### `internal/service/` — Business Logic Core (**new**)

This is the heart of the refactor. **All** business logic lives here. Both HTTP handlers and MCP tools call these methods.

```
internal/service/
├── platform.go      # Platform struct, Deploy, Rollback, Plan, Down, Health, Status
├── config.go        # ConfigGet, ConfigSet, writeAndCommit (private helper)
├── agents.go        # AgentList, AgentStart, AgentStop, AgentLogs
├── connectors.go    # ConnectorList, ConnectorGet, ConnectorCreate, ConnectorUpdate, ConnectorDelete, OAuthStart, OAuthCallback, ConnectorStatus
├── credentials.go   # CredentialList, CredentialSet, CredentialDelete
├── logs.go          # ServiceLogs (shared log retrieval)
├── history.go       # History
├── errors.go        # ServiceError type with HTTP status codes
└── health.go        # HealthChecker (moved from operator)
```

#### The Platform struct

```go
type Platform struct {
    Root     *root.Root
    Cfg      *config.OperatorConfig
    Backend  runtime.RuntimeBackend
    Compiler *compiler.Compiler
    Git      *git.Repo
    Creds    creds.Backend
    Health   *HealthChecker
    Log      *slog.Logger
}
```

#### Method signatures — every method returns `(result, error)`:

```go
// platform.go
func (p *Platform) HealthCheck() *api.HealthResponse
func (p *Platform) Deploy(ctx context.Context, message string) (*api.ApplyResult, error)
func (p *Platform) Plan(ctx context.Context) (*api.ChangeSet, error)
func (p *Platform) Rollback(ctx context.Context, sha string) (*api.RollbackResponse, error)
func (p *Platform) Status(ctx context.Context) ([]*api.ServiceStatus, error)
func (p *Platform) Down(ctx context.Context) error

// config.go
func (p *Platform) ConfigGet() (*config.AngeeConfig, error)
func (p *Platform) ConfigSet(ctx context.Context, content, message string, deploy bool) (*api.ConfigSetResponse, error)

// agents.go
func (p *Platform) AgentList(ctx context.Context) ([]api.AgentInfo, error)
func (p *Platform) AgentStart(ctx context.Context, name string) (*api.ApplyResult, error)
func (p *Platform) AgentStop(ctx context.Context, name string) error
func (p *Platform) AgentLogs(ctx context.Context, name string, lines int) (string, error)

// connectors.go
func (p *Platform) ConnectorList(tags []string) ([]api.ConnectorSpec, error)
func (p *Platform) ConnectorGet(name string) (*api.ConnectorSpec, error)
func (p *Platform) ConnectorCreate(ctx context.Context, req api.ConnectorCreateRequest) (*api.ConnectorSpec, error)
func (p *Platform) ConnectorUpdate(ctx context.Context, name string, req api.ConnectorUpdateRequest) (*api.ConnectorSpec, error)
func (p *Platform) ConnectorDelete(ctx context.Context, name string) error
func (p *Platform) ConnectorOAuthStart(name string) (redirectURL string, err error)
func (p *Platform) ConnectorOAuthCallback(ctx context.Context, code, state string) error
func (p *Platform) ConnectorStatus(name string) (bool, error)

// credentials.go
func (p *Platform) CredentialList() ([]string, error)
func (p *Platform) CredentialSet(name, value string) error
func (p *Platform) CredentialDelete(name string) error

// logs.go
func (p *Platform) ServiceLogs(ctx context.Context, service string, opts runtime.LogOptions) (io.ReadCloser, error)

// history.go
func (p *Platform) History(n int) ([]api.CommitInfo, error)
```

#### errors.go — typed errors for HTTP status mapping

```go
type ServiceError struct {
    Status  int
    Message string
}

func (e *ServiceError) Error() string { return e.Message }

func NotFound(msg string) error    { return &ServiceError{404, msg} }
func BadRequest(msg string) error  { return &ServiceError{400, msg} }
func Conflict(msg string) error    { return &ServiceError{409, msg} }
```

HTTP handlers use `errorStatus(err)` to extract the status code. MCP tools just return the error message. The service layer decides the semantics; the adapters decide the encoding.

#### Private helpers (shared within the service package):

```go
// platform.go — used by Deploy, ConfigSet, AgentStart, ConnectorCreate, etc.
func (p *Platform) prepareAndCompile(cfg *config.AngeeConfig) error
func (p *Platform) writeAndCommit(cfg *config.AngeeConfig, message string) error
func (p *Platform) loadConfig() (*config.AngeeConfig, error)
func (p *Platform) restartHealthProbes(ctx context.Context)

// agents.go
func (p *Platform) agentServiceName(name string) string  // "agent-" + name
func (p *Platform) buildStatusMap(ctx context.Context) (map[string]*runtime.ServiceStatus, error)
```

### `internal/operator/` — HTTP + MCP Adapters (thin)

```
internal/operator/
├── server.go          # Server struct, Handler(), Start(), middleware
├── handlers.go        # HTTP handlers (3-5 lines each)
├── mcp.go             # MCP JSON-RPC protocol + dispatch (thin)
├── mcp_tools.go       # MCP tool definitions (names, descriptions, schemas)
└── openapi.go         # OpenAPI 3.1 spec
```

#### server.go

```go
type Server struct {
    Platform *service.Platform
    Log      *slog.Logger
}

func New(angeeRoot string) (*Server, error) {
    plat, err := service.NewPlatform(angeeRoot)
    if err != nil {
        return nil, err
    }
    return &Server{Platform: plat, Log: plat.Log}, nil
}

func (s *Server) Handler() http.Handler {
    mux := http.NewServeMux()

    // Health
    mux.HandleFunc("GET /health", s.handleHealth)

    // Config
    mux.HandleFunc("GET /config", s.handleConfigGet)
    mux.HandleFunc("POST /config", s.handleConfigSet)

    // Deploy
    mux.HandleFunc("POST /deploy", s.handleDeploy)
    mux.HandleFunc("GET /plan", s.handlePlan)
    mux.HandleFunc("POST /rollback", s.handleRollback)

    // Runtime
    mux.HandleFunc("GET /status", s.handleStatus)
    mux.HandleFunc("GET /logs/{service}", s.handleLogs)
    mux.HandleFunc("POST /scale/{service}", s.handleScale)
    mux.HandleFunc("POST /down", s.handleDown)

    // Agents
    mux.HandleFunc("GET /agents", s.handleAgentList)
    mux.HandleFunc("POST /agents/{name}/start", s.handleAgentStart)
    mux.HandleFunc("POST /agents/{name}/stop", s.handleAgentStop)
    mux.HandleFunc("GET /agents/{name}/logs", s.handleAgentLogs)

    // Connectors
    mux.HandleFunc("GET /connectors", s.handleConnectorList)
    mux.HandleFunc("POST /connectors", s.handleConnectorCreate)
    mux.HandleFunc("GET /connectors/{name}", s.handleConnectorGet)
    mux.HandleFunc("PATCH /connectors/{name}", s.handleConnectorUpdate)
    mux.HandleFunc("DELETE /connectors/{name}", s.handleConnectorDelete)
    mux.HandleFunc("GET /connectors/{name}/start", s.handleConnectorOAuthStart)
    mux.HandleFunc("GET /connectors/callback", s.handleConnectorOAuthCallback)
    mux.HandleFunc("GET /connectors/{name}/status", s.handleConnectorStatus)

    // Credentials
    mux.HandleFunc("GET /credentials", s.handleCredentialList)
    mux.HandleFunc("POST /credentials", s.handleCredentialSet)
    mux.HandleFunc("DELETE /credentials/{name}", s.handleCredentialDelete)

    // History + MCP + OpenAPI
    mux.HandleFunc("GET /history", s.handleHistory)
    mux.HandleFunc("POST /mcp", s.handleMCP)
    mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)

    return s.middleware(mux)
}
```

#### handlers.go — every handler is a thin adapter:

```go
func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
    var req api.DeployRequest
    json.NewDecoder(r.Body).Decode(&req) // ignores decode error for empty body (no message)
    result, err := s.Platform.Deploy(r.Context(), req.Message)
    if err != nil {
        writeError(w, err)
        return
    }
    writeJSON(w, result)
}

func (s *Server) handleConnectorList(w http.ResponseWriter, r *http.Request) {
    tags := r.URL.Query()["tags"]
    result, err := s.Platform.ConnectorList(tags)
    if err != nil {
        writeError(w, err)
        return
    }
    writeJSON(w, result)
}

func (s *Server) handleConnectorCreate(w http.ResponseWriter, r *http.Request) {
    var req api.ConnectorCreateRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, service.BadRequest("invalid request body"))
        return
    }
    result, err := s.Platform.ConnectorCreate(r.Context(), req)
    if err != nil {
        writeError(w, err)
        return
    }
    writeJSON(w, result)
}
```

Two shared response helpers:

```go
func writeJSON(w http.ResponseWriter, v any) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
    code := 500
    var se *service.ServiceError
    if errors.As(err, &se) {
        code = se.Status
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(api.ErrorResponse{Error: err.Error()})
}
```

#### mcp.go — protocol + dispatch (thin):

```go
func (s *Server) dispatchTool(ctx context.Context, name string, args json.RawMessage) (any, error) {
    switch name {
    // Platform
    case "platform_health":
        return s.Platform.HealthCheck(), nil
    case "platform_status":
        return s.Platform.Status(ctx)
    case "platform_down":
        return nil, s.Platform.Down(ctx)

    // Config
    case "config_get":
        return s.Platform.ConfigGet()
    case "config_set":
        var p struct {
            Content string `json:"content"`
            Message string `json:"message"`
            Deploy  bool   `json:"deploy"`
        }
        if err := json.Unmarshal(args, &p); err != nil {
            return nil, err
        }
        return s.Platform.ConfigSet(ctx, p.Content, p.Message, p.Deploy)

    // Deploy
    case "deploy":
        return s.Platform.Deploy(ctx, "")
    case "deploy_plan":
        return s.Platform.Plan(ctx)
    case "deploy_rollback":
        var p struct{ SHA string `json:"sha"` }
        json.Unmarshal(args, &p)
        return s.Platform.Rollback(ctx, p.SHA)

    // Services
    case "service_logs":
        var p struct {
            Service string `json:"service"`
            Lines   int    `json:"lines"`
        }
        json.Unmarshal(args, &p)
        if p.Lines == 0 { p.Lines = 100 }
        return s.Platform.ServiceLogsText(ctx, p.Service, p.Lines)
    case "service_scale":
        var p api.ScaleRequest
        json.Unmarshal(args, &p)
        return s.Platform.Scale(ctx, p.Service, p.Replicas)

    // Agents
    case "agent_list":
        return s.Platform.AgentList(ctx)
    case "agent_start":
        var p struct{ Name string `json:"name"` }
        json.Unmarshal(args, &p)
        return s.Platform.AgentStart(ctx, p.Name)
    case "agent_stop":
        var p struct{ Name string `json:"name"` }
        json.Unmarshal(args, &p)
        return nil, s.Platform.AgentStop(ctx, p.Name)
    case "agent_logs":
        var p struct{ Name string `json:"name"`; Lines int `json:"lines"` }
        json.Unmarshal(args, &p)
        if p.Lines == 0 { p.Lines = 200 }
        return s.Platform.AgentLogs(ctx, p.Name, p.Lines)

    // Connectors
    case "connector_list":
        var p struct{ Tags []string `json:"tags"` }
        json.Unmarshal(args, &p)
        return s.Platform.ConnectorList(p.Tags)
    case "connector_create":
        var req api.ConnectorCreateRequest
        json.Unmarshal(args, &req)
        return s.Platform.ConnectorCreate(ctx, req)
    case "connector_delete":
        var p struct{ Name string `json:"name"` }
        json.Unmarshal(args, &p)
        return nil, s.Platform.ConnectorDelete(ctx, p.Name)

    // Credentials
    case "credentials_list":
        return s.Platform.CredentialList()
    case "credentials_set":
        var p api.CredentialSetRequest
        json.Unmarshal(args, &p)
        return nil, s.Platform.CredentialSet(p.Name, p.Value)
    case "credentials_delete":
        var p struct{ Name string `json:"name"` }
        json.Unmarshal(args, &p)
        return nil, s.Platform.CredentialDelete(p.Name)

    // History
    case "history":
        var p struct{ N int `json:"n"` }
        json.Unmarshal(args, &p)
        if p.N == 0 { p.N = 20 }
        return s.Platform.History(p.N)

    default:
        return nil, fmt.Errorf("unknown tool: %s", name)
    }
}
```

Every case: unmarshal args (if any) → call Platform method → return. The MCP JSON-RPC protocol framing (initialize, tools/list, tools/call) stays in mcp.go but is just protocol code, not business logic.

### `cli/` — CLI Commands (DRY)

```
cli/
├── root.go        # root cmd, global flags, resolveRoot/resolveOperator
├── client.go      # NEW: shared HTTP client helpers
├── compile.go     # NEW: shared local compile sequence
├── init.go        # angee init
├── up.go          # angee up (local compile + docker compose)
├── deploy.go      # angee deploy (POST /deploy)
├── plan.go        # angee plan (GET /plan)
├── rollback.go    # angee rollback (POST /rollback)
├── ls.go          # angee ls (GET /status)
├── logs.go        # angee logs (GET /logs)
├── chat.go        # angee chat/admin/develop (docker exec)
├── connect.go     # angee connect (connector flows)
├── credential.go  # angee credential CRUD
├── add.go         # angee add (components)
├── remove.go      # angee remove (components)
├── pull.go        # angee pull (local compile + docker compose pull)
├── restart.go     # angee restart
└── destroy.go     # angee destroy
```

#### client.go — eliminates all HTTP client duplication:

```go
// apiGet decodes a JSON GET response into result.
// If --json flag is set, prints raw JSON and returns (nil, nil).
func apiGet[T any](path string) (*T, error)

// apiPost sends JSON body and decodes response.
func apiPost[T any](path string, body any) (*T, error)

// apiDelete sends DELETE and returns error if any.
func apiDelete(path string) error

// apiStream sends GET and streams response body to stdout.
func apiStream(path string) error
```

Every CLI command becomes 5-10 lines: parse flags → call apiGet/apiPost → format output.

#### compile.go — eliminates the up/restart/pull duplication:

```go
// localCompile loads angee.yaml, validates, renders agent files,
// compiles docker-compose.yaml, and returns the compose path.
func localCompile(rootPath string) (composePath string, projectName string, err error)
```

`up.go`, `restart.go`, `pull.go` all call `localCompile()` then run docker compose.

### `internal/creds/` — Credential Backend

```
internal/creds/
├── backend.go         # Backend interface
├── env.go             # .env file backend
├── factory.go         # NewBackend() factory
└── openbao/
    └── openbao.go     # OpenBao KV v2 backend
```

Interface (unchanged):
```go
type Backend interface {
    Get(ctx context.Context, name string) (string, error)
    Set(ctx context.Context, name, value string) error
    Delete(ctx context.Context, name string) error
    List(ctx context.Context) ([]string, error)
    Type() string
}
```

### `internal/runtime/` — RuntimeBackend (unchanged)

```
internal/runtime/
├── backend.go              # interface + types
└── compose/
    └── backend.go          # docker compose implementation
```

### `internal/compiler/` — Compiler (minor addition)

```
internal/compiler/
├── compose.go    # Compile() — add connector env injection
└── hooks.go      # RenderAgentFiles, template functions
```

Addition: when compiling services/agents with `connectors: [name]`, the compiler:
1. Looks up the connector's `env` mapping
2. Injects credential references as env vars (e.g., `GH_TOKEN=${CONNECTOR_GITHUB}`)

### `internal/root/`, `internal/git/`, `internal/tmpl/` — unchanged

---

## Implementation Phases

### Phase 1: Create service layer + refactor operator

1. Create `internal/service/` with `Platform` struct
2. Move all business logic from `handlers.go` into service methods
3. Move all business logic from `mcp.go` tool methods into same service methods
4. Rewrite `handlers.go` as thin HTTP adapters (3-5 lines each)
5. Rewrite `mcp.go` dispatch to call Platform methods
6. Move `HealthChecker` to `internal/service/health.go`
7. Add `ServiceError` type for status code mapping
8. Add `writeJSON`/`writeError` helpers in server.go

**Result:** handlers.go shrinks from ~430 to ~150 lines. mcp.go shrinks from ~660 to ~300 lines. service/ adds ~400 lines (single copy of all logic).

### Phase 2: DRY CLI

1. Create `cli/client.go` with generic HTTP helpers
2. Create `cli/compile.go` with shared local compile
3. Simplify all CLI commands to use these helpers
4. Remove duplicated io.ReadAll/Unmarshal/status-check patterns

**Result:** CLI commands shrink to 5-15 lines each. compile.go is ~50 lines used by 3 commands.

### Phase 3: Connector CRUD

1. Add `ConnectorSpec` to `internal/config/angee.go`
2. Add `Connectors` field to `AngeeConfig`
3. Add connector validation to `validate.go`
4. Implement `ConnectorList/Get/Create/Update/Delete` on Platform
5. Implement `ConnectorOAuthStart/Callback/Status` on Platform
6. Add HTTP routes in server.go
7. Add MCP tools in mcp.go dispatch
8. Add connector types to api/types.go
9. Update compiler to inject connector env vars
10. Update openapi.go

### Phase 4: Rename connected_accounts → connectors

1. Update all config struct field names
2. Update all YAML tags
3. Update all handler/tool references
4. Update templates
5. Update tests

### Phase 5: Tests

1. Service layer unit tests (mock runtime + creds backends)
2. Handler integration tests (HTTP → service → mock backend)
3. MCP integration tests (JSON-RPC → service → mock backend)
4. Connector CRUD tests
5. CLI compile helper tests

---

## Verification

After the refactor, verify:

```bash
# Build both binaries
make build

# Run all tests
make test

# Lint
make lint

# Full pre-commit check
make check

# Manual smoke test
make run-operator        # start operator
angee init               # scaffold
angee up                 # compile + start
angee status             # check services
angee deploy -m "test"   # deploy via operator
angee plan               # dry-run
angee connect --help     # connector CLI
angee logs traefik       # stream logs
angee rollback HEAD~1    # rollback
angee down               # tear down
```

MCP smoke test (via curl):
```bash
# Initialize
curl -X POST http://localhost:9000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"initialize","params":{},"id":1}'

# List tools
curl -X POST http://localhost:9000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","params":{},"id":2}'

# Call deploy
curl -X POST http://localhost:9000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"deploy","arguments":{}},"id":3}'
```

Connector API test:
```bash
# List connectors
curl http://localhost:9000/connectors

# Create connector
curl -X POST http://localhost:9000/connectors \
  -H "Content-Type: application/json" \
  -d '{"name":"test-imap","provider":"custom","type":"api_key","tags":["email"]}'

# Get connector
curl http://localhost:9000/connectors/test-imap

# Delete connector
curl -X DELETE http://localhost:9000/connectors/test-imap
```

---

## File Count Estimate

| Package | Files | Lines (est.) | Purpose |
|---------|-------|-------------|---------|
| `api/types.go` | 1 | ~150 | Shared types |
| `internal/config/` | 4 | ~550 | YAML types, validation, overlay |
| `internal/service/` | 8 | ~500 | Business logic core |
| `internal/operator/` | 5 | ~600 | HTTP + MCP adapters |
| `internal/compiler/` | 2 | ~550 | Compilation |
| `internal/runtime/` | 2 | ~320 | Backend interface + compose |
| `internal/creds/` | 4 | ~500 | Credential backends |
| `internal/root/` | 1 | ~190 | ANGEE_ROOT fs |
| `internal/git/` | 1 | ~200 | Git wrapper |
| `internal/tmpl/` | 1 | ~400 | Templates |
| `cli/` | 16 | ~1200 | CLI commands |
| `cmd/` | 2 | ~60 | Entry points |
| **Total** | **~47** | **~5200** | Down from ~10k+ |
