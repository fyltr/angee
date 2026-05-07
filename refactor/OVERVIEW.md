# Angee Refactor Overview

Status: target clean design, informed by the current Go codebase.

This document describes the streamlined version of Angee we want to build. It is not a backward-compatibility plan. Old command shapes, legacy manifest files, framework-specific dispatch, and unused compatibility branches should be deleted rather than carried forward.

## What Angee Is

Angee is a self-managed orchestration layer for application stacks and AI agents.

It gives a project one control directory, `ANGEE_ROOT`, and one initial file-backed stack source of truth, `angee.yaml`. From that manifest Angee can provision services, jobs, agents, MCP servers, service ports, agent workspaces, sources, volumes, and generated runtime files such as `docker-compose.yaml`.

The core promise is simple:

```sh
angee init --dev --yes
angee dev
```

For staging or another environment, the user should change the template, not the mental model. `init` creates or updates the manifest; runtime power is managed separately with `up`, `down`, `start`, and `stop`:

```sh
angee stack init staging-docker --yes --set domain=staging.example.com
angee up
angee down
angee up
```

For agent work, the user provisions an agent. The agent init/update flow creates or updates the agent-owned workspace definition and materializes it into that agent's derived workspace folder:

```sh
angee agent init reviewer \
  --template agents/angee-developer \
  --source app=https://github.com/fyltr/app#feature-x \
  --start \
  --yes
angee agent stop reviewer
angee agent start reviewer
angee agent destroy reviewer
```

## Product Shape

Angee has six core concepts.

| Concept | Meaning |
|---|---|
| Stack | The whole environment: services, jobs, agents, agent workspaces, secret declarations, service ports, sources, volumes, and runtime settings. |
| Service | A runnable workload managed by Docker Compose in v1. |
| Workspace | An agent-owned source/mount definition plus the agent's materialized workspace folder. It has no standalone CLI lifecycle and no global materialized folder. |
| Source | Content that can come from a template, git repository, URL/archive, local path, or volume and be placed at a workspace subpath. |
| Agent | A composite resource bundle: one service plus one materialized copy of a workspace under `agents/<name>/workspace`. Its runtime lifecycle is the same as the bundled service lifecycle. |
| Operator | The reconciler. It renders templates, updates `angee.yaml` in place, syncs file and database backends when configured, compiles runtime files, and applies runtime changes. |

Everything goes through the operator path. The CLI is only a user interface and transport adapter.

The default source-of-truth backend is the file backend: `$ANGEE_ROOT/angee.yaml`. A database can participate through `angee-operator`, including bidirectional sync, but direct database writes must not bypass the operator's validation, version checks, and conflict handling.

## Current Code Bearings

The current code already points in the right direction:

| Area | Current files | Keep the idea |
|---|---|---|
| Shared API types | `api/types.go` | CLI, HTTP, MCP, and app clients should share request/response contracts. |
| Local vs remote dispatch | `cli/client.go` | Use in-process `service.Platform` by default; use HTTP only with `--operator` or `ANGEE_OPERATOR_URL`. |
| Business core | `internal/service/platform.go` | Keep one service layer that knows nothing about Cobra or HTTP. |
| Provisioning | `internal/service/provision.go` | Stack and agent init/update belong behind the operator. Workspace materialization is part of agent provisioning, not a standalone lifecycle. |
| Templates | `internal/copier/template.go` | Copier should render and update stack and agent templates. Workspace templates can produce source content consumed by agent provisioning; agent templates produce runtime scaffold. |
| Sources, secrets, ports | `internal/provision/*` and related code | Keep operator-owned source materialization. Materialize workspace sources only into agent-derived workspace folders. Move concrete service ports into `angee.yaml`; put secret values in `.env` or OpenBao. |
| Runtime backend | `internal/runtime/*` | For the clean v1, keep one Docker Compose runner. Preserve interfaces only where they simplify code, not to design for Kubernetes yet. |
| Docker Compose compiler | `internal/compiler/compose.go` | Compose is the v1 runtime target and should stay generated from the rendered template/`angee.yaml` model. |
| HTTP/MCP operator | `internal/operator/*` | External runtimes can manage Angee through operator APIs. |

The clean refactor should remove anything that bypasses these paths.

## Clean Command Surface

Use noun-first commands. Keep top-level aliases only when they are obvious shortcuts.

```sh
angee init --dev [path]                    # shortcut for: angee stack init dev [path]
angee init stack <template-name> [path]     # explicit noun form
angee init agent <name>
angee stack init <template-name> [path]
angee stack update
angee stack destroy

angee dev [--only name] [--except name] [--ui lines|panes]
angee up [service...]                     # docker compose up -d
angee down                                # docker compose down
angee start [service...]                  # docker compose start
angee stop [service...]                   # docker compose stop
angee restart [service...]                # docker compose restart
angee pull
angee plan
angee status                              # alias: ls, ps
angee logs [service...]                   # docker compose logs
angee service init <name>
angee service update <name>
angee service list
angee service start <name>
angee service stop <name>
angee service restart <name>
angee service logs <name>
angee service destroy <name>

angee job run <name>
angee job list
angee job logs <name>

angee agent init <name>
angee agent update <name>
angee agent start <name>
angee agent stop <name>
angee agent restart <name>
angee agent logs <name>
angee agent destroy <name>
angee agent list

angee operator                            # visible standalone operator command
```

Target rule: every command either calls the in-process operator runtime or an explicitly configured remote operator. No command should render templates, inspect Django, run `manage.py`, call pnpm, or manage containers directly outside the operator service layer and its shared Compose runner.

## Init, Up, And Down

There is no first-class `deploy` command in the clean model. It is ambiguous because it mixes environment definition changes with runtime power.

| Command | Meaning | Backend analogy |
|---|---|---|
| `angee stack init <template>` | Provision a stack manifest from a template. It creates `ANGEE_ROOT`, renders `angee.yaml`, resolves non-runtime values, and records template answers. | `copier copy` plus Angee manifest initialization. |
| `angee stack update` | Update the stack manifest from the active stack template. | `copier update` plus Angee manifest reconciliation. |
| `angee up [service...]` | Start or reconcile the runtime from the current `angee.yaml`. | `docker compose --project-directory $ANGEE_ROOT -f $ANGEE_ROOT/docker-compose.yaml up -d [service...]`. |
| `angee down` | Stop or remove the stack runtime while preserving `ANGEE_ROOT`, templates, `angee.yaml`, secrets, service port assignments, and persistent volumes by default. | `docker compose --project-directory $ANGEE_ROOT -f $ANGEE_ROOT/docker-compose.yaml down`. |
| `angee start [service...]` | Start existing stopped containers without recreating them. | `docker compose --project-directory $ANGEE_ROOT -f $ANGEE_ROOT/docker-compose.yaml start [service...]`. |
| `angee stop [service...]` | Stop running containers without removing them. | `docker compose --project-directory $ANGEE_ROOT -f $ANGEE_ROOT/docker-compose.yaml stop [service...]`. |
| `angee restart [service...]` | Restart running containers. | `docker compose --project-directory $ANGEE_ROOT -f $ANGEE_ROOT/docker-compose.yaml restart [service...]`. |
| `angee logs [service...]` | Stream or print runtime logs. | `docker compose --project-directory $ANGEE_ROOT -f $ANGEE_ROOT/docker-compose.yaml logs [service...]`. |
| `angee service start <name>` | Start one declared service. | Same shared Compose runner with `<name>` as the service argument. |
| `angee service stop <name>` | Stop one declared service. | Same shared Compose runner with `<name>` as the service argument. |
| `angee service destroy <name>` | Remove a dynamic service from the manifest and stop its runtime. If the service belongs to an agent bundle, the agent command decides whether to remove that agent's materialized workspace copy. | Manifest edit plus runtime stop. |
| `angee agent start/stop <name>` | Alias for service start/stop on the service inside the agent bundle. | Scoped runtime start/stop for the agent's service. |

The important distinction is intent. `init` and `update` change the manifest. `up`, `down`, `start`, `stop`, `restart`, and `logs` are DRY shorthands around Docker Compose for already-declared resources. In v1, the Compose project directory is always `ANGEE_ROOT` itself, usually `.angee`.

If CI wants one line, it can run `angee stack init ... && angee up`, but Angee core should keep those operations separate.

## Services, Workspaces, Sources, And Agents

Services are runtime primitives. Workspace definitions are source/mount bundles owned by an agent. They become files only when `agent init` or `agent update` materializes them into that agent's derived workspace folder.

DRY rule: implement runtime operations once in the shared Docker Compose runner, then route stack, service, and agent commands through it. An agent is a bundle of one service plus one per-agent materialized workspace copy. Keep `agent start`, `agent stop`, `agent restart`, `agent logs`, and `agent list` as aliases or filtered views over those bundled service resources.

Do not confuse resource type with runtime operations or metadata. `stack`, `service`, `job`, `workspace`, and `source` are manifest concepts. `agent` is a composite resource bundle. Workspace definitions are not independently initialized by the user. Services do not need a `lifecycle` field.

| Primitive | Purpose | Operations |
|---|---|---|
| Service | A runnable workload: app process, worker, sidecar, system service, or agent. | init, list, start, stop, restart, logs, destroy. |
| Job | A one-shot or scheduled command, usually implemented as a Docker Compose service with restart disabled or a template-generated command. | init/update through templates, run/status/logs when supported. |
| Workspace | An agent-owned source/mount bundle created or updated by agent provisioning. It declares source entries, subpaths, template answers, and persistence policy. It is not a running thing and has no standalone lifecycle. | Managed by agent init/update/destroy policy. |
| Source | A content input inside an agent workspace or stack template. It can come from a template, git repo, URL/archive, local path, or volume. | Declared and updated through stack or agent provisioning. |

Use service metadata only when it has a concrete purpose. For example, Docker labels, Kubernetes labels, routing labels, or tags can identify roles without changing the core model:

| Metadata | Use |
|---|---|
| `labels` | Backend labels such as Traefik routing, Docker labels, Kubernetes labels, or Angee ownership labels. |
| `tags` | Human or API filtering such as `system`, `worker`, `database`, `agent`, or `internal`. |
| `workspace` | Agent-owned workspace metadata. The operator materializes it into `$ANGEE_ROOT/agents/<agent>/workspace`. |
| `jobs` | One-shot or scheduled work should be represented as jobs, not as long-running services. |

Jobs should cover real provisioning commands such as Django migrations, `collectstatic`, preset loading, and agent-template sync. In v1 they can compile to Compose services or template-generated commands, but they should be declared as jobs so Angee can record intent, ordering, logs, and success/failure status.

Agent commands should be aliases for the bundled service lifecycle and composite wrappers for template operations:

| Agent command | Internal operation |
|---|---|
| `angee agent init <name>` | Render the agent template, create the agent workspace definition, materialize that workspace into `agents/<name>/workspace`, then initialize a service that mounts or uses that folder. |
| `angee agent update <name>` | Update the agent template, re-materialize agent workspace sources as needed, then update the service definition. |
| `angee agent list` | List agent bundles by joining services with their agent-owned workspace metadata. |
| `angee agent start <name>` | `service start <name>`. |
| `angee agent stop <name>` | `service stop <name>`. |
| `angee agent restart <name>` | `service restart <name>`. |
| `angee agent logs <name>` | `service logs <name>`. |
| `angee agent destroy <name>` | Destroy the service and remove or preserve that agent's materialized workspace copy according to policy. |

The same shape should exist in the API:

| Resource | CLI | Operator API |
|---|---|---|
| Stack runtime | `angee up/down/start/stop/restart/logs` | `POST /stack/up`, `POST /stack/down`, `POST /stack/start`, `POST /stack/stop`, `POST /stack/restart`, `GET /logs` |
| Services | `angee service init/update/list/start/stop/restart/logs/destroy` | `POST /services/init`, `POST /services/{name}/update`, `GET /services`, `POST /services/{name}/start`, `POST /services/{name}/stop`, `POST /services/{name}/restart`, `GET /services/{name}/logs`, `POST /services/{name}/destroy` |
| Jobs | `angee job run/list/logs` | `POST /jobs/{name}/run`, `GET /jobs`, `GET /jobs/{name}/logs` |
| Agents | `angee agent init/list/start/stop/restart/logs/destroy` | Composite endpoints over services and workspace materialization: `POST /agents/init`, `GET /agents`, `POST /agents/{name}/start`, `POST /agents/{name}/stop`, `POST /agents/{name}/restart`, `GET /agents/{name}/logs`, `POST /agents/{name}/destroy` |

## Resource Operations Matrix

This matrix is the target operation surface. Agent is listed only to show it is shorthand for service plus per-agent workspace materialization, not a separate runtime implementation path.

| Resource | Type | Init | Update | List/Status | Start/Up | Stop/Down | Logs | Destroy |
|---|---|---|---|---|---|---|---|---|
| Stack | Primitive | `angee stack init <template>` / `POST /stack/init` | `angee stack update` / `POST /stack/update` | `angee status` / `GET /stack/status` | `angee up` or `angee start` / `POST /stack/up`, `POST /stack/start` | `angee down` or `angee stop` / `POST /stack/down`, `POST /stack/stop` | `angee logs` / `GET /logs` | `angee stack destroy` / `POST /stack/destroy` |
| Service | Primitive | `angee service init <name>` / `POST /services/init` | `angee service update <name>` / `POST /services/{name}/update` | `angee service list` / `GET /services` | `angee service start <name>` / `POST /services/{name}/start` | `angee service stop <name>` / `POST /services/{name}/stop` | `angee service logs <name>` / `GET /services/{name}/logs` | `angee service destroy <name>` / `POST /services/{name}/destroy` |
| Job | Primitive command | Declared by stack or template | Updated by stack or template | `angee job list` / `GET /jobs` | `angee job run <name>` / `POST /jobs/{name}/run` | Not applicable | `angee job logs <name>` / `GET /jobs/{name}/logs` | Removed by stack update |
| Workspace | Agent-owned source/mount bundle | Created by `angee agent init` when needed | Updated by `angee agent update` | Visible through `angee agent list/status` | Not applicable | Not applicable | Not applicable | Removed or preserved by `angee agent destroy` policy |
| Agent | Composite bundle | `angee agent init <name>` initializes service and materializes workspace sources into the agent folder | `angee agent update <name>` updates service and re-materializes sources as needed | `angee agent list` joins services with agent workspace metadata | `angee agent start <name>` aliases service start | `angee agent stop <name>` aliases service stop | `angee agent logs <name>` aliases service logs | `angee agent destroy <name>` destroys service plus the agent's materialized workspace copy according to policy |

Stack is singular per `ANGEE_ROOT`, so `list` is normally `status`. Services and agents are many per stack and must support `list` through both CLI and API. Workspace definitions are inspected through their owning agents.

## Jobs

Jobs are declared commands that belong to the stack but are not long-running services. They are the right primitive for provisioning steps that need ordering, logs, and success/failure status.

Examples:

| Job | Purpose |
|---|---|
| `django-migrate` | Run `manage.py migrate --noinput` before the Django service starts. |
| `collectstatic` | Build static assets for production or staging. |
| `load-presets` | Load taxonomy, seed, or template data into the database. |
| `sync-agents` | Sync Django agent templates/tools with the application database. |

Target job fields:

| Field | Meaning |
|---|---|
| `command` | Command to execute, compiled to Docker Compose in v1. |
| `image` / `build` | Runtime image or build context. |
| `env` / `env_file` | Runtime environment and generated env artifacts. |
| `depends_on` | Services or jobs that must start or complete first. |
| `run_on` | `init`, `up`, `manual`, or `always`. |
| `restart` | Usually `no`; jobs are not services. |
| `logs` | Retain enough output for status and debugging. |

In v1 jobs can compile directly to Docker Compose services with one-shot commands. Do not design a separate workflow engine around them.

## Template Setup

Templates are Copier templates with Angee metadata in `copier.yml` under `_angee`.

Canonical template families:

```text
templates/stacks/<name>/
templates/workspaces/<name>/
templates/agents/<name>/
```

Useful refs:

```text
stacks/dev
stacks/staging-docker
workspaces/angee-worktree
agents/angee-developer
agents/personal-assistant
./templates/stacks/dev
gh:org/repo#templates/stacks/prod
```

The operator template pipeline is:

1. Resolve the template ref.
2. Load `_angee` metadata and verify the template kind.
3. Render or update with Copier using explicit answers.
4. Write `.copier-answers.yml` for future updates.
5. Load the rendered manifest.
6. Validate it.
7. Materialize sources that belong to the stack or active agent. Workspace sources are not materialized globally; they are materialized only into agent-derived workspace folders.
8. Resolve required secrets and write concrete service port assignments into `angee.yaml`.
9. Register the stack or agent in `$ANGEE_ROOT/angee.yaml`.
10. Commit non-secret manifest changes in the `ANGEE_ROOT` git repository when git history is enabled.

Template responsibilities for v1:

1. The Copier template is the source of truth for the generated scaffold: `angee.yaml`, `docker-compose.yaml` inputs, env-file artifacts, helper commands, and developer documentation.
2. The rendered `angee.yaml` is the operational file backend after init/update. Users can edit it, but template updates remain the preferred way to keep generated structure coherent.
3. Port questions and defaults belong in template metadata. The operator writes concrete selected ports into `angee.yaml` and propagates the same values into generated Docker Compose and env artifacts.
4. Docker Compose is the v1 runtime target. Templates may include Compose-specific fields and artifacts.
5. Local dev processes are still an Angee concern through `angee dev`, but only when they are declared by the template. Angee core starts declared commands; it does not infer Django, Vite, `uv`, `pnpm`, or `manage.py` behavior on its own.
6. Operator bootstrap belongs in template output: generate `ANGEE_OPERATOR_URL`, a token secret, and service registration config in `angee.yaml` or env artifacts. Do not introduce a separate `operator.yaml`.

## Interactive Mode

CLI provisioning commands are interactive by default. Without `--yes`, Angee should ask for missing template answers, required secrets, port overrides, source refs, agent workspace policy choices, and destructive confirmations.

This preserves the old main-branch UX where init guided the user through template questions, while keeping the clean operator-owned implementation. The clean command grammar should not make bare `angee init` guess a resource type: use `angee init --dev` for the dev stack shortcut, or use an explicit noun.

| Mode | Behavior | Use |
|---|---|---|
| Interactive CLI | Prompt for missing answers and confirmations. | Humans running `angee init --dev`, `angee stack init`, or `angee agent init`. |
| Non-interactive CLI | Require `--yes` plus all required values via flags, env imports, defaults, or generated values. | CI, scripts, tests. |
| API/operator | Always non-interactive. Missing required values return structured errors listing what is needed. | Django, dashboards, agents, CI. |

Examples:

```sh
angee init --dev
```

Prompts for the dev stack template answers, required secrets, and service port choices.

```sh
angee stack init staging-docker --yes \
  --set domain=staging.example.com \
  --secret anthropic-api-key=env:ANTHROPIC_API_KEY
```

Runs without prompts and fails fast if a required answer is missing.

Prompting belongs in the CLI adapter. The operator should expose a dry validation response such as `missing_answers`, `missing_secrets`, `conflicts`, and `confirmations_required`, but it should not block waiting for terminal input.

## Stack Setup

A stack template creates the environment root. `stack init` renders the template, updates `angee.yaml`, and prepares `ANGEE_ROOT`. `up` is the separate runtime operation that starts or reconciles the stack.

Example local dev stack:

```sh
angee stack init dev --yes
```

Without `--yes`, `stack init` should prompt for Copier answers, required secrets, and service port choices before calling the operator provisioning endpoint.

Expected result:

```text
.angee/
  angee.yaml
  docker-compose.yaml        # generated when docker services exist
  agents/
  run/                       # volatile operator working files, gitignored
  .env                       # generated, gitignored secrets
```

The stack template decides the Docker Compose shape for the environment. Other backend targets are explicitly future work; do not complicate v1 with Kubernetes or generic backend abstractions beyond what the current code already needs.

## Dev Environment

`angee dev` should run an ephemeral in-process operator runtime for the lifetime of the command and orchestrate the template-declared local runtime. This preserves the old useful dev UX while keeping framework knowledge out of Angee core.

The dev flow:

1. Discover or accept `ANGEE_ROOT`.
2. Load `$ANGEE_ROOT/angee.yaml`.
3. Resolve sources, secrets, and concrete service ports from `angee.yaml`. Workspace sources are materialized only for agents that own them.
4. Generate or refresh Docker Compose and env artifacts when needed.
5. Start Docker Compose sidecars or services declared for dev.
6. Run dev jobs such as migrations or seed commands when declared.
7. Start local processes declared by the template.
8. Stream logs with line or pane UI.
9. Stop local processes on exit and leave persistent Docker sidecars according to template policy.
10. Expose operator URLs, service registration, status, and logs.

Local application processes are template-declared Angee runtime entries. For a Django template, the template can declare local processes for Django and Vite, plus Docker sidecars for Postgres and Redis. Angee starts what is declared; it should not hardcode Django, Vite, pnpm, uv, or `manage.py` behavior.

`angee dev` must not dispatch to framework-specific tooling by detection. Django, Vite, pnpm, uv, or other details belong inside template-declared local runtime commands.

Local runtime fields should be minimal:

| Field | Meaning |
|---|---|
| `runtime: local` | Run this service as a host process during `angee dev`. |
| `command` | Exact command supplied by the template. |
| `workdir` | Directory relative to `ANGEE_ROOT` or an absolute path. |
| `env` / `env_file` | Environment and generated env artifacts. |
| `depends_on` | Docker services, jobs, or local services that must start first. |
| `ports` | Concrete host ports shared with generated env and Compose artifacts. |
| `health` | Optional HTTP or command health check. |
| `shutdown` | Signal/timeout policy for Ctrl+C and process cleanup. |

## Staging Environment

Staging is the same stack flow with a different template.

```sh
angee stack init staging-docker --yes \
  --set domain=staging.example.com \
  --secret anthropic-api-key=env:ANTHROPIC_API_KEY
angee up
```

The staging template can emit Docker Compose services, volumes, networks, and an operator service. The stack manifest still declares desired Angee resources; generated Compose files remain implementation details.

A staging operator usually runs as a service:

```sh
angee operator --bind 0.0.0.0 --port 9000
```

Remote operators must require an API key for non-loopback binds. The current server already enforces this direction and should keep it.

## Agent Workspace Definitions

A workspace is an agent-owned source/mount definition. It is useful when an agent should work from code, skills, docs, memory shape, or template scaffold. It is not initialized on its own and it does not have one global materialized directory.

```sh
angee agent init reviewer \
  --template agents/angee-developer \
  --source app=https://github.com/fyltr/app#feature-x \
  --source skills=https://github.com/fyltr/agent-skills#main \
  --start \
  --yes
```

The workspace flow happens inside `agent init` and `agent update`:

1. Resolve the agent template from `--template` or `agents.default_template`.
2. Resolve the workspace template from an agent template default or explicit agent options, if one is used.
3. Convert the template output, CLI flags, and API inputs into workspace source/mount declarations.
4. Validate source names, refs, subpaths, persistence policy, and volume references.
5. Store the workspace definition under the owning agent in `$ANGEE_ROOT/angee.yaml`.
6. Materialize the workspace into `$ANGEE_ROOT/agents/<agent>/workspace`.
7. Do not create a global `$ANGEE_ROOT/workspaces/<name>` folder.

There are no standalone workspace commands. Starting, stopping, logging, and materialization belong to the owning agent's service lifecycle and agent provisioning flow.

Without `--yes`, `agent init` and `agent update` should prompt for agent template answers, workspace source refs, source subpaths, persistence policy, and update confirmations.

Multiple agents can be initialized from the same source inputs, but each agent gets its own workspace definition and materialized copy:

```sh
angee agent init reviewer --source app=https://github.com/fyltr/app#feature-x --start --yes
angee agent init tester --source app=https://github.com/fyltr/app#feature-x --start --yes
```

## Mounts, Sources, And Volumes

Agent workspaces are materialized in layers. First the agent template renders runtime scaffold into `$ANGEE_ROOT/agents/<name>`. Then the agent's workspace sources are materialized into `$ANGEE_ROOT/agents/<name>/workspace`.

Mounts, sources, and volumes are related but not the same. Mounts are the general file presentation primitive. A source is a mount whose content must be materialized from a template, git repo, URL/archive, local path, or volume before runtime.

| Resource | Purpose | Example |
|---|---|---|
| Agent workspace | Agent-owned set of source declarations and mounts. | `reviewer` has app code, skills, docs, and memory shape. |
| Mount | A file tree or storage location exposed into a service or agent workspace. | Bind `./src/fyltr-django` at `/app`, mount `agent-memory` at `/workspace/memory`. |
| Source | Content materialized into an agent workspace subpath before the agent service runs. | Clone `https://github.com/org/skills#main` into `skills/`. |
| Template source | Source content rendered from a Copier template. | Render `workspaces/angee-worktree` into `base/`. |
| Persistent source | Per-agent materialized content that should be preserved across updates unless explicitly refreshed or purged. | Agent `skills/`, memory, checked-out worktree. |
| Volume declaration | Persistent storage managed by the host or backend. | Keep agent memory under a per-agent volume or mount Postgres data. |
| Volume source | A source whose content is backed by a declared volume. | Expose `agent-memory` at `memory/`. |
| Source subpath | The path inside each agent's materialized workspace where source content appears. | `subpath: code`, `subpath: skills`, `subpath: docs`. |
| Service volume mount | A runtime mount into a container or process. | Mount `memory` at `/workspace/memory`. |

A template answers "what static scaffold should exist?" A source answers "what external or persistent content should exist inside each agent workspace?" A volume answers "what storage should persist across runs?"

Mounts should be able to replace most ad hoc `raw_volumes` and many source-specific fields over time:

| Mount type | Meaning |
|---|---|
| `bind` | Host path mounted into a service, normally dev-only. |
| `volume` | Docker volume or Angee logical volume. |
| `tmpfs` | Ephemeral in-memory mount. |
| `cache` | Persistent cache such as Hugging Face or package-manager cache. |
| `config` | Generated file artifact such as `.env.letta`, `opencode.json`, or agent instructions. |
| `source` | Materialized content from template/git/url/local/volume into a target path. |

Source kinds should include:

| Kind | Meaning |
|---|---|
| `template` | Render a Copier template as source content. |
| `git` or `github` | Clone or sync a repository at `ref`, optionally restricted to `tree`. Branch and tag refs are resolved to commit locks during provisioning. |
| `url` or `archive` | Download a file or extract an archive into the workspace. Non-dev sources should include a checksum. |
| `local` | Copy or bind material from the local project worktree. |
| `volume` | Expose persistent volume-backed content at a workspace subpath. Requires a `volume` field that references a top-level volume declaration. |

Example agent workspace layout after materialization:

```text
agents/reviewer/
  agent.yaml
  .env
  workspace/
    base/                 # template source
    code/                 # git/github/local source
    skills/               # git/url/template source
    memory/               # persistent volume source
    docs/                 # archive/url/template source
```

Example source and volume model:

```yaml
volumes:
  agent-memory:
    scope: agent
    driver: local-fs

agents:
  reviewer:
    template: agents/angee-developer
    workspace:
      sources:
        base:
          subpath: base
          source:
            kind: template
            template: workspaces/angee-worktree
        app:
          subpath: code
          source:
            kind: git
            url: https://github.com/fyltr/app
            ref: feature-x
            lock:
              commit: "<resolved-commit-sha>"
        skills:
          subpath: skills
          persistent: true
          source:
            kind: git
            url: https://github.com/fyltr/agent-skills
            ref: main
            lock:
              commit: "<resolved-commit-sha>"
        docs:
          subpath: docs
          source:
            kind: url
            url: https://example.com/agent-docs.zip
            checksum: "sha256:<digest>"
        memory:
          subpath: memory
          persistent: true
          scope: agent
          source:
            kind: volume
            volume: agent-memory

services:
  reviewer:
    tags: [agent]
    mounts:
      - type: bind
        source: agents/reviewer/workspace
        target: /workspace
      - type: volume
        source: agent-memory
        target: /workspace/memory
```

Target behavior:

1. Agent workspace definitions live under their owning agent in `angee.yaml`.
2. Agent materialization creates or updates `$ANGEE_ROOT/agents/<agent>/workspace`.
3. Source materialization creates or updates subpaths inside that per-agent folder.
4. Two agents created from the same source inputs still get separate materialized copies, so agent writes do not collide by default.
5. Persistent sources preserve each agent's materialized content across update/destroy unless explicitly refreshed or purged.
6. Volume-backed workspace sources default to `scope: agent`; the operator derives a concrete volume identity per agent.
7. Shared mutable content must be explicit with `scope: shared` on the source or volume declaration.
8. Agent update changes that agent's workspace definition and re-materializes the workspace when needed.
9. Agent destroy removes or preserves that agent's materialized workspace copy according to policy.
10. Volume-backed sources preserve content across `up`, `down`, `start`, and `stop`.
11. Persistent volumes are preserved by default and deleted only with an explicit purge option.
12. Docker Compose maps volumes to bind mounts or named volumes in v1.
13. Internally, source declarations can compile to typed mounts. Keep the user-facing model simple: use `mounts` for runtime file presentation and `source` only when content must be fetched or rendered.

Volume identity should be explicit. Something is a volume when it is declared under top-level `volumes`. A source can then reference that volume with `kind: volume` and `volume: <name>`. A service can mount that same volume with a typed `mounts[]` entry. The logical volume name can still compile to per-agent backend volumes when `scope: agent` is set.

Source resolution rules:

1. CLI and API inputs may use shorthand such as `--source app=https://github.com/fyltr/app#feature-x`.
2. The committed manifest should be normalized to explicit `kind`, `url`, `ref`, `checksum`, `lock`, `subpath`, `persistent`, and `scope` fields where relevant.
3. Git branch and tag refs should write a resolved commit lock during init/update. Updating to a newer commit is an `agent update` operation, not an implicit side effect of `up`.
4. URL and archive sources should require a checksum outside local dev templates.
5. Source subpaths must be unique and non-overlapping by default. Overlay behavior requires an explicit order and conflict policy.

Static-only agent workspace example:

```yaml
agents:
  assistant:
    workspace:
      sources:
        base:
          subpath: .
          source:
            kind: template
            template: workspaces/angee-worktree
```

Static template rendered under a subpath:

```yaml
agents:
  assistant:
    workspace:
      sources:
        base:
          subpath: base
          source:
            kind: template
            template: workspaces/angee-worktree
```

## Secrets Management

Secrets sit next to sources and volumes as declared requirements. Secret values are never part of committed template output or `angee.yaml`; manifests contain only secret declarations and references such as `${secret:anthropic-api-key}`.

Target secret value sources:

| Source | Meaning |
|---|---|
| Supplied value | `--secret name=value` or API `secrets: { "name": "value" }`. |
| Environment import | `--secret name=env:ENV_VAR`; the operator reads `ENV_VAR` and stores the value in the configured secrets backend. |
| Generated | `generated: true` creates a stable random value in the selected secret backend and preserves it across updates. |
| Default | `default: value` is acceptable only for non-sensitive defaults or local-only templates. |
| Backend lookup | The operator reads an existing value from `.env` or OpenBao. |

Target secret backends:

| Backend | Use |
|---|---|
| `.env` | Local dev default. Stores values in `.angee/.env` or a template-selected env file. Gitignored. |
| `env` | Bootstrap/import source. Environment variables are inputs, not a persistent backend by themselves. |
| `openbao` | Shared secret storage for staging or server environments. Can run as a service tagged `system` or point to an external OpenBao instance. |

The original default template used OpenBao this way:

```yaml
services:
  openbao:
    image: openbao/openbao:2
    tags: [system, secrets]
    command: server -dev -dev-listen-address=0.0.0.0:8200
    env:
      BAO_DEV_ROOT_TOKEN_ID: "${secret:bao-token}"

secrets:
  bao-token:
    description: OpenBao root token for dev bootstrap
    required: true
    generated: true
    length: 32

secrets_backend:
  type: openbao
  openbao:
    address: http://openbao:8200
    auth:
      method: token
      token_env: BAO_TOKEN
    prefix: angee
```

Keep the concept, but make the safety boundary explicit:

1. The OpenBao root token is a dev/bootstrap credential only.
2. Production and shared staging should use a non-root OpenBao auth method such as AppRole, Kubernetes auth, or a scoped service token.
3. `BAO_TOKEN`, `ANTHROPIC_API_KEY`, and similar environment variables are bootstrap inputs; Angee should import or reference them without committing values.
4. The operator writes `.env` files only when the template chooses env-file secrets or when needed as runtime delivery artifacts for local processes and Docker Compose.
5. Future non-Compose backends must still keep secret values out of committed manifests.

Example clean secret declaration:

```yaml
secrets:
  anthropic-api-key:
    required: true
  bao-token:
    generated: true
    length: 32

services:
  reviewer:
    env:
      ANTHROPIC_API_KEY: "${secret:anthropic-api-key}"
```

## Agent Setup

An agent is a composite resource bundle: a service plus one materialized copy of its own workspace definition. The default developer-agent template creates `$ANGEE_ROOT/agents/<name>` and materializes the selected sources into `$ANGEE_ROOT/agents/<name>/workspace`.

```sh
angee agent init reviewer \
  --template agents/angee-developer \
  --source app=https://github.com/fyltr/app#feature-x \
  --secret anthropic-api-key=env:ANTHROPIC_API_KEY \
  --start \
  --yes
```

The agent flow:

1. Resolve the agent template from `--template` or `agents.default_template`.
2. Render into `$ANGEE_ROOT/agents/<name>`.
3. Create or update the agent-owned workspace definition from supplied source options and template defaults.
4. Materialize code, memory, template output, or other declared sources into `$ANGEE_ROOT/agents/<name>/workspace`.
5. Merge required secrets, MCP servers, volumes, service metadata, and agent workspace metadata into the stack manifest.
6. Write agent `.env` only if the template selected env-file secrets.
7. Register the agent in `$ANGEE_ROOT/angee.yaml`.
8. Optionally start the bundled service when `--start` is passed.

Without `--yes`, `agent init` and `agent update` should prompt for agent template answers, workspace source refs, model/provider choices, required secrets, and destroy policy when applicable.

Agent templates should cover at least two common shapes:

| Template | Use |
|---|---|
| `agents/angee-developer` | Code agent with an agent-owned workspace, filesystem MCP, Angee operator MCP, and application MCP. |
| `agents/personal-assistant` | Long-lived personal agent with memory volume, user identity, and MCP tools. |

## Agent Bundle Operations

An agent has the same runtime lifecycle as a service. `start`, `stop`, `restart`, `logs`, and status all operate on the bundled service and reuse that agent's materialized workspace copy.

Agent-specific commands and API endpoints can exist for UX, but the implementation should call the same service operations.

The only agent-specific behavior is that `init`, `update`, and `destroy` are composite template operations. They affect the service, agent scaffold, and that agent's materialized workspace copy. Runtime operations are not special.

| Action | Implementation shape | Agent-facing command/API |
|---|---|---|
| Create | Composite provisioning: render agent template, create the agent workspace, materialize sources, then register service. | `angee agent init <name>` / `POST /agents/init` |
| Update | Composite provisioning: update agent template/source material, re-materialize when needed, then update service. | `angee agent update <name>` / `POST /agents/{name}/update` |
| Start | Service runtime operation. | `angee agent start <name>` / `POST /agents/{name}/start` |
| Stop | Service runtime operation. | `angee agent stop <name>` / `POST /agents/{name}/stop` |
| Restart | Service runtime operation. | `angee agent restart <name>` / `POST /agents/{name}/restart` |
| Logs | Service runtime operation. | `angee agent logs <name>` / `GET /agents/{name}/logs` |
| List | Service list plus agent/workspace join. | `angee agent list` / `GET /agents` |
| Destroy | Service destroy plus per-agent materialization policy. | `angee agent destroy <name>` / `POST /agents/{name}/destroy` |

Runtime operation semantics are identical:

| Action | Behavior |
|---|---|
| Start | Compile current stack and apply enough runtime resources to start the bundled service. The materialized workspace copy and persistent volumes are reused. |
| Stop | Stop the bundled service without deleting config, agent workspace definitions, or materialized workspace files. |
| Restart | Stop then start. |

Composite template operation semantics are agent-specific only because they coordinate service, agent scaffold, and materialized workspace-source changes:

| Action | Behavior |
|---|---|
| Init | Render agent template, create the agent workspace definition, materialize sources into the agent folder, register a service that mounts or uses that folder, resolve secrets/service ports, optionally start. |
| Update | Update agent template/source material, re-materialize workspace sources if needed, rewrite service registration, optionally restart. |
| Destroy | Stop and unregister service, then remove or preserve the agent's materialized workspace copy and volumes according to explicit policy. |

## Operator And Application Control

Django or any other runtime should manage agents by calling the operator, not by reimplementing Angee logic.

The integration model:

1. Application creates a domain concept such as a user agent, review agent, or task agent.
2. Application calls the Angee operator HTTP API with the agent name, template, source overrides, secrets, and start preference.
3. Operator creates or updates the agent workspace definition, materializes it under the agent folder, and provisions the runtime service.
4. Application stores only its own domain record and the Angee agent name.
5. Application calls operator endpoints to start, stop, destroy, tail logs, or inspect status.

Example API calls:

```http
POST /agents/init
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "name": "review-123",
  "template": "agents/angee-developer",
  "source_overrides": {
    "app": "https://github.com/fyltr/app#feature-x"
  },
  "secrets": { "anthropic-api-key": "env:ANTHROPIC_API_KEY" },
  "start": true,
  "yes": true
}
```

```http
POST /agents/review-123/stop
Authorization: Bearer <api-key>
```

```http
POST /agents/review-123/destroy
Authorization: Bearer <api-key>
```

MCP should expose the same operational surface for agents that need self-service. The current MCP layer exposes list/start/stop/logs, but the clean target should also expose init/update/restart/destroy when safe.

## Standalone Operator

The standalone operator is for staging, server environments, dashboards, CI, and application backends.

It should:

1. Serve exactly one `ANGEE_ROOT`; use separate operators for separate environments, tenants, or trust boundaries.
2. Expose HTTP, OpenAPI, and MCP.
3. Require bearer auth for non-loopback bindings.
4. Operate from canonical `.angee/angee.yaml`; no `operator.yaml` is required.
5. Serialize writes to `angee.yaml`, generated runtime files, and runtime apply operations.
6. Update `angee.yaml` in place with atomic writes and conflict detection.
7. Sync the file backend and a configured database through operator-mediated import/export with revision checks.
8. Keep business logic in `service.Platform`.
9. Support graceful shutdown without destroying managed runtime resources.

The CLI should default to the in-process operator path. It should only use HTTP when the user explicitly configures `--operator` or `ANGEE_OPERATOR_URL`.

Operator bootstrap should come from the Copier template and be recorded in `angee.yaml` plus generated env artifacts:

| Bootstrap value | Purpose |
|---|---|
| `operator.url` / `ANGEE_OPERATOR_URL` | URL used by Django, frontend proxies, MCP clients, and agents. |
| `operator.token_secret` / `ANGEE_OPERATOR_TOKEN` | Secret-backed bearer token for local app/operator communication. |
| `operator.service_registration` | Optional service metadata the application can read or sync, such as app name, environment, MCP URLs, and allowed operations. |

Templates should render these consistently into Docker Compose, Django env, frontend proxy env, and agent env. The operator should validate and serve the configured values, not invent unrelated runtime configuration.

## Deferred Scope

These should be designed as clean extension points, but they are not required for the first clean refactor:

1. ReBAC authorization with SpiceDB. The target policy file is `permissions.zed`, shared by Angee runtime integrations such as `django-angee` and the Angee operator.
2. Agent workspace source provisioning hooks. Templates may eventually declare commands to run before or after source materialization, but hooks must be explicit, operator-owned, and auditable.
3. Durable workflows. For now, Angee should focus on services, jobs, agents, and agent workspace source provisioning. Do not claim Temporal-like workflow semantics without durable event history, replay rules, and worker/task-queue boundaries.

## Runtime And Reconciliation

The v1 operator reconciliation pipeline is Docker Compose-oriented:

1. Load and validate `angee.yaml`.
2. Resolve stack/service sources immediately and workspace sources only into their owning agent directories, using existing source locks unless the operation explicitly updates sources.
3. Resolve required, generated, default, and supplied secrets.
4. Use concrete service ports declared in `angee.yaml`; new services get ports through prompts, flags, or API values during init/update.
5. Render agent config files and `.env` files only when the template selected env-file secrets.
6. Compile `docker-compose.yaml` and generated env/config artifacts.
7. Apply runtime changes through the shared Docker Compose runner when the operation is `up`, `start`, or `restart`; do not silently advance branch refs or URL/archive content during runtime-only operations.
8. Stop runtime resources through the shared Docker Compose runner when the operation is `down`, `stop`, or destroy.
9. In dev mode, start and stop template-declared local processes.
10. Report status, health, logs, and changed resources.

Docker Compose is the only runtime backend for this clean v1. The Compose runner should always invoke Docker Compose with `--project-directory $ANGEE_ROOT` and the generated `$ANGEE_ROOT/docker-compose.yaml`, keeping `up`, `down`, `start`, `stop`, `restart`, and `logs` as dry shorthands over one implementation. `angee dev` also supports a local host-process runtime declared by templates. Kubernetes or another runtime backend can be refactored in later once the Docker Compose and local-dev models are solid.

## Files And Run Scratch

`ANGEE_ROOT` should be easy to inspect:

```text
ANGEE_ROOT/
  angee.yaml                 # stack desired manifest, committed
  docker-compose.yaml         # generated runtime file
  .env                       # local env-file secrets when selected, gitignored
  agents/<name>/             # rendered agent config, env, materialized workspace
  volumes/                   # local persistent volumes when selected
  run/                       # volatile operator working copies, gitignored
```

Secrets and host-local runtime files must stay out of git. Template answers and non-secret config should be committed so updates and rollback are understandable.

The operator may create a run-scoped working copy while reconciling:

```text
.angee/run/angee-operator.yaml.<pid>
```

Rules for `run/`:

1. Canonical manifest is always `.angee/angee.yaml`.
2. `run/angee-operator.yaml.<pid>` is a working copy for one operator process.
3. The operator writes final changes back to `.angee/angee.yaml` atomically.
4. `run/` is gitignored and can be cleaned safely when no operator is running.
5. If `run/angee-operator.yaml.*` remains after the operator exits, the previous operator likely crashed.

## Cases To Include

The user-covered cases are stack setup, agent workspace materialization, agent bundle setup, dev mode, staging via Compose, standalone operator, and service operations. The clean version should also explicitly cover these cases:

| Case | Why it matters |
|---|---|
| Stack update | Users need to pull template improvements without recreating an environment. |
| Init/up/down semantics | Provisioning and runtime power controls need separate meanings, while v1 runtime power stays a thin Docker Compose wrapper. |
| Agent workspace cleanup | Agent destroy needs clear remove/preserve policy for the agent's materialized workspace copy. |
| Agent workspace materialization | Agent workspaces aggregate sources, but they are created and updated through agent provisioning, not standalone commands. |
| Explicit volume identity | Templates need a clear way to say what is persistent storage versus materialized source content. |
| Typed mounts | Templates need a clean replacement for raw Compose bind mounts and generated config files. |
| Source locks and checksums | Runtime-only operations must not silently advance branch refs or remote archive content. |
| Secrets backend strategy | Env-file secrets, env imports, and OpenBao-backed secrets need one resource model. |
| Jobs | Django migrations, `collectstatic`, presets, and agent sync need ordered one-shot command semantics. |
| Local dev runtime | `angee dev` needs to start template-declared host processes like the older dev UX without framework detection. |
| `--start` and `--restart` semantics | Current CLI/API flags exist but are not fully wired in service methods. |
| Config get/set | Applications and agents need to read, validate, commit, and optionally run `up` after manifest changes. |
| Plan/status/logs/history | Operators need observability and safe previews before `up`. |
| Rollback | Git-backed `ANGEE_ROOT` history should remain an explicit recovery path. |
| Scale | Runtime services and agents may need replica adjustments. |
| Pull/restart | Staging needs image refresh without changing config. |
| OpenAPI | Django or other runtimes need a generated contract. |
| MCP | Agents should manage allowed Angee operations through tool calls. |
| Manifest/database sync | The operator may sync the file backend and an application database without bypassing validation or conflict handling. |
| Security | Remote operator access must require auth, safe CORS, root scoping, and a clean path to deferred SpiceDB/ReBAC policy. |

## Refactor Decisions

Make these decisions explicit in the clean version:

1. `$ANGEE_ROOT/angee.yaml` is the initial file-backed stack source of truth.
2. `service.Platform` is the only business logic layer.
3. CLI commands do not implement provisioning or runtime actions directly.
4. HTTP and MCP handlers are thin adapters over `service.Platform`.
5. `init` and `update` are environment provisioning; `up`, `down`, `start`, and `stop` are runtime power controls.
6. Docker Compose is the v1 runtime backend; `up`, `down`, `start`, `stop`, `restart`, and `logs` are shared Compose shorthands rooted at `ANGEE_ROOT`.
7. Docker Compose files are generated artifacts, not source of truth.
8. Templates are Copier templates with `_angee` metadata.
9. Templates own generated scaffold, port questions/defaults, env artifacts, and local runtime command declarations.
10. Concrete service ports live in `angee.yaml` and are propagated into Docker Compose and env artifacts; workspaces do not have ports.
11. Secret values live in `.env` or OpenBao, never committed manifests.
12. Agent workspace definitions aggregate sources; agents create/update them and materialize them into per-agent folders; volumes provide persistent storage.
13. Typed mounts replace raw volume strings where Angee needs to reason about files, config artifacts, or storage.
14. Jobs are first-class one-shot or scheduled commands, not long-running services.
15. Secret backends are pluggable: `.env` for local dev, env as bootstrap input, OpenBao for shared environments.
16. Agent is a composite bundle, not a separate runtime operation implementation.
17. One operator serves exactly one `ANGEE_ROOT`; use separate operators for separate environments, tenants, or trust boundaries.
18. File/database sync happens only through the operator with validation, revision checks, and conflict handling.
19. Source shorthand is accepted at the edge, but committed source declarations are normalized and locked.
20. Source subpaths are non-overlapping by default; overlays require explicit order and conflict policy.
21. Operator bootstrap values are generated by templates and stored in `angee.yaml` plus env artifacts; no separate `operator.yaml` is required.
22. `.angee/run/` is volatile scratch only; stale operator run files indicate a likely crash.
23. No framework-specific command dispatch exists in Angee core.
24. No backward-compatibility branches remain in active code.

## Current Gaps To Fix

These are visible in the current implementation and should be closed during the refactor:

| Gap | Target fix |
|---|---|
| `agent init --start` is accepted but not applied. | Start the agent when `AgentInitRequest.Start` is true. |
| Standalone workspace runtime flags exist today but do not fit the target model. | Remove standalone workspace commands and route workspace creation through `agent init/update`. |
| `agent update --restart` is accepted but not applied. | Restart the agent when `AgentUpdateRequest.Restart` is true. |
| MCP cannot init/update/destroy agent bundles. | Add MCP tools for safe agent bundle operations and service lifecycle operations. |
| Workspace cleanup policy is implicit. | Add agent destroy/update policies for removing, preserving, or refreshing the agent's materialized workspace copy. |
| Service operations lack explicit start/stop commands. | Add Docker Compose-backed service start/stop API and CLI commands. |
| Source model lacks explicit volume-backed source fields. | Add `kind: volume` with `volume: <name>`, `scope: agent|shared`, and require a top-level `volumes` declaration. |
| Mounts are still raw strings in old templates. | Add typed `mounts` with `type`, `source`, `target`, `read_only`, `persistent`, and `scope`, then compile them to Compose. |
| Jobs are not first-class. | Add job declarations and compile them to one-shot Compose services or template-generated commands. |
| Local dev runtime is under-specified. | Add `runtime: local` services with command, workdir, env, depends_on, ports, health, and shutdown policy for `angee dev`. |
| Agent workspace source model needs subpaths. | Add `agents.<name>.workspace.sources.<source>.subpath`, persistent source policy, and non-overlap validation, created through agent provisioning. |
| Source model lacks lock metadata. | Normalize CLI/API source shorthand into explicit source entries with git commit locks or URL/archive checksums. |
| Secret backend is file-store based today. | Add a clean `secrets_backend` model with `.env`/env/OpenBao behavior and safe OpenBao bootstrap rules. |
| Operator config file is unnecessary. | Remove `operator.yaml`; use `angee.yaml`, flags, env, and API context. |
| Persistent state directory is unnecessary. | Remove required `state/`; keep only `.angee/run/` for volatile operator working copies. |
| OpenAPI omits some provisioning endpoints. | Generate or maintain OpenAPI for stack, agent, reconcile, pull, and operator APIs. |
| ReBAC authorization is not implemented. | Defer SpiceDB integration with shared `permissions.zed`; keep current remote auth simple until then. |
| Workflow semantics are undefined. | Defer durable workflows; only keep a future hook for command execution during agent workspace source provisioning. |
| Standalone operator command is hidden in the CLI. | Make standalone operator an intentional visible command or document the standalone binary as canonical. |
| Workspace/agent manifest fallback paths add ambiguity. | Pick one canonical rendered manifest name per template kind and remove fallback behavior. |

## Success Shape

The clean version is successful when these flows are boring and consistent:

```sh
angee init --dev --yes
angee dev
```

```sh
angee stack init staging-docker --yes --set domain=staging.example.com
angee up
angee logs web --follow
angee restart web
angee stop
angee start
angee down
angee up
angee operator --bind 0.0.0.0 --port 9000
```

```sh
angee agent init reviewer --template agents/angee-developer --source app=https://github.com/fyltr/app#feature-x --start --yes
angee agent logs reviewer --follow
angee agent stop reviewer
angee agent start reviewer
angee agent destroy reviewer
```

And the same operations work through the operator API so Django, CI, dashboards, and agents can manage Angee without shelling out or duplicating provisioning logic.
