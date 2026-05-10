# Getting started

Angee is a self-managed stack manager for agent-native applications. It
compiles one `angee.yaml` manifest into runtime files (Docker Compose for
container services, process-compose for local processes), resolves secrets,
manages services and jobs, provisions workspaces, and exposes the same
control plane through the CLI plus REST and GraphQL operator APIs.

## Install

From a release:

```sh
curl -fsSL https://angee.ai/install.sh | sh
```

From a checkout:

```sh
make install
```

`make install` builds `dist/angee` and `dist/angee-operator`, then runs
`scripts/install.sh` against those local binaries. Set `ANGEE_INSTALL_DIR`
to install somewhere other than `/usr/local/bin`.

## Quick start

Angee needs an `angee.yaml` in the selected `ANGEE_ROOT`. By default the CLI
walks upward from the current directory, or uses the checkout's `.angee` for
dev checkouts that contain `templates/workspaces`.

```sh
angee doctor
angee status
angee up
angee dev
```

`angee init --dev --yes` is supported when a `dev` stack template is
available through the template search paths.

## Where to next

- [Commands](/guide/commands) — every CLI subcommand and flag.
- [Manifest](/guide/manifest) — `angee.yaml` schema and patterns.
- [Templates](/guide/templates) — Copier templates for stacks and workspaces.
- [Operator API](/reference/operator-api) — REST + GraphQL transport details.
- [Surface parity](/reference/surfaces) — which Platform methods are exposed
  on which surface.
