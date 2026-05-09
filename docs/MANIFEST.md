# Manifest

Angee reads one manifest at `$ANGEE_ROOT/angee.yaml`.

Minimal shape:

```yaml
version: 1
kind: stack
name: example

services:
  web:
    runtime: container
    image: nginx:alpine
    ports:
      - "8080:80"
```

## Top-Level Fields

```yaml
version: 1
kind: stack
name: example
template: {}
operator: {}
secrets_backend: {}
secrets: {}
ports: {}
volumes: {}
sources: {}
workspaces: {}
services: {}
jobs: {}
port_leases: {}
```

`version`, `kind`, and `name` are required. Empty maps are accepted.

## Operator

```yaml
operator:
  url: http://127.0.0.1:9000
  domain: operator.example.test
  token_secret: operator-token
  port_pool:
    workspace:
      range: "8100-8199"
```

`url`, `domain`, `token_secret`, and `port_pool` are used by substitutions,
workspace allocation, and operator setup.

## Secrets

Env-file backend:

```yaml
secrets_backend:
  type: env-file
  path: .env

secrets:
  django-secret-key:
    generated: true
    length: 48
  github-token:
    import: GITHUB_TOKEN
```

OpenBao backend:

```yaml
secrets_backend:
  type: openbao
  address: http://127.0.0.1:8200
  mount: secret
  token: ${BAO_TOKEN}
```

Secret substitutions use `${secret.name}` in service and job fields.

## Services

Container service:

```yaml
services:
  web:
    runtime: container
    image: nginx:alpine
    command: ["nginx", "-g", "daemon off;"]
    env:
      EXAMPLE: value
    ports:
      - "8080:80"
    mounts:
      - "source://app:/app"
    workdir: /app
    depends_on: [db]
```

Local service:

```yaml
services:
  api:
    runtime: local
    command: ["go", "run", "./cmd/server"]
    env:
      PORT: "${ports.api}"
    workdir: "source://app"
```

Container services require `image` or `build`. Local services require
`command` and must not set `image`.

## Jobs

```yaml
jobs:
  migrate:
    runtime: local
    command: ["go", "test", "./..."]
    workdir: "source://app"
    depends_on: [db]
```

Jobs are run explicitly with `angee job run <name>`.

## Sources

Implemented source kinds:

```yaml
sources:
  app:
    kind: local
    path: ..

  library:
    kind: git
    repo: https://github.com/example/library.git
    default_ref: main
    cache_path: sources/library
```

Git commands use the host git environment.

## Workspaces

Workspace records are usually written by `angee workspace create`.

```yaml
workspaces:
  fix-123:
    template: workspaces/pr
    inputs:
      branch: fix-123
    ttl: 24h
    ttl_expires_at: 2026-05-10T12:00:00Z
```

TTL values are stored and surfaced by status commands.

## Substitutions

Supported namespaces include:

```text
${secret.name}
${service.name.host}
${service.name.port}
${service.name.url}
${ports.name}
${alloc.pool}
${workspace.name.path}
${source.name.path}
${persist.name}
${operator.url}
${operator.domain}
${inputs.name}
${name}
```

Supported filters include `slug`, `lower`, `upper`, `local_part`,
`truncate(n)`, `default(value)`, `required(message)`, `b64encode`, and
`replace(old,new)`.
