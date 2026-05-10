---
layout: home

hero:
  name: Angee
  text: Stack manager for agent-native apps
  tagline: One angee.yaml, compiled to Docker Compose and process-compose. CLI, REST, and GraphQL — all dispatching through the same control plane.
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/fyltr/angee

features:
  - title: One manifest, two runtimes
    details: Container services compile to Docker Compose. Local-process services compile to process-compose. The same angee.yaml drives both.
  - title: Workspaces & sources
    details: Spin up isolated workspaces from Copier templates, with materialized git sources, scoped ports, and per-workspace lifecycle.
  - title: CLI / REST / GraphQL parity
    details: The operator exposes the same surface area through three transports. Use the CLI locally, point it at a remote operator, or drive everything from GraphQL.
  - title: Secrets backends
    details: env-file by default; OpenBao for production. Resolved values land in run/secrets.env and substitute into compose at compile time.
---
