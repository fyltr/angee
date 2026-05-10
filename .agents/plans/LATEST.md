# Build-vs-Reuse Migration Plan

**Status:** Draft · **Created:** 2026-05-10 · **Source:** Internet build-vs-reuse audit + grounded review of repo

This plan turns the audit's three highest-leverage reuse opportunities into
discrete, executable migrations and tracks the small remaining backlog from
the previous refactor pass (whose completed items are recorded in
`CHANGELOG.md`). Each migration section is sized so it can be merged
independently.

## Context

`angee-go` is a self-managed stack manager. Most of the codebase is correctly
scoped to the differentiator (manifest schema, workspace/source management,
substitution resolver, port pool, operator API surface). The audit identified
three subsystems where in-house code duplicates a mature OSS solution:

| # | Subsystem | Custom LoC | OSS replacement | Effort |
|---|-----------|------------|-----------------|--------|
| 1 | OpenBao HTTP client (`internal/secrets/openbao.go`) | 120 | `github.com/openbao/openbao/api/v2` (MPL-2.0) | Small |
| 2 | MCP operator endpoint (`internal/operator/mcp.go`) | 16 (stub) | `modelcontextprotocol/go-sdk` (Apache-2.0) **or** `mark3labs/mcp-go` (MIT) | Medium |
| 3 | Compose YAML model (`internal/runtime/compose/doc.go`) | 33 | `compose-spec/compose-go/v2` (Apache-2.0) | Medium |

Order: **#1 first** (lowest risk, highest staleness liability), **#2 next**
(must precede MCP feature work), **#3 last** (interacts with manifest compiler;
can wait until a missing Compose field forces it).

## Migration 1 — OpenBao official client

**Goal:** Replace the hand-rolled KV v2 HTTP client with the official OpenBao
Go API library. Eliminate the divergence risk on auth, retry, TLS, namespaces,
and token renewal.

**Verified-on 2026-05-10:** `github.com/openbao/openbao/api/v2` v2.5.1
(Feb 2026), MPL-2.0, official upstream library. Wraps the same KV v2 endpoints
the custom client speaks.

### Files

- Replace: `internal/secrets/openbao.go` (120 lines)
- Touch: `internal/secrets/backend.go` (interface unchanged), `go.mod`,
  `go.sum`, any operator wiring that constructs `OpenBaoBackend`.

### Steps

1. `go get github.com/openbao/openbao/api/v2@latest`
2. Rewrite `OpenBaoBackend` to hold an `*api.Client` and a `*api.KVv2` handle.
   Construct via `api.NewClient(&api.Config{Address: ...})` then
   `client.SetToken(token)` and `client.KVv2(mount)`.
3. Map the `Backend` interface methods to KVv2 calls:
   - `Get` → `kv.Get(ctx, path)` returning `secret.Data["value"]`. Treat
     `api.ErrSecretNotFound` (or 404 status on the response) as the
     `(value="", ok=false, err=nil)` case the rest of the code expects.
   - `Set` → `kv.Put(ctx, path, map[string]any{"value": value})`.
   - `Delete` → `kv.Delete(ctx, path)`.
   - `List` → use `client.Logical().List("<mount>/metadata/<path>")` and
     extract `data.keys`. This finally implements the method that currently
     returns `"openbao list is not implemented"` (`openbao.go:71`).
4. Preserve the existing env-var fallbacks (`OPENBAO_ADDR`, `OPENBAO_TOKEN`)
   and defaults (`mount=secret`, `path=angee`). The official client also reads
   `VAULT_ADDR`/`BAO_ADDR` automatically — keep our explicit precedence so
   behavior does not silently change.
5. Update tests. The current test surface uses a mock HTTP server; switch the
   mocks to satisfy the OpenBao client's expected request/response shape (path
   prefix unchanged: `/v1/<mount>/data/<path>`). Add coverage for the new
   `List` implementation.

### Acceptance

- `go test -race ./internal/secrets/...` passes.
- The `${secret:...}` resolver path (`internal/secrets/resolve.go`) works
  end-to-end against a local `openbao server -dev` instance.
- `internal/secrets/openbao.go` no longer imports `net/http`,
  `encoding/json`, or `bytes`.

### Risk / rollback

- Risk: low. The `Backend` interface is unchanged; only the implementation
  differs.
- Rollback: revert the package; no manifest, schema, or runtime files change.

## Migration 2 — MCP server SDK adoption

**Goal:** Replace the 16-line MCP descriptor stub with a real MCP server
implementation **before** expanding the surface to dispatch tools. Doing this
now (while it's a stub) is near-free; doing it after building 18 tools by hand
means rewriting protocol framing and transport.

**Verified-on 2026-05-10:**
- `github.com/modelcontextprotocol/go-sdk` v1.6.0 (May 2026), Apache-2.0,
  4.5k★. Official, Anthropic + Google co-maintained. Stdio transport, tool
  registration, schema validation built-in.
- `github.com/mark3labs/mcp-go` v0.49.0 (April 2026), MIT, 8.6k★. Stdio +
  SSE + streamable-HTTP, per-session state, hooks/middleware. Better fit for
  an HTTP-native operator.

### Decision: which SDK

Recommend **`mark3labs/mcp-go`** for angee. Reasons:

- The operator is HTTP-native; mcp-go's streamable-HTTP transport drops in,
  whereas the official SDK leans stdio-first today.
- Per-session state matches the per-stack control-plane model.
- The MIT license matches the rest of the dependency set.

If the official SDK reaches transport parity in the next minor, revisit. The
public-facing MCP tool surface is small and a future swap is cheap.

### Files

- Rewrite: `internal/operator/mcp.go` (16 lines → real handler)
- Touch: `internal/operator/operator.go` (mount the MCP HTTP handler on the
  existing mux)
- Tools dispatch through `service.Platform`, matching the REST/GraphQL
  pattern from `CLAUDE.md`.

### Tool surface (initial, mirrors existing descriptor + planned expansion)

Stack: `stack.status`, `stack.up`, `stack.down`, `stack.compile`.
Services: `services.list`, `services.create`, `services.logs`, `services.restart`.
Workspaces: `workspaces.list`, `workspaces.create`, `workspaces.sync_base`.
Sources: `sources.list`, `sources.fetch`, `sources.pull`.
Jobs: `jobs.run`.
Secrets: `secrets.list`, `secrets.set`, `secrets.delete`.

Each tool is a thin `func(ctx, args) (result, error)` that calls a
`service.Platform` method. Input/output schemas come from `api/` request/
response DTOs — generate JSON Schemas via the existing `invopop/jsonschema`
dependency to avoid hand-writing them.

### Steps

1. `go get github.com/mark3labs/mcp-go@latest`.
2. In `internal/operator/mcp.go`, build a `*server.MCPServer` configured with
   server name `"angee-operator"` and the operator's existing version string.
3. Register each tool by calling `mcp.NewTool(name, mcp.WithDescription(...),
   mcp.WithInputSchema(...))` and pairing it with a handler that unmarshals
   args, calls the matching `Platform` method, and marshals the result.
4. Mount the HTTP transport: `server.NewStreamableHTTPServer(mcpServer)` and
   register on the operator's existing mux at `/mcp` (replacing the stub
   descriptor route).
5. Keep the descriptor JSON available at `/mcp/info` for older clients during
   the transition.
6. Add an end-to-end test that opens an MCP session, calls `stack.status`,
   and asserts the response matches the REST `/stack/status` payload — proving
   surface parity per the architecture rule in `CLAUDE.md`.

### Acceptance

- `go test -race ./internal/operator/...` passes including a new
  `mcp_e2e_test.go`.
- `mcp inspect http://localhost:9000/mcp` (or equivalent client) lists all
  registered tools with valid input schemas.
- `internal/operator/mcp.go` no longer hard-codes a `[]string` of tool names;
  the list comes from server state.

### Risk / rollback

- Risk: medium. Adds a new dependency and new HTTP route. No existing client
  depends on the descriptor stub's exact shape (it's not in `api/`).
- Rollback: revert the route; the descriptor stub returns.

## Migration 3 — Adopt compose-go as compile IR (deferred trigger)

**Goal:** Replace the local minimal `compose.File` model with
`compose-spec/compose-go` types so the manifest compiler emits spec-valid
Compose by construction and gains new spec fields (e.g., `develop.watch`,
`build.secrets`, named profiles) without compiler changes.

**Verified-on 2026-05-10:** `github.com/compose-spec/compose-go/v2` v2.10.2
(April 2026), Apache-2.0, 431★. The reference library used by Docker Desktop,
Podman, Testcontainers, and `score-compose`.

**Status:** Evaluated and deferred (see CHANGELOG R7 entry). The container
runtime currently renders exactly the fields used by manifests and templates
(`name`, `services.{image,build,command,environment,ports,volumes,working_dir,depends_on}`,
named `volumes.{driver,name}`). No present manifest needs networks,
healthcheck, labels, deploy, profiles, secrets/configs, or extra_hosts.
Trigger this migration when (a) a template needs an unsupported Compose field,
or (b) angee starts validating full Compose semantics. Until then, the local
minimal model is fit-for-purpose.

### Files

- Rewrite: `internal/runtime/compose/doc.go` (33 lines, model + Marshal)
- Update: `internal/service/platform.go:460-499` (writeCompiled, Text) —
  swap `compose.File` for `*types.Project`.
- Update: any `internal/service/` site that constructs a `compose.Service`
  literal.

### Steps

1. `go get github.com/compose-spec/compose-go/v2@latest`.
2. Replace `compose.File` with `types.Project` and `compose.Service` with
   `types.ServiceConfig`. Keep `compose.Marshal` as a thin wrapper around
   `project.MarshalYAML()` so callers in `service/platform.go` don't churn.
3. Where the compiler builds a `compose.Service{Image: ..., Environment: ...}`
   literal, populate `types.ServiceConfig` instead. Note:
   `types.MappingWithEquals` for environment, `types.PortConfig` for ports
   (richer than the current `[]string`), `types.DependsOnConfig` for
   depends_on (current local `ServiceDependency` is already shaped to match).
4. Run compose-go's normalizer (`project.WithoutUnnecessaryResources()` and
   friends) before marshaling to ensure stable output.
5. Snapshot-test a representative `angee.yaml` → `docker-compose.yaml` and
   diff against the pre-migration output. Acceptable diffs: re-ordered keys,
   added defaults; not acceptable: changed semantics.
6. Once green, expose the richer fields (e.g., `develop.watch`) as opt-in
   manifest knobs.

### Acceptance

- `go test -race ./internal/service/... ./internal/runtime/compose/...` passes.
- Generated `docker-compose.yaml` validates with `docker compose config -q`
  (add this to CI for the example stack).
- A snapshot test shows the diff vs. the pre-migration output is limited to
  whitelisted normalization changes.

### Risk / rollback

- Risk: medium-high. The compiler's output is a contract observed by users
  who run `docker compose` directly against generated files.
- Rollback: revert; the local model returns. Snapshot test catches
  regressions before merge.

## Deferred / parked

Tracked here so they aren't lost; not on the active roadmap.

### Full go-git migration (network / worktree / merge)

Read-only ops have already moved to go-git (CHANGELOG R1a). The remaining
network, worktree, and merge/rebase operations stay on the `git` CLI. To
revisit, all three of the following must hold:

- `go-git/go-git#1956` (parallel checkout) lands so workspace creation latency
  on a 100k-file repo is within 2× of the CLI baseline. **Verified-on
  2026-05-10:** still open; no parallel-checkout PR merged.
- Credential-helper parity: SSH key auto-detection from `~/.ssh/`, HTTPS via
  `git credential` helper exec, GPG signing parity.
- Native go-git merge/rebase (currently plumbing only) reaches semantic parity
  with `git --ff-only` and standard rebase against the fixtures in
  `internal/service/workspaces_test.go`.

Until then the hybrid boundary documented in the `internal/git/` package
doc-comment is the long-term shape, not a stopgap.

### Compose-spec/compose-go adoption

See Migration 3 above. Trigger conditions documented there.

## Out of scope

The audit confirmed these remain in-house — do **not** propose replacements:

- `internal/substitute/` — domain-specific namespaced expression language.
- `fyltr/copier-go` — no Go-native scaffolder with `copier update` semantics.
- `internal/ports/` — domain-specific named lease allocator. `phayes/freeport`
  is unmaintained and solves a different problem.
- `internal/runtime/compose/backend.go` and `internal/runtime/proccompose/` —
  correct to shell out to `docker compose` / `process-compose` for execution.
- `internal/git/` — hybrid go-git + CLI shell-out is intentional; go-git's
  worktree gaps are documented in the package.
- `angee.yaml` schema and the manifest compiler's input parser.

## Sequencing

1. **Now:** Migration 1 (OpenBao). Self-contained, low risk, eliminates an
   ongoing maintenance liability.
2. **Before next MCP tool work:** Migration 2 (mcp-go). Cheap while the
   surface is a stub; expensive once the surface grows.
3. **When a Compose-spec field gap blocks a feature:** Migration 3
   (compose-go). Plan it, but don't run it on spec.

## References

- compose-spec/compose-go: https://github.com/compose-spec/compose-go
- modelcontextprotocol/go-sdk: https://github.com/modelcontextprotocol/go-sdk
- mark3labs/mcp-go: https://github.com/mark3labs/mcp-go
- openbao api v2: https://pkg.go.dev/github.com/openbao/openbao/api/v2
