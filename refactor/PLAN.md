# Angee v2 Rebuild Plan

Status: proposed plan to execute the design in `refactor/OVERVIEW-v2.md`.

## Strategy

We rebuild on a clean slate, but **inside the same repo** so cherry-picking from
the old code is one `cp` away. The plan has three macro-phases:

1. **Quarantine** — move everything that isn't the design or examples into
   `.deprecated/`. Nothing buildable lives outside `.deprecated/` until we have
   re-introduced it deliberately.
2. **Skeleton** — set up the new package layout, `service.Platform`, the v2
   manifest schema, the substitution resolver, and a passing `make build` with
   a single `angee version` command.
3. **Re-introduce features** in milestones that map to the two goals from the
   overview. Each milestone is a vertical slice that ships a usable command end
   to end (CLI → `service.Platform` → backend).

Cherry-picking rule: code that lands in `.deprecated/` is **reference, not
import**. We may copy a function or a test verbatim, but a milestone is not
"done" until the new code lives in its new package and the old file is no
longer referenced. The `.deprecated/` tree is read-only once Phase 1 lands.

## Phase 1 — Quarantine the old code

Goal: leave a buildable repo with effectively empty Go packages + intact docs +
intact templates. Everything else moves wholesale to `.deprecated/`.

### Move list

| From | To |
|---|---|
| `cli/` | `.deprecated/cli/` |
| `cmd/angee/` | `.deprecated/cmd/angee/` |
| `cmd/operator/` | `.deprecated/cmd/operator/` |
| `internal/compiler/` | `.deprecated/internal/compiler/` |
| `internal/config/` | `.deprecated/internal/config/` |
| `internal/copier/` | `.deprecated/internal/copier/` |
| `internal/dev/` | `.deprecated/internal/dev/` |
| `internal/git/` | `.deprecated/internal/git/` |
| `internal/operator/` | `.deprecated/internal/operator/` |
| `internal/projmode/` | `.deprecated/internal/projmode/` |
| `internal/provision/` | `.deprecated/internal/provision/` |
| `internal/root/` | `.deprecated/internal/root/` |
| `internal/runtime/` | `.deprecated/internal/runtime/` |
| `internal/service/` | `.deprecated/internal/service/` |
| `internal/state/` | `.deprecated/internal/state/` |
| `internal/tmpl/` | `.deprecated/internal/tmpl/` |
| `api/` | `.deprecated/api/` |
| `templates/` | `.deprecated/templates/` (the **examples-v2** templates become the new seed; see Phase 2) |
| `examples/` | `.deprecated/examples/` |
| `deferred/` | leave in place — already a parking lot |
| `dist/` | gitignored, leave |

### What stays at the top level

- `refactor/` — the design (`OVERVIEW-v2.md`, `PLAN.md`, `examples-v2/`).
- `docs/` — keep `OVERVIEW.md` etc. for now; mark them stale at the top.
  Replace incrementally as features land.
- `Makefile` — strip down to `build`, `test`, `lint`, `fmt`, `vet`, `check`.
  The new make targets only know about the new layout.
- `Dockerfile.cli`, `Dockerfile.operator`, `scripts/` — stay; revisit when the
  new binaries are ready to be packaged.
- `go.mod` / `go.sum` — keep; add new deps as the plan progresses.
- `CLAUDE.md`, `README.md`, `QUICKSTART.md` — keep; they get rewritten in
  Phase 3+ as the new commands replace the old ones.

### Build invariant after Phase 1

`go build ./...` must succeed against an essentially empty tree. Concretely:

- `cmd/angee/main.go` exists with a single `angee version` command wired to a
  package-level constant. No imports beyond cobra and the new (empty)
  `internal/version`.
- `cmd/operator/main.go` exists and prints "operator v0; not implemented".
- `internal/.../*` does not exist yet; packages are introduced in Phase 2 only
  when we have something to put in them.
- Tests under `.deprecated/` are excluded from `go test ./...` because the
  package paths don't match `./...` against the new module root once we add a
  `.deprecated/go.mod` shim (see below) **or** by giving the directory a name
  Go's tooling already ignores. Implementation choice:

  - **Preferred:** rename to `_deprecated/` (Go ignores directories starting
    with `_` for `./...`, build, and test). This avoids a sub-module and keeps
    `git mv` history clean. The README clarifies the leading-underscore name
    is purely a build-toolchain convention.
  - Fallback if `_deprecated/` is inconvenient: drop a `.deprecated/go.mod`
    declaring `module github.com/fyltr/angee/deprecated` so `./...` from the
    root no longer crawls it.

  We pick `_deprecated/` to keep history readable. The plan refers to it as
  `.deprecated/` for prose continuity; the on-disk name is `_deprecated/`.

### Concrete steps (one PR)

1. `git mv` each path above into `_deprecated/<old-path>`.
2. Add `_deprecated/README.md` explaining the rule (read-only reference, copy
   forward into the new tree, do not `import` from `_deprecated/`).
3. Replace `cmd/angee/main.go` and `cmd/operator/main.go` with the minimal
   stubs described above.
4. Strip `Makefile` to its minimum surface.
5. Verify `make build && make test && make lint` is green.

After Phase 1 the repo is small, the design lives in `refactor/`, and every
old artifact is one `find _deprecated -name 'X'` away.

## Phase 2 — Skeleton

Goal: stand up the package layout from the overview, the substitution
resolver, and the v2 manifest types — but nothing that runs containers or
processes yet. Tests cover compile-time behavior only.

### New package layout

```
api/                              # request/response types shared by CLI/HTTP/MCP
internal/
  manifest/                       # v2 angee.yaml types + load/save (no validation yet)
  substitute/                     # ${ns.path | filter} grammar — see SUBSTITUTIONS.md
  secrets/                        # Backend interface + .env impl (OpenBao deferred)
  ports/                          # in-memory port pool + lease persistence in angee.yaml
  copierx/                        # thin wrapper around github.com/fyltr/copier-go
  git/                            # `git worktree add` shim (stdlib os/exec)
  fslock/                         # per-root advisory lockfile
  service/                        # service.Platform — the only business-logic layer
  runtime/
    backend.go                    # Runtime interface (Up, Down, Start, Stop, Logs, ...)
    compose/                      # docker compose adapter (Phase 4)
    proccompose/                  # process-compose adapter (Phase 5)
  operator/
    server.go                     # HTTP + SSE server (Phase 6)
    mcp.go                        # MCP JSON-RPC adapter (Phase 6)
  cli/                            # cobra commands; thin adapters on service.Platform
cmd/
  angee/                          # CLI binary
  operator/                       # daemon binary
templates/
  stacks/                         # seeded from refactor/examples-v2/templates/stacks
  agents/                         # seeded from refactor/examples-v2/templates/agents
```

Notes:

- `service.Platform` is one struct with one method per operator action
  (`StackInit`, `StackUp`, `StackDev`, `StackDown`, `StackDestroy`,
  `ServiceStart`, `ServiceStop`, `ServiceLogs`, `JobRun`, `AgentInit`,
  `AgentUpdate`, `AgentStart`, `AgentStop`, `AgentDestroy`, `AgentList`,
  `AgentGet`). The CLI, HTTP, and MCP layers are all thin shims over this.
- `internal/cli/` (not the top-level `cli/`) keeps cobra commands as Go
  helpers we can unit-test. The top-level `cli/` from v1 is gone.
- `internal/copierx/` is a 30-line wrapper. The actual templating is the
  sibling `github.com/fyltr/copier-go` library (already at `../copier-go`).
- `internal/manifest/` owns `angee.yaml` types; `internal/runtime/compose/`
  and `internal/runtime/proccompose/` own the **generated** types
  (`docker-compose.yaml`, `process-compose.yaml`). The two never share types.

### What lands in this phase

1. `internal/manifest/` — Go types matching `OVERVIEW-v2.md` §Manifest. Round-
   trip test: load `refactor/examples-v2/templates/stacks/angee-notes-dev/...`
   rendered output, marshal back, deep-equal.
2. `internal/substitute/` — implements the grammar from
   `refactor/examples-v2/SUBSTITUTIONS.md`. Resolves `${ns.path | filter}`.
   Includes only the namespaces that are pure data: `inputs`, `name`,
   `ports`, `workspace`, `operator` (URL only — host rewrite deferred until
   Phase 4 has actual addresses). `secret`, `connector`, `service`,
   `persist`, `alloc` are stubbed to return errors with TODO links until the
   relevant phase wires them up.
3. `internal/secrets/` — `Backend` interface + `EnvFileBackend`. `OpenBao`
   stays a TODO until Phase 7.
4. `internal/ports/` — pool allocator backed by `angee.yaml.operator.port_pool`.
   Allocation persists into the manifest; release happens on agent destroy.
5. `internal/fslock/` — `Lock(root)` using `flock` semantics; tests cover
   contention and stale-PID cleanup.
6. `internal/copierx/` — `Render(template, dst, answers)` and
   `Update(dst, answers)` over the `copier-go` library. No template family
   logic here; that's `service.Platform`'s job.
7. `cmd/angee/main.go` — cobra root with `version` and an `internal stack
   compile <root>` debug subcommand that prints the resolved compose +
   process-compose YAML to stdout. This is enough to drive Phase 3 tests.

### Build invariant after Phase 2

- `go test ./...` covers the four library packages above plus the manifest
  round-trip.
- `angee version` prints. `angee internal stack compile` prints YAML.
- No subprocess is spawned yet. No HTTP listener exists yet.

## Phase 3 — Goal 1, container slice (`angee up` / `down` / `logs`)

Goal: a stack with **only** `runtime: container` services works end to end.
This is the smallest vertical slice that exercises secrets resolution,
substitution, the manifest, and a real backend.

1. `internal/runtime/compose/` — adapter that translates a manifest +
   resolved substitutions into `docker-compose.yaml`, then shells out to
   `docker compose`. Implements `runtime.Backend` for container services
   (Up, Down, Start, Stop, Restart, Logs, Status). `git mv` candidate from
   `_deprecated/internal/runtime/compose/backend.go` — keep what fits, rewrite
   the rest. The output schema is new (substitution grammar changed), so the
   compose-emitter is mostly fresh code.
2. `internal/secrets/` two-pass compile:
   - Pass 1 ("reference pass"): rewrite `${secret.x}` → `${SECRET_X}` in the
     emitted `docker-compose.yaml`. Strip resolved values from any committed
     file.
   - Pass 2 ("resolve-into-env pass"): write resolved values into a 0600
     `.env` next to the compose file. Refuse to write a resolved value into
     a path that isn't gitignored (hard rule from the overview).
3. `service.Platform.StackInit / StackUp / StackDown / ServiceLogs / ServiceStart /
   ServiceStop / ServiceRestart`.
4. `internal/cli/` — `angee init`, `angee stack init`, `angee up`, `angee down`,
   `angee logs <name>`, `angee status` (alias `ls`, `ps`), `angee start`,
   `angee stop`, `angee restart`. All thin adapters.
5. Acceptance test: `templates/stacks/angee-notes-dev/` (seeded from
   `refactor/examples-v2/...`) without local services → `angee init && angee
   up` gives a healthy postgres + redis. Logs work. Down works. Secrets are
   present in `.env` and absent from any committed file.

The CLI flag surface in this phase is intentionally tiny — `--input k=v`
repeatable, `--name`, `--yes`, `--operator`, `ANGEE_OPERATOR_URL`. No
template-aware "pretty flags".

## Phase 4 — Goal 1, local slice (`angee dev`)

Goal: `angee dev` runs container sidecars + local processes + the operator
HTTP/MCP server in foreground, all together.

1. `internal/runtime/proccompose/` — emit `process-compose.yaml`, start the
   `process-compose` binary in daemon mode, and drive lifecycle through its
   REST API. Health gating via process-compose's readiness probes covers the
   cross-boundary `depends_on` between containers and host processes.
2. Address rewriting in `internal/substitute/`: now that we know whether each
   service runs as host process or container, `${service.x}` and
   `${operator.url}` resolve to different host strings per consumer location
   (overview §Address resolution). Implementation: render context carries the
   consumer's runtime; the resolver looks up the producer's runtime.
3. In-process operator HTTP+MCP listener for the lifetime of `angee dev`.
   Bound to loopback only; bearer from `${secret.operator-token}`. The same
   server type is reused as a standalone binary in Phase 6.
4. Env injection into every supervised process: `ANGEE_OPERATOR_URL`,
   `ANGEE_OPERATOR_TOKEN`, resolved port env vars. This is what lets a Django
   dev server under `angee dev` call the operator without configuration.
5. `internal/cli/dev.go` — boots compose, then process-compose, then the
   operator; tails everything; clean shutdown on Ctrl+C in reverse order.
6. Acceptance test: a stack with both `runtime: container` (postgres) and
   `runtime: local` (a Python `http.server` standing in for Django) → `angee
   dev` brings everything up; `curl $ANGEE_OPERATOR_URL/healthz` works from
   the local process; Ctrl+C tears down cleanly; container sidecars persist
   across restarts.

## Phase 5 — Goal 2, agent provisioning (the novel primitive)

Goal: `angee agent init <template> --input branch=… --start --yes` produces
a worktree, materializes a workspace, wires MCP servers, injects secrets,
starts a compose/process-compose service, and registers an addressable name.

1. `internal/manifest/` — agents block, `_angee.inputs`, `instance_naming`,
   `persist:`, `chain:`. Substitution adds `inputs`, `alloc`, `persist`,
   `workspace.*` namespaces.
2. `internal/git/` — `git worktree add`, shared cache_path semantics
   (multiple agents off one fetch).
3. `internal/copierx/` upgraded to support `_angee` metadata extraction so
   `service.Platform.AgentInit` can read inputs/instance_naming **before**
   rendering. (`copier-go` already exposes the answers/metadata API; thin
   wrapper only.)
4. `service.Platform.AgentInit` — runs the 12-step provisioning sequence
   from the overview (resolve template → compute name → allocate ports →
   resolve secrets → materialize sources → render agent files (two passes)
   → create persist dirs → start per-agent MCP local processes → run chained
   stack init/dev → register and start bundled service → write addressability
   into agent `.env`).
5. Template chaining: `service.Platform.StackInit/StackDev/StackDown/StackDestroy`
   accept an explicit `root` argument. Each call takes the per-root advisory
   lock (`internal/fslock/`) so concurrent reconciliation against the same
   root serializes; calls against different roots run in parallel.
6. `service.Platform.AgentUpdate / AgentStart / AgentStop / AgentDestroy /
   AgentList / AgentGet`. Lifecycle order from overview §Lifecycle.
7. Acceptance test: `agents/claude-angee-developer` template (seeded from
   `refactor/examples-v2/...`) → `angee agent init agents/claude-angee-developer
   --input branch=feat/issue-123 --start --yes` produces a worktree, brings up
   the chained `angee-notes-dev` stack on per-agent ports, hot-adds the bundled
   service, and `angee agent get feat-issue-123` returns the expected
   addressability shape.

## Phase 6 — Operator standalone (HTTP + MCP) and external-runtime flow

Goal: `cmd/operator/` is a real daemon. The HTTP + MCP API matches the
overview, and an external runtime (Django) can drive agent provisioning over
HTTP.

1. `cmd/operator/` — same server type as Phase 4's in-process listener,
   bound on a non-loopback address; bearer auth required.
2. `internal/operator/server.go` — REST surface from overview §Operator HTTP
   + MCP API. Handlers are 1:1 thin adapters over `service.Platform`.
3. `internal/operator/mcp.go` — MCP JSON-RPC over `/mcp` exposing the same
   operations as tools.
4. `GET /events` SSE — backed by an in-memory event bus on `service.Platform`.
   No persistence (overview §ANGEE_ROOT Layout: ephemeral state stays
   in-memory in v1).
5. Async `POST /agents`: returns 202 with `operation_id` and `status_url`.
   Backed by an in-memory operations table; lost on restart, by design.
6. Per-instance overrides: `name`, `secrets`, `ports`. Same code path as the
   CLI input parser.
7. Multi-tenancy: one operator process per ANGEE_ROOT. Refuse to start if
   the lockfile is owned by an alive PID. Recommended deployment shape
   documented in `docs/OPERATOR.md` (rewritten this phase).
8. Acceptance test: docker-compose dev stack of operator + a tiny Django app
   that `POST /agents` → SSE → addressability → `POST /agents/<n>/messages`
   (only when the template declares `service.http_port`).

## Phase 7 — Secrets backend pluralism

Goal: `OpenBao` backend lands. The Backend interface stays unchanged; the
implementation is one struct.

1. `internal/secrets/openbao/` — KV v2 over plain `net/http`. No new deps.
   Cherry-pick from `_deprecated/internal/credentials/openbao/` if intact.
2. Switch by `secrets_backend.type` in `angee.yaml`.
3. Bootstrap rule from the overview: host env vs. backend, `import: env:VAR`
   imports persist into the backend on first run.
4. Acceptance test: same stack as Phase 3 with `type: openbao` instead of
   `env-file` produces identical end-state behavior; resolved values land in
   the OpenBao instance and the gitignored `.env` is empty.

## Phase 8 — TTLs, port leases, GC, polish

Goal: the operator is a steward. Things expire on schedule. Worktrees and
ports don't leak.

1. TTL sweep — periodic loop in the operator that calls `agent destroy` on
   expired records. `POST /agents/{n}` with a new `ttl` extends.
2. Port lease persistence in `angee.yaml.agents.<name>.ports` (already
   plumbed in Phase 5 — this phase formalizes the schema and adds the
   release on destroy).
3. `--purge` semantics across `agent destroy` and `stack destroy` (worktrees,
   persist dirs, chained `.angee/` only when explicit).
4. Documentation pass: rewrite `docs/USAGE.md`, `docs/OPERATOR.md`,
   `docs/ARCHITECTURE.md`. Old `docs/REFACTOR.md` becomes obsolete and is
   archived under `_deprecated/docs/`.

## Out of Phase 1–8 (deferred)

- Kubernetes runtime backend.
- Durable workflow engine.
- ReBAC / SpiceDB authorization.
- Per-template "pretty" CLI flags (overview leaves `cli_flag:` reserved).
- Web UI.
- Persistence of `/operations/{id}` across operator restarts.
- Workspace as a top-level CLI noun.
- Framework-specific dispatch (Django, Vite, pnpm, uv).

## Cherry-picking guide (per package)

These are the parts of `_deprecated/` likely worth copying forward, with the
understanding that **schemas changed** (new substitution grammar, new
manifest, two-pass secret handling), so most files need rewriting:

| Old path | Likely fate |
|---|---|
| `_deprecated/internal/config/angee.go` | Reference for field names; rewritten in `internal/manifest/` because the schema diverges (no `state/`, no `operator.yaml`, new `runtime:` discriminator on services, new `${ns.path}` grammar). |
| `_deprecated/internal/compiler/compose.go` | Reference for the docker-compose mapping; rewritten in `internal/runtime/compose/` to emit `${ENV_VAR}` references in the compose file and to do the two-pass secret split. |
| `_deprecated/internal/operator/server.go` + `handlers.go` | Reference for routing layout; rewritten in `internal/operator/` against the new API surface (overview §Operator HTTP + MCP API). |
| `_deprecated/internal/runtime/compose/backend.go` | Useful as-is for the `docker compose` shell-out; the surrounding interface is new. |
| `_deprecated/internal/copier/template.go` | Replaced. The new `internal/copierx/` is a thin wrapper over `github.com/fyltr/copier-go` (already at `../copier-go`). |
| `_deprecated/internal/git/` | Likely usable as-is for the `git worktree add` shim. Audit for `os/exec` quoting. |
| `_deprecated/internal/service/platform.go` + `provision.go` | Heavy reference for the orchestration order. Most of `provision.go`'s logic gets reborn inside `service.Platform.AgentInit`'s 12-step sequence, but the new code is structured around the v2 manifest, not v1's. |
| `_deprecated/internal/service/local_runtime.go`, `_deprecated/internal/dev/` | Replaced. Local-process supervision goes through `process-compose`'s REST API in `internal/runtime/proccompose/`; we no longer maintain our own supervisor or TUI. |
| `_deprecated/templates/components/` | Folded into stack templates. The `add` flow is gone — components are now declared by stack templates that the user picks at `stack init`. |
| `_deprecated/templates/default/` | Replaced by `refactor/examples-v2/templates/stacks/angee-notes-dev/`. |

## Decision log (commitments from OVERVIEW-v2)

The plan honors these design commitments without re-litigating them:

- One operator process per `ANGEE_ROOT` for HTTP/MCP serving.
- Per-root advisory lock for reconciliation; chained-stack reconciliation
  runs as library calls inside the same process, each taking the lock for
  its target root.
- Resolved secrets never enter committed files. The compiler refuses to
  write a resolved value into a non-gitignored path.
- `angee up` is compose-only. `angee dev` is compose + process-compose +
  in-process operator.
- `service.Platform` is the only business-logic layer; CLI/HTTP/MCP are
  thin adapters.
- Templates are Copier templates rendered in-process by `copier-go` (no
  Python runtime).
- Workspace is not a CLI noun.
- No backward-compatibility branches in active code. Old paths live in
  `_deprecated/` and are not imported.
