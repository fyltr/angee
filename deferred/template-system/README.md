# Legacy Template System Archive

Removed from active code during the zero-backward-compatibility refactor.

Deleted active pieces:

- `internal/tmpl/**`
- `templates/default/**`
- `.angee-template.yaml` metadata handling
- Go `text/template` rendering for stack init

The target system uses Copier templates with Angee metadata under `_angee`. Use Git history if source details are needed while rebuilding template resolution and rendering on the new model.
