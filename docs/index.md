---
layout: home

hero:
  name: Angee
  text: Self-managed stack manager for agent-native apps
  tagline: A Go CLI and operator that pulls source repositories, composes them into Workspaces for development, and compiles them into production Stacks — all from one declarative manifest.
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Concepts
      link: /guide/concepts
    - theme: alt
      text: View on GitHub
      link: https://github.com/fyltr/angee

features:
  - title: GitOps over Sources
    details: Declare git or local Sources in angee.yaml. Angee fetches, caches, and worktrees them — the same set of repositories drives both your Workspaces and your production Stack.
  - title: Workspaces for development
    details: Render a Copier template, materialize Sources on a per-feature branch, allocate ports, and bring up an isolated stack to develop, test, or run a persistent agent — without touching production.
  - title: Stacks for deployment
    details: One angee.yaml compiles to docker-compose for container Services and process-compose for local Services. The operator deploys, restarts, and tails logs through the same control plane.
  - title: Engine, not application
    details: Angee-go is the engine — generic Stack, Source, Workspace, Service, and Job primitives. Application runtimes like angee-django plug in as Hosts on top.
---
