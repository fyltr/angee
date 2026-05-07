# Templates

Angee templates are Copier templates that produce or update Angee resources.

## Kinds

```text
templates/
  stacks/<name>/
  workspaces/<name>/
  agents/<name>/
```

Commands:

```sh
angee stack init dev
angee stack init staging-docker
angee workspace init feat-x --template workspaces/feature-dev
angee agent init feat-x --template agents/angee-developer
```

`angee init` is shorthand for the default stack init.

## What A Template Does

A template can render:

- `$ANGEE_ROOT/angee.yaml`.
- Project helper files.
- Workspace instructions such as `AGENTS.md`.
- Agent runtime config such as `opencode.json`.
- Deployment backend files when the stack uses a generated backend file.

After Copier renders files, the operator performs Angee-specific work:

1. Validate `_angee` metadata.
2. Materialize sources.
3. Resolve secrets.
4. Allocate port leases.
5. Create volumes and state directories.
6. Provision services, jobs, workflows, workspaces, agents, and MCP servers.

## Template Metadata

Templates use normal Copier questions for user answers and `_angee` metadata for Angee resources.

Example:

```yaml
_min_copier_version: "9.0"
_subdirectory: template
_templates_suffix: .jinja

_angee:
  schema: 1
  kind: stack
  name: dev
  port_leases:
    - { name: web, default: 8100, band: web, export_env: WEB_PORT }
  sources:
    - { name: app, kind: local, ref: current, tree: ., target: . }
  secrets:
    - { name: app-secret-key, generated: true, length: 50 }

project_name:
  type: str
  default: "{{ _folder_name }}"
```

Port leases should be named resources. Do not create separate questions such as `web_port` for every port. Users override named leases with `--port web=8120`.

## Updates

Use noun-first update commands:

```sh
angee stack update
angee workspace update feat-x
angee agent update feat-x
```

Copier owns three-way file updates. Angee owns sources, secrets, port leases, runtime state, and reconciliation.
