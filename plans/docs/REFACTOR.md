# angee-go Refactor Plan

This document consolidates two audits performed against `angee-go`:

1. **DRY audit** of CLI / REST / GraphQL surfaces over `service.Platform`.
2. **Build-vs-reuse audit** of in-house subsystems against current OSS.

It enumerates concrete refactors, prioritized by payoff, with enough detail to
execute each one as an isolated change.

## Audit summary

### DRY across CLI / REST / GraphQL

The architecture stated in `CLAUDE.md` — CLI, REST, and GraphQL all dispatch
through `service.Platform` — is genuinely realized. Business logic is centralized
in `internal/service/`. Real but small violations exist:

- **Surface drift.** Several methods on `Platform` are exposed on only one
  surface. `gitOpsTopology` is GraphQL-only; workspace source `fetch`/`pull`/`push`
  are GraphQL-only; `WorkspaceLogsLimited` and `StackLogsLimited` are GraphQL-only.
  The corresponding REST routes and CLI commands do not exist.
- **`resolveRoot` duplicated.** `internal/operator/operator.go:660-684` and
  `internal/cli/root.go:748-775` reimplement the same walk-up algorithm with
  subtly different fallbacks (CLI also accepts `.templates/workspaces` and
  `templates/workspaces` directories).
- **Inline request structs that should live in `api/`.**
  `JobRunRequest`-shaped struct typed three times; `WorkspaceUpdateRequest`-shaped
  struct typed three times. Wire-contract drift risk.
- **Three near-identical `sorted<Type>s` helpers** in `internal/operator/graphql.go:1199-1250`
  duplicate what generic `sortedKeys` (`platform.go:549`) already does.
- **Service-error → HTTP status mapping is ad-hoc.** Everything flattens to 500 or
  400; `service.StackRootExistsError` and "not declared" errors are not mapped to
  409 / 404. The CLI handles them specifically (`root.go:623-629`).

### Build-vs-reuse

Most of the codebase is correct about what to own vs. delegate. Three swaps are
worth scheduling:

- **GraphQL library** (`graphql-go/graphql` v0.8.1) — last release **April 2018**,
  ~200 open issues, no maintainer activity. Single highest-urgency change.
  `99designs/gqlgen` v0.17.90 (MIT, Apr 2026, 10.7k★) is the right replacement;
  schema-first codegen maps directly onto angee's Platform-forwarding pattern.
- **Compose YAML generation** — text-template strings in `service/templates.go`
  are a spec-drift risk. `compose-spec/compose-go v2` (Apache-2.0, Docker-maintained)
  provides typed structs. Worth ~15 direct deps.
- **Manifest validation** — scattered `if`-block validation in
  `internal/manifest/ensure.go` would consolidate behind `go-playground/validator`
  struct tags. `invopop/jsonschema` then generates a JSON Schema for free → LSP
  completion in `angee.yaml` and CI schema checks.

Things to **definitely keep building**: `internal/substitute/` (domain-specific
namespaced expression language), `fyltr/copier-go` (no Go-native template
scaffolder with `copier update` semantics exists), `internal/ports/` (range-based
named lease allocator; `phayes/freeport` is unmaintained and solves a different
problem), `internal/runtime/compose/` and `internal/runtime/proccompose/` (correct
to shell out to `docker compose` and `process-compose`).

### Decision overrides

The build-vs-reuse audit recommended **keeping** the git CLI wrapper for write
operations because go-git v5 lacks parallel checkout (issue #1956 makes worktree
creation slow on large repos). User decision: **proceed with go-git migration
now** and accept the perf trade-off; revisit if workspace creation latency
becomes a problem.

---

## Refactor backlog

Items are ranked by payoff. Each is independently shippable.

| #   | Item                                            | Priority    | Risk   | Est. LOC delta |
| --- | ----------------------------------------------- | ----------- | ------ | -------------- |
| R1  | Migrate `internal/git/` read-only ops to go-git (DONE; write ops remain CLI) | High | Medium | ±0 |
| R2  | Replace `graphql-go/graphql` with `gqlgen`      | High        | Medium | -800           |
| R3  | Promote inline request structs to `api/`        | High        | Low    | -20            |
| R4  | Hoist `resolveRoot` into shared package         | Medium-High | Low    | -25            |
| R5  | Close REST/CLI parity gap with GraphQL          | Medium-High | Low    | +200           |
| R6  | Centralize service-error → HTTP status mapping  | Medium      | Low    | +30            |
| R7  | Adopt `compose-spec/compose-go v2` for compose  | Medium      | Medium | varies         |
| R8  | Adopt `go-playground/validator` + JSON Schema   | Medium      | Low    | +50            |
| R9  | Collapse `sorted*` helpers in `graphql.go`      | Low         | Low    | -40            |

---

## R1. Migrate `internal/git/` to go-git v5 — read-only done

**Status:** read-only queries migrated; write/network ops deliberately kept on
CLI. See "What was done" below.

**Goal:** replace the CLI wrapper at `internal/git/git.go` (313 LOC) with a
go-git-backed implementation while preserving the existing `Client` API.

### What was done (this PR)

- `go.mod` now has a direct dep on `github.com/go-git/go-git/v5 v5.19.0`.
- `internal/git/git.go` rewritten as a hybrid: `openRepo` helper plus go-git
  implementations for `RefExists`, `CurrentBranch`, `CurrentRef`, `Upstream`,
  `AheadBehind`/`AheadCount`, `Config`, `Remotes`, `Dirty`. `SyncBaseRef` and
  `PushRemote` reuse the new query methods.
- Existing tests in `internal/git/git_test.go` pass unchanged against the
  go-git-backed implementation.
- `make check` passes across the full project.

### What was deliberately NOT migrated

The following methods still shell out to `git` and that is intentional:

- `Clone`, `CloneRef`, `Fetch`, `Pull`, `Push`, `PushSetUpstream` — network
  ops need credential helpers, SSH config, and submodule handling. go-git's
  Auth model would force us to reimplement credential discovery.
- `Merge`, `Rebase` — go-git does not natively implement these.
- `WorktreeAdd`, `WorktreeAddBranch` — go-git lacks parallel checkout
  (go-git/go-git#1956); workspace creation is a hot path.
- `Run` — kept as an escape hatch (used by `templates.go` for `checkout`).

This split is documented in the package doc-comment of `internal/git/git.go`
and is a defensible long-term boundary, not a stopgap.

### Future migration steps (not this PR)

**Why now:** the package already has a clean abstraction surface (`Client`
struct with method receivers); call sites pass through `Client` instances. The
swap is contained. go-git is already an indirect dep via `fyltr/copier-go`.

**Trade-off accepted:** worktree creation may be slower on very large repos
(go-git lacks parallel checkout, issue #1956). For typical workspace creation
this is acceptable. We can fall back to CLI per-method if a hot path turns out
to need it.

### Steps

1. Add direct dep: `go get github.com/go-git/go-git/v5@latest`.
2. Rewrite `internal/git/git.go` keeping the same public surface — `Client`,
   `New()`, and every existing method signature. Internally use go-git APIs:
   - `Run`: keep as-is (some callers may want to invoke arbitrary git commands;
     check call sites — if none, drop it).
   - `Clone` / `CloneRef`: `git.PlainCloneContext` with `ReferenceName` for ref.
   - `Fetch`: `repo.FetchContext` with `RefSpecs` covering all remotes.
   - `Merge` / `Rebase`: **not natively supported by go-git.** Either keep a
     CLI fallback for these two methods, or document that merge/rebase require
     a clean working tree and use go-git's plumbing (low-level objects and
     index manipulation). **Recommendation:** keep CLI fallback for `Merge`
     and `Rebase` only; introduce `cliFallback` field on `Client` for those.
   - `RefExists`: `repo.Reference(plumbing.ReferenceName(ref), true)`.
   - `WorktreeAdd` / `WorktreeAddBranch`: go-git supports linked worktrees via
     the `Worktree` API but the experience is rougher than the CLI. **First
     pass: keep CLI fallback for worktree add.** Migrate later if/when go-git
     parallel checkout lands.
   - `CurrentBranch` / `CurrentRef`: `repo.Head()` + reference name parsing.
   - `Upstream`: read `branch.<name>.remote` + `branch.<name>.merge` from
     repo config (`repo.Config()`).
   - `AheadBehind`: walk commits from base to HEAD via `repo.Log()` with
     `Order: LogOrderCommitterTime`. Or use `Reference` + `Object` traversal.
   - `Config`: `repo.Config()` → look up `Raw.Section(...).Option(...)`.
   - `Remotes`: `repo.Remotes()`.
   - `PushRemote`: same precedence chain (`branch.<n>.pushRemote` →
     `remote.pushDefault` → `branch.<n>.remote` → `origin` → single remote).
   - `Dirty`: `worktree.Status()` → check len.
   - `Pull`: `worktree.PullContext` with `--ff-only` semantics (set
     `Force: false`, ensure no merge commit is created — go-git's pull is
     fast-forward by default if upstream tracks).
   - `Push` / `PushSetUpstream`: `repo.PushContext` with `RefSpecs` and
     `Auth`. Setting upstream requires writing to repo config after push.
3. Auth: where the CLI wrapper relies on the user's `~/.ssh` config or
   credential helper, go-git needs explicit auth. For SSH, use
   `ssh.NewPublicKeysFromFile` defaulting to `~/.ssh/id_ed25519` /
   `~/.ssh/id_rsa`. For HTTPS, fall back to git credential helper via a tiny
   exec.
4. Update `internal/git/git_test.go` (if present) and ensure
   `internal/service/gitops*` tests still pass.
5. Run `make check` and `go test -race ./...`.
6. Smoke-test: from `~/Work/fyltr/angee-django`, run
   `angee workspace create dev-pr-multi --name refactor-git --start` and
   verify worktrees materialize and sync correctly.

### Acceptance (read-only migration)

- [x] All callers of `internal/git/Client` compile unchanged.
- [x] `make check` passes.
- [ ] Smoke test creates a workspace, fetches, pulls, pushes a branch — to be
  run from a real workspace before tagging a release.

---

## R2. Replace `graphql-go/graphql` with `99designs/gqlgen`

**Goal:** delete `internal/operator/graphql.go` (1271 LOC) and replace with a
schema-first gqlgen-generated resolver layer that forwards to `service.Platform`.

**Why:** current lib's last release was 2018 (security exposure), and the
hand-built schema is the largest single file in the repo.

### Steps

1. Add deps: `go get github.com/99designs/gqlgen@latest` and
   `go get github.com/vektah/gqlparser/v2@latest`.
2. Add `gqlgen` tool dep via `tools.go` pattern.
3. Define `internal/operator/schema.graphql` covering all currently-exposed
   queries, mutations, and types. Reference the existing `graphql.go` to enumerate.
4. Add `internal/operator/gqlgen.yml` configuring:
   - `model.bindings` → bind generated models to types in `api/` and
     `internal/service/` where they already exist.
   - `resolver.layout: follow-schema` → one file per top-level type.
5. Run `go run github.com/99designs/gqlgen generate` to produce `generated/`.
6. Implement `Resolver` methods. Each resolver should be a 3-line forward to
   `Platform`: `return r.platform.X(ctx, args)`. No business logic.
7. Wire the generated handler into `operator.go`'s mux at `POST /graphql`,
   replacing the current handler.
8. Delete `internal/operator/graphql.go`.
9. Update `docs/OPERATOR-API.md` GraphQL section.
10. Run query parity tests: hit each query/mutation against both the old and
    new server (during migration) to confirm response shape matches.

### Acceptance

- `internal/operator/graphql.go` deleted.
- All existing GraphQL queries return identical shapes (verified by tests).
- `go.mod` no longer requires `graphql-go/graphql`.
- Subscription support is **out of scope** for this PR; track separately if
  added later.

---

## R3. Promote inline request structs to `api/`

**Goal:** eliminate three duplicated request shapes.

### Steps

1. In `api/types.go`, add:
   - `JobRunRequest{ Inputs map[string]string }`.
   - `WorkspaceUpdateRequest{ Inputs map[string]string; TTL string }`.
2. Replace inline declarations:
   - `internal/operator/operator.go:295-303` → use `api.JobRunRequest`.
   - `internal/operator/operator.go:486-498` → use `api.WorkspaceUpdateRequest`.
   - `internal/cli/operator_client.go:192-196` → use `api.JobRunRequest`.
   - `internal/cli/operator_client.go:270-280` → use `api.WorkspaceUpdateRequest`.
3. Verify JSON tags match the wire format already used (no breaking change).

### Acceptance

- `grep -n 'struct {' internal/operator/operator.go internal/cli/operator_client.go`
  shows no remaining inline request structs that mirror existing API types.
- Tests pass.

---

## R4. Hoist `resolveRoot` into a shared package

**Goal:** one `ResolveRoot` implementation used by both operator and CLI.

### Steps

1. Add `internal/manifest.ResolveRoot(start string) (string, error)`.
   Behavior: walk up from `start`, accept any directory containing `angee.yaml`
   OR a directory that contains `templates/workspaces` or `.templates/workspaces`
   (preserving CLI's broader detection).
2. Replace `internal/operator/operator.go:660-684` with a call to
   `manifest.ResolveRoot`.
3. Replace `internal/cli/root.go:748-775` with a call to `manifest.ResolveRoot`.
4. Add unit tests covering both stack-rooted and template-rooted detection.

### Acceptance

- Both call sites delegate to the shared function.
- A directory containing only `templates/workspaces/` is accepted as a root by
  both surfaces (matching current CLI behavior).

---

## R5. Close REST/CLI parity gap with GraphQL

**Goal:** every Platform method is reachable from all three surfaces, or
explicitly documented as one-surface-only.

### Methods missing on REST + CLI

- `Platform.GitOpsTopology` → `GET /gitops/topology` + `angee gitops topology`.
- `Platform.WorkspaceSourceFetch/Pull/Push` →
  `POST /workspaces/{name}/sources/{slot}/{op}` + `angee workspace source <op>`.
- `Platform.WorkspaceLogsLimited` and `StackLogsLimited` →
  `GET /workspaces/{name}/logs?limit=N` and `GET /stack/logs?limit=N` +
  `--limit` flag on existing `angee logs` commands.

### Steps

1. Add REST routes in `internal/operator/operator.go`.
2. Add CLI commands and `platformClient` interface methods in
   `internal/cli/operator_client.go` and `internal/cli/root.go`.
3. Update `docs/COMMANDS.md` and `docs/OPERATOR-API.md`.

### Acceptance

- For every method on the `platformClient` interface, there is a REST endpoint
  and a CLI command (or a documented justification for the omission).

---

## R6. Centralize service-error → HTTP status mapping

**Goal:** REST and GraphQL return meaningful status codes / error categories.

### Steps

1. In `internal/service/`, ensure all sentinel errors are typed (or wrapped
   with `errors.Is`-checkable values): e.g., `ErrStackRootExists`,
   `ErrNotDeclared`, `ErrServiceNotFound`.
2. Add `internal/operator/errs.go` with `writeServiceError(w, err)` mapping:
   - `errors.Is(err, service.ErrStackRootExists)` → 409 Conflict.
   - `errors.Is(err, service.ErrNotDeclared)` → 404 Not Found.
   - default → 500.
3. Update REST handlers to call `writeServiceError` instead of `writeError`
   when the error originates from `Platform`.
4. Add a parallel `formatGraphQLError` for the gqlgen resolvers (after R2).
5. Update CLI to surface specific errors uniformly via `errors.Is` rather than
   string matching (replaces `root.go:623-629` ad-hoc handling).

### Acceptance

- Existing CLI behavior unchanged.
- REST returns 409 for stack-root-exists and 404 for not-declared.

---

## R7. Adopt `compose-spec/compose-go v2` for compose YAML generation

**Goal:** replace text-template-based compose generation with type-safe struct
marshaling.

### Steps

1. Add dep: `go get github.com/compose-spec/compose-go/v2@latest`.
2. In `internal/service/templates.go` (or wherever `docker-compose.yaml` is
   produced), construct `types.Project` and `types.ServiceConfig` from the
   manifest, then marshal via `project.MarshalYAML()`.
3. Remove the corresponding text template.
4. Add a unit test that compares the generated compose YAML against a golden
   file for a representative manifest.
5. Audit dep weight: confirm the new transitive deps (logrus, mapstructure,
   jsonschema) are acceptable.

### Acceptance

- `docker-compose.yaml` output is byte-for-byte equivalent (or
  semantically equivalent and documented) for existing test fixtures.
- All compose-related tests pass.

### Caveats

- compose-go v2 brings ~15 direct deps. Re-evaluate dep policy before merging.
- Process-compose generation is **not** part of this swap; keep its
  text-template generation (process-compose has no equivalent typed library).

---

## R8. Adopt `go-playground/validator` + `invopop/jsonschema`

**Goal:** consolidate manifest validation into struct tags; emit a JSON Schema.

### Steps

1. Add deps:
   - `go get github.com/go-playground/validator/v10@latest`.
   - `go get github.com/invopop/jsonschema@latest`.
2. In `internal/manifest/`, annotate manifest types with `validate:"..."` tags
   (e.g., `validate:"required,oneof=container local"` on `runtime`).
3. Replace ad-hoc `if`-block validation in `internal/manifest/ensure.go` (or
   wherever it lives) with `validator.Validate(m)`.
4. Add `make schema` target that emits `docs/angee.schema.json` from the
   manifest types via `invopop/jsonschema`.
5. Document the schema in `docs/MANIFEST.md` and reference it for editor LSP
   integration (e.g., `# yaml-language-server: $schema=...`).

### Acceptance

- Existing valid manifests still validate; existing invalid manifests still
  fail with equivalent or better messages.
- `docs/angee.schema.json` is generated and matches the structs.
- Editors with `yaml-language-server` get autocompletion against the schema.

---

## R9. Collapse `sorted*` helpers in `graphql.go`

If R2 (gqlgen migration) ships first, this becomes moot — gqlgen generates
ordered fields. Otherwise:

- Replace `sortedServiceStates`, `sortedJobStates`, `sortedWorkspaceRefs`
  (`internal/operator/graphql.go:1199-1250`) with a single generic helper using
  `sortedKeys` from `platform.go:549`.

---

## Sequencing

Recommended order:

1. **R1** (go-git) — isolated, exercise the abstraction.
2. **R3** (api types) — trivial, removes drift risk.
3. **R4** (resolveRoot) — trivial, removes duplication.
4. **R5** (parity gap) — unblocks single-surface migrations.
5. **R6** (error mapping) — improves UX, prereq for R2's GraphQL errors.
6. **R2** (gqlgen) — large but well-bounded; consumes R6's error helpers.
7. **R8** (validator + JSON Schema) — independent, ship anytime.
8. **R7** (compose-go) — last, given dep weight needs deliberation.
9. **R9** — only if R2 doesn't happen first.

Ship each as its own PR. None of them require coordinated lockstep changes.

## Out of scope

- Migrating to `openbao/api/v2` — defer until functionality exceeds the four
  KV ops the hand-rolled client supports.
- Replacing `phayes/freeport` (unmaintained) — angee's port pool solves a
  different problem (range-based named leases) and `phayes/freeport` is not a
  candidate.
- Replacing `internal/substitute/` — namespaced filter pipeline is core
  domain; no library models it.
- Replacing `fyltr/copier-go` — no Go-native alternative exists.

## References

- DRY audit: this plan, "Audit summary" section above.
- Build-vs-reuse audit sources:
  - https://github.com/compose-spec/compose-go
  - https://github.com/99designs/gqlgen
  - https://github.com/graphql-go/graphql (last release Apr 2018)
  - https://github.com/go-git/go-git
  - https://github.com/go-git/go-git/issues/1956 (worktree perf)
  - https://github.com/go-playground/validator
  - https://github.com/invopop/jsonschema
