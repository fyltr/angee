# Gaps Surfaced By These Templates

Status updated to reflect the design decisions made during review.

## ✅ Resolved

### 1. Shorthand-input mechanism — design pinned + grammar locked
`_angee.inputs` and `_angee.instance_naming` are in OVERVIEW-v2. Substitution grammar is fully specified in `SUBSTITUTIONS.md`: single `${...}`, `.` as namespace separator, `|` for filters, curated 8-filter stdlib.

### 2. Nested stack provisioning → **template chaining**
No child operator. Agent templates declare `chain:` entries; after the worktree is materialized, the operator runs further provisioning passes inside the worktree using the **same in-process operator code path**. Lifecycle triggers tie chained-stack runtime to the outer agent's lifecycle.

### 3. Secrets backend — `.env` or OpenBao, host env is bootstrap
Host env imported via `import: env:VAR_NAME` on first run. Backend is durable. `generated: true` secrets live in the backend.

### 4. Per-agent operator tokens → just a generated secret
No auth subsystem. Generated secret declared in the agent's `secrets:` block, substituted by Copier into `.mcp.json` and `.env`. Operator validates incoming bearers and scopes to the agent's namespace.

### 5. `${connector.provider.user}` resolver — keep it
External connector resolution stays. Django registers as the resolver.

### 6. Resource explosion — out of scope for v1
Acknowledged. `inner_stack.share` and quotas deferred until pain motivates them.

### 7. PR creation — agent uses git directly
No Angee PR API. Agent runs `git`, `gh pr create`, etc. with credentials inherited from the operator host.

### 8. CLI shape — fixed, template-blind, `--input k=v` only
No per-template "pretty" flags in v1. Canonical shape: `angee agent init <template> [--input k=v ...] [--name explicit]`. Pretty flags can be revisited later as an opt-in addition.

### 9. `${service.name}` substitution — added, with host rewrite rule
Address resolution rewrites the host portion based on where the consumer runs (host process / container in operator network / container in different network). Same rule for `${operator.url}`. Templates write the reference once; the operator picks the right host per destination.

### a. Worktree-mode contract — sources are a global pool
Top-level `sources:` block in `angee.yaml` is the source pool, like `volumes:` in compose. Agents reference by name (`source: django-angee`) and add per-instance overrides (branch, ref, mode, worktree_path). Multiple agents pointing at the same source share a local clone (`cache_path`) and `git worktree add` distinct worktrees off it. One fetch, many isolated checkouts.

### b. Cross-boundary networking — solved by address resolution
The host-portion rewrite (item 9 above) is the answer. `${operator.url}` and `${service.<name>}` resolve differently in a host-process agent vs. a containerized agent. Templates are unaware.

### c. Template-aware vs. template-blind CLI flags
Decided: template-blind. `--input k=v` only.

### d. Conditional MCP wiring — operator tolerates undeclared servers
The operator skips `mcp_servers: [...]` entries that aren't declared in the same manifest, with a warning. Templates can list/declare freely without keeping the two perfectly aligned.

### e. Persistent paths — template-declared via `persist:` blocks
No hardcoded `.playwright-data` or any other path. Templates declare per-service/per-MCP persistent dirs:
```yaml
persist:
  browser-data: { subpath: .playwright-data, scope: agent }
```
Operator creates them on first start, exposes the absolute path as `${persist.<key>}`, preserves across restarts/updates, removes on `destroy --purge`.

## ⏸️ Deferred (locked-in main functionality first)

### f. `.copier-answers.yml` discovery on `update`
Three answer-file locations exist. `angee stack update` / `angee agent update` need to find the right one. Auto-discovery from the active template path is the obvious approach but not specified yet. **Deferred — `update` semantics aren't blocking v1 main flow.**

### g. Resource refresh on `agent update`
What does `agent update` re-do exactly (re-render scaffold, re-fetch worktree branch tip, re-run chained stack init)? **Deferred — same reason; main `init / start / stop / destroy` flow doesn't need it.**
