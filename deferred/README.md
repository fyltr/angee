# Deferred Archive

This directory is for reference material that has been removed from the active code path during the zero-backward-compatibility refactor.

Rules:

- Nothing in this directory is part of the build, tests, CLI, operator, API, or template system.
- Do not import code from here.
- Do not add buildable Go files here. If a code snapshot is useful, use `.go.deferred`, `.txt`, or Markdown fenced code.
- Prefer short design notes over full source copies when Git history is enough.
- Move material back into active packages only by rewriting it to match the current `angee.yaml`, Copier, and operator-owned provisioning model.

The goal is to preserve useful ideas without keeping unused flags, commands, package dependencies, framework adapters, or compatibility paths in the active repository structure.
