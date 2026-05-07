# Project-Mode Archive

Removed from active code during the zero-backward-compatibility refactor.

Deleted active pieces:

- `cli/init_runtime.go`
- `cli/project.go`
- `cli/build.go`
- `cli/migrate.go`
- `cli/doctor.go`
- `cli/fixtures.go`
- `internal/projmode/**`
- `internal/dev/**`
- root `pyproject.toml` project-mode example

These implemented the old `.angee/project.yaml` parent-walk mode, Django adapter, `manage.py` dispatch, `pyproject.toml` process discovery, and framework-specific dev orchestrator.

The target model replaces this with `angee.yaml`, template-declared services/jobs/workflows, and operator-owned provisioning/reconciliation. Use Git history if source details are needed while rebuilding equivalent ideas on the new model.
