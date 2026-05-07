# Operator Surface Cleanup Archive

Removed from active code during the zero-backward-compatibility refactor.

Deleted active pieces:

- unimplemented `angee agent chat` and `angee agent ask` CLI/API/HTTP/service stubs
- unused MCP credential dispatch stubs
- deploy `--message` request plumbing that no longer had a commit target
- unused git convenience helpers and branch checkout helpers
- unused host-local operator config fields such as `django_url` and Docker socket config
- unused `ReconcileRequest.follow` field from the old log-follow shape

Why deferred:

- Chat/ask may return later, but should be implemented against the current agent runtime contract instead of a 501 stub.
- Credential management may return later, but connectors are application-managed and should not be a fake operator endpoint.
- Deploy messaging may return later only if deploy creates a concrete state/history record.
- Git branch helpers may return later only where active source/workspace workflows require them.

Files:

- `agent-chat-ask.go.deferred` preserves the removed chat/ask CLI/API/HTTP/service snippets.
- `git-helpers.go.deferred` preserves removed unused git helper snippets.
- `mcp-credentials.md` preserves the removed MCP credential stubs and future design notes.
- `operator-and-deploy-notes.md` preserves deploy message, operator config, and reconcile flag notes.

Rules:

- This directory is reference-only.
- Do not import from here.
- Do not move snippets back verbatim; rewrite them against the current `$ANGEE_ROOT/angee.yaml` and operator-owned provisioning model.
