# Templates

Stack and workspace templates are Copier templates rendered by
`github.com/fyltr/copier-go`. A template must contain `copier.yml` with
Angee metadata under `_angee`.

## Kinds

```yaml
_angee:
  kind: stack
  name: dev
```

```yaml
_angee:
  kind: workspace
  name: pr
```

`angee stack init <template>` resolves stack templates. `angee workspace create
<template>` resolves workspace templates.

## Local Resolution

For a short name like `dev`, stack resolution looks for `stacks/dev`. Workspace
resolution looks for `workspaces/dev`.

Current local search includes:

```text
$ANGEE_ROOT/.templates/<kind>/<name>
$ANGEE_ROOT/templates/<kind>/<name>
$ANGEE_ROOT/<kind>/<name>
$ANGEE_ROOT/<name>
ancestor/.templates/<kind>/<name>
$PWD/.templates/<kind>/<name>
$PWD/templates/<kind>/<name>
ancestor-of-PWD/.templates/<kind>/<name>
```

`<kind>` is `stacks` or `workspaces`.

`angee init --dev` requires a local or remote `stacks/dev` template.

## Remote Resolution

HTTP(S) GitHub URLs are supported. The URL must include owner, repo, and a
template path.

```sh
angee stack init https://github.com/example/templates/tree/main/.templates/stacks/dev
angee workspace create https://github.com/example/templates/tree/main/.templates/workspaces/pr
```

The resolver clones the repository into the user cache, checks out the
requested branch or `?ref=`, and renders the template path.

## Metadata

Workspace templates may declare inputs, sources, chained stack templates, port
allocations, and persistent paths:

```yaml
_angee:
  kind: workspace
  name: pr
  instance_naming:
    pattern: "${inputs.branch | slug | truncate(40)}"
  inputs:
    branch:
      type: string
      required: true
  sources:
    app:
      source: app
      mode: worktree
      branch: "${inputs.branch}"
      subpath: app
  chain_root: stack
  chain_lifecycle: auto
  chain:
    - template: stacks/dev
      root: stack
  ensure:
    operator.port_pool.workspace:
      range: "8100-8199"
  persist:
    browser-data:
      subpath: .browser-data
      scope: workspace
```

Stack templates use the same Copier rendering path and must produce an
`angee.yaml` under the initialized stack root.
