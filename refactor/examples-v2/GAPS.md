# Gaps Surfaced By These Templates

Status: updated for the agent-free v2 design. These notes track decisions the
examples rely on, plus deferred implementation details.

## Resolved

### 1. Shorthand-input mechanism

`_angee.inputs` and `_angee.instance_naming` are pinned in `OVERVIEW-v2.md`.
The grammar is specified in `SUBSTITUTIONS.md`: single `${...}`, `.` as the
namespace separator, `|` for filters, and a curated filter set.

### 2. Template chaining

No child operator process. Workspace templates declare `chain_root:` and any
chained templates. After source materialization, the operator runs further
stack prepare/reconcile calls against the inner `ANGEE_ROOT` using the same
`service.Platform` library path and per-root lock.

### 3. Secrets backend

Host env is only bootstrap input for `import: env:VAR_NAME`. The configured
backend is durable. `generated: true` secrets live in `.env` or OpenBao.

### 4. Operator tokens

No per-agent auth subsystem. Operator access is a normal generated secret
(`operator-token`) and bearer auth is enforced by the operator API. Narrower
service-scoped tokens can be added later if a real caller needs them.

### 5. Connector resolver removal

`${connector.*}` is dropped from Angee. Applications that need per-user OAuth
or connector credentials resolve them themselves and pass values as normal
inputs or secrets.

### 6. PR creation

No Angee PR API. Angee makes sure worktree-backed sources can fetch, status,
pull, and push. Opening a PR is the application or user's job.

### 7. CLI shape

Template-backed commands are template-blind and use repeatable
`--input k=v`: `angee stack init <template>` and `angee workspace create
<template>`. `service init` is not template-backed in v1; it accepts the
small service-spec flags directly.

### 8. Worktree-mode contract

Top-level `sources:` is the global source pool, like compose `volumes:`.
Workspaces reference source names and add per-instance overrides such as
branch, ref, mode, and subpath. Multiple workspaces share one local clone and
get distinct git worktrees from it.

### 9. Cross-boundary networking

`${operator.url}` and `${service.<name>}` rewrite the host portion based on
where the consumer runs: host process, container in the same network, or
container crossing back to the host.

### 10. Persistent paths

Templates declare persistent directories with `persist:` blocks. The operator
creates them, exposes `${persist.<key>}`, preserves them across workspace
start/stop/update, and removes them on `workspace destroy --purge`.

## Deferred

### 1. `.copier-answers.yml` discovery on update

`angee stack update` and `angee workspace update` need a precise rule for
finding the active answers file. Auto-discovery from `template.active` is the
likely path, but it is not blocking the create/start flow.

### 2. Workspace update refresh semantics

`workspace update` still needs exact semantics: re-render scaffold only,
re-fetch source branch tips, re-run chained stack prepare, or some subset.
The main `create/start/stop/destroy` flow does not depend on this yet.
