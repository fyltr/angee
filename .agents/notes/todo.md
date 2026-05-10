# Angee Operator TODO

Follow-ups from the GitOps topology/operator manager pass.

## GitOps API

- [ ] Document the GraphQL GitOps API in `docs/OPERATOR-API.md`:
  `gitOpsTopology`, `workspaceSourceFetch`, `workspaceSourcePull`, and
  `workspaceSourcePush`.
- [ ] Add a real-time topology update channel so clients can observe
  workspace/source divergence and convergence without manual refresh.
- [ ] Add conflict detection for workspace source pull/merge paths before
  exposing higher-level "bring together" flows.
- [ ] Add diff metadata for GitOps links: changed files, staged/unstaged
  counts, and commit range summaries.
- [ ] Add safe convergence operations beyond fetch/pull/push: merge, rebase,
  abort/continue when supported, and explicit branch publish flows.

## Operator Hardening

- [ ] Add integration tests for behind and diverged worktree states.
- [ ] Add integration tests for dirty worktrees blocking pull/push.
- [ ] Add coverage for missing workspace source paths and undeclared source
  references inside `gitOpsTopology`.
- [ ] Decide whether topology refresh should be polling, SSE, GraphQL
  subscription, or operator event stream.
