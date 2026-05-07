# Angee Refactor Overview

Status: target clean design, informed by the current Go codebase.

This document describes the streamlined version of Angee we want to build. It is not a backward-compatibility plan. Old command shapes, legacy manifest files, framework-specific dispatch, and unused compatibility branches should be deleted rather than carried forward.

## What Angee Is

Angee is a self-managed orchestration layer for application stacks and AI agents.

It gives a project one control directory, `ANGEE_ROOT`, and one stack source of truth, `angee.yaml`. From that manifest Angee can provision services, jobs, workflows, workspaces, agents, MCP servers, secrets, port leases, sources, volumes, and deployment backend files such as `docker-compose.yaml`.

The core promise is simple:

```sh
angee init --yes
angee dev
```

For staging or another environment, the user should change the template, not the mental model:

```sh
angee stack init staging-docker --yes --set domain=staging.example.com
angee deploy
```

For agent work, the user provisions an agent-backed workspace and controls its lifecycle:

```sh
angee agent init reviewer --template agents/angee-developer --branch feature-x --start --yes
angee agent stop reviewer
angee agent start reviewer
angee agent destroy reviewer
```

## Product Shape

Angee has four first-class concepts.

| Concept | Meaning |
|---|---|
| Stack | The whole environment: services, jobs, workflows, agents, workspaces, secrets, ports, sources, and backend settings. |
| Workspace | An isolated source tree and optional dev runtime for a feature, task, branch, user, or agent. |
| Agent | A managed AI worker with config, tools, MCP servers, secrets, and a workspace. |
| Operator | The reconciler. It renders templates, mutates `angee.yaml`, resolves state, compiles backend files, and applies runtime changes. |

Everything goes through the operator path. The CLI is only a user interface and transport adapter.

## Current Code Bearings

The current code already points in the right direction:

| Area | Current files | Keep the idea |
|---|---|---|
| Shared API types | `api/types.go` | CLI, HTTP, MCP, and app clients should share request/response contracts. |
| Local vs remote dispatch | `cli/client.go` | Use in-process `service.Platform` by default; use HTTP only with `--operator` or `ANGEE_OPERATOR_URL`. |
| Business core | `internal/service/platform.go` | Keep one service layer that knows nothing about Cobra or HTTP. |
| Provisioning | `internal/service/provision.go` | Stack, workspace, and agent init/update belong behind the operator. |
| Templates | `internal/copier/template.go` | Copier should render and update stack, workspace, and agent templates. |
| Sources, secrets, ports | `internal/provision/*` and `internal/state/*` | Operator-owned source materialization, secret resolution, and port leases are the right model. |
| Runtime backend | `internal/runtime/*` | The operator should only speak through `RuntimeBackend`. |
| Docker Compose compiler | `internal/compiler/compose.go` | Compose is the first backend and should stay generated from `angee.yaml`. |
| HTTP/MCP operator | `internal/operator/*` | External runtimes can manage Angee through operator APIs. |

The clean refactor should remove anything that bypasses these paths.

## Clean Command Surface

Use noun-first commands. Keep top-level aliases only when they are obvious shortcuts.

```sh
angee init [path]                         # shortcut for: angee stack init dev [path]
angee stack init <template-name> [path]
angee stack update

angee dev [--only name] [--except name] [--ui lines|panes]
angee deploy                              # compile and apply
angee up                                  # friendly alias for deploy
angee down
angee restart
angee pull
angee plan
angee status                              # alias: ls, ps
angee logs [service]

angee workspace init <name>
angee workspace update <name>
angee workspace dev <name>
angee workspace list
angee workspace destroy <name>

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

Target rule: every command either calls the in-process operator runtime or an explicitly configured remote operator. No command should render templates, inspect Django, run `manage.py`, call pnpm, or manage containers directly outside the operator service layer.

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
workspaces/feature-dev
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
7. Materialize sources.
8. Resolve secrets and port leases.
9. Register the stack, workspace, or agent in `$ANGEE_ROOT/angee.yaml`.
10. Commit non-secret state in the `ANGEE_ROOT` git repository.

## Stack Setup

A stack template creates the environment root.

Example local dev stack:

```sh
angee stack init dev --yes
```

Expected result:

```text
.angee/
  angee.yaml
  operator.yaml
  docker-compose.yaml        # generated when docker services exist
  agents/
  workspaces/
  state/
  .env                       # generated, gitignored secrets
```

The stack template decides whether this is a local process graph, Docker Compose staging stack, or later another backend. Angee does not need different modes or compatibility shims for each environment.

## Dev Environment

`angee dev` should run an ephemeral in-process operator runtime for the lifetime of the command.

The dev flow:

1. Discover or accept `ANGEE_ROOT`.
2. Load `$ANGEE_ROOT/angee.yaml`.
3. Resolve sources, secrets, and port leases.
4. Run local jobs such as migrations or fixtures.
5. Start local services declared with `runtime: local`.
6. Apply Docker services if the manifest also has Docker-backed services or agents.
7. Stream output with line or pane UI.
8. Stop local dev processes on Ctrl+C.

`angee dev` must not dispatch to framework-specific tooling. Django, Vite, pnpm, uv, or other details belong inside the template-generated `services` and `jobs` commands.

## Staging Environment

Staging is the same stack lifecycle with a different template.

```sh
angee stack init staging-docker --yes \
  --set domain=staging.example.com \
  --secret anthropic-api-key=env:ANTHROPIC_API_KEY

angee deploy
```

The staging template can emit Docker Compose services, volumes, networks, and an operator service. The stack manifest still declares desired Angee resources; generated backend files remain implementation details.

A staging operator usually runs as a service:

```sh
angee operator --bind 0.0.0.0 --port 9000
```

Remote operators must require an API key for non-loopback binds. The current server already enforces this direction and should keep it.

## Workspace Setup

A workspace is an isolated materialized source tree registered in the stack.

```sh
angee workspace init feature-x --branch feature-x --start --yes
```

The workspace flow:

1. Resolve the workspace template from `--template` or `workspaces.default_template`.
2. Render into `$ANGEE_ROOT/workspaces/<name>`.
3. Apply branch or source overrides.
4. Materialize sources into the workspace directory.
5. Resolve workspace-scoped secrets and port leases.
6. Register the workspace in `$ANGEE_ROOT/angee.yaml`.
7. Optionally start workspace services when `--start` is passed.

Workspace commands should support the full lifecycle: init, update, dev/start, stop, logs, and destroy. The current code has init, update, list, and dev, but not destroy.

## Agent Workspace Setup

An agent is a managed service plus a workspace. The default developer-agent template creates `$ANGEE_ROOT/agents/<name>` and materializes source into `$ANGEE_ROOT/agents/<name>/workspace`.

```sh
angee agent init reviewer \
  --template agents/angee-developer \
  --branch feature-x \
  --secret anthropic-api-key=env:ANTHROPIC_API_KEY \
  --start \
  --yes
```

The agent flow:

1. Resolve the agent template from `--template` or `agents.default_template`.
2. Render into `$ANGEE_ROOT/agents/<name>`.
3. Create the agent workspace directory.
4. Materialize code, memory, or other declared sources.
5. Merge required secrets, MCP servers, volumes, and agent service metadata into the stack manifest.
6. Write agent `.env` with resolved secrets and exported port leases.
7. Register the agent in `$ANGEE_ROOT/angee.yaml`.
8. Optionally start the agent service when `--start` is passed.

Agent templates should cover at least two common shapes:

| Template | Use |
|---|---|
| `agents/angee-developer` | Code agent with a source workspace, filesystem MCP, Angee operator MCP, and application MCP. |
| `agents/personal-assistant` | Long-lived personal agent with memory volume, user identity, and MCP tools. |

## Agent Lifecycle

The agent lifecycle should be available from the CLI and the operator API.

| Action | CLI | Operator API |
|---|---|---|
| Create | `angee agent init <name>` | `POST /agents/init` |
| Update | `angee agent update <name>` | `POST /agents/{name}/update` |
| Start | `angee agent start <name>` | `POST /agents/{name}/start` |
| Stop | `angee agent stop <name>` | `POST /agents/{name}/stop` |
| Restart | `angee agent restart <name>` | `POST /agents/{name}/restart` |
| Logs | `angee agent logs <name>` | `GET /agents/{name}/logs` |
| List | `angee agent list` | `GET /agents` |
| Destroy | `angee agent destroy <name>` | `POST /agents/{name}/destroy` |

Lifecycle semantics:

| Action | Behavior |
|---|---|
| Init | Render template, register agent, resolve state, optionally start. |
| Update | Update template/source material, rewrite registration, optionally restart. |
| Start | Compile current stack and apply only enough runtime state to start the agent service. |
| Stop | Stop the agent service without deleting config or workspace. |
| Restart | Stop then start. |
| Destroy | Stop service, unregister agent, remove agent directory, commit config change. |

## Operator And Application Control

Django or any other runtime should manage agents by calling the operator, not by reimplementing Angee logic.

The integration model:

1. Application creates a domain concept such as a user agent, review agent, or task agent.
2. Application calls the Angee operator HTTP API with the agent name, template, source ref, secrets, and start preference.
3. Operator provisions the agent workspace and runtime service.
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
  "branch": "feature-x",
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

1. Serve one `ANGEE_ROOT`.
2. Expose HTTP, OpenAPI, and MCP.
3. Require bearer auth for non-loopback bindings.
4. Serialize writes to `angee.yaml`, state files, generated backend files, and runtime apply operations.
5. Keep business logic in `service.Platform`.
6. Support graceful shutdown without destroying managed runtime state.

The CLI should default to the in-process operator path. It should only use HTTP when the user explicitly configures `--operator` or `ANGEE_OPERATOR_URL`.

## Runtime And Reconciliation

The operator reconciliation pipeline is:

1. Load and validate `angee.yaml`.
2. Resolve sources into the correct root, workspace, or agent directory.
3. Resolve required, generated, default, and supplied secrets.
4. Allocate named port leases and preserve existing leases.
5. Render agent config files and environment files.
6. Compile backend files such as `docker-compose.yaml`.
7. Apply runtime changes through `RuntimeBackend`.
8. Start or stop local process runtime records when in dev mode.
9. Report status, health, logs, and changed resources.

Docker Compose is the first backend. Kubernetes can come later through the same `RuntimeBackend` interface and the same `angee.yaml` model.

## State And Files

`ANGEE_ROOT` should be easy to inspect:

```text
ANGEE_ROOT/
  angee.yaml                 # stack desired state, committed
  operator.yaml              # local operator settings, not committed
  docker-compose.yaml         # generated backend file
  .env                       # resolved stack secrets, not committed
  agents/<name>/             # rendered agent config, workspace, env
  workspaces/<name>/          # rendered workspace files and sources
  state/                     # leases, secrets, run records, locks
```

Secrets and host-local runtime files must stay out of git. Template answers and non-secret config should be committed so updates and rollback are understandable.

## Cases To Include

The user-covered cases are stack setup, workspace setup, agent setup, dev mode, staging via Compose, standalone operator, and agent lifecycle. The clean version should also explicitly cover these cases:

| Case | Why it matters |
|---|---|
| Stack update | Users need to pull template improvements without recreating an environment. |
| Workspace destroy | Current lifecycle is incomplete without removing unused workspace state. |
| Workspace start/stop/logs | Workspace services need the same operational feel as agents. |
| `--start` and `--restart` semantics | Current CLI/API flags exist but are not fully wired in service methods. |
| Config get/set | Applications and agents need to read, validate, commit, and optionally deploy manifest changes. |
| Plan/status/logs/history | Operators need observability and safe previews before deploy. |
| Rollback | Git-backed `ANGEE_ROOT` history should remain an explicit recovery path. |
| Scale | Runtime services and agents may need replica adjustments. |
| Pull/restart | Staging needs image refresh without changing config. |
| OpenAPI | Django or other runtimes need a generated contract. |
| MCP | Agents should manage allowed Angee operations through tool calls. |
| State-source strategy | File state is local default; application-backed state can be added without changing the resource model. |
| Security | Remote operator access must require auth, safe CORS, and root scoping. |

## Refactor Decisions

Make these decisions explicit in the clean version:

1. `$ANGEE_ROOT/angee.yaml` is the stack source of truth.
2. `service.Platform` is the only business logic layer.
3. CLI commands do not implement provisioning or runtime actions directly.
4. HTTP and MCP handlers are thin adapters over `service.Platform`.
5. Docker Compose files are generated artifacts, not source of truth.
6. Templates are Copier templates with `_angee` metadata.
7. Named port leases replace one-off port questions.
8. Secret values live in state and `.env`, never committed manifests.
9. No framework-specific command dispatch exists in Angee core.
10. No backward-compatibility branches remain in active code.

## Current Gaps To Fix

These are visible in the current implementation and should be closed during the refactor:

| Gap | Target fix |
|---|---|
| `agent init --start` is accepted but not applied. | Start the agent when `AgentInitRequest.Start` is true. |
| `workspace init --start` is accepted but not applied. | Start workspace runtime when `WorkspaceInitRequest.Start` is true. |
| `agent update --restart` is accepted but not applied. | Restart the agent when `AgentUpdateRequest.Restart` is true. |
| MCP cannot init/update/destroy agents. | Add MCP tools for the full safe agent lifecycle. |
| Workspace lifecycle lacks destroy/stop/logs. | Add complete workspace operational commands and API endpoints. |
| OpenAPI omits some provisioning endpoints. | Generate or maintain OpenAPI for stack, workspace, agent, reconcile, pull, and operator APIs. |
| Standalone operator command is hidden in the CLI. | Make standalone operator an intentional visible command or document the standalone binary as canonical. |
| Workspace/agent manifest fallback paths add ambiguity. | Pick one canonical rendered manifest name per template kind and remove fallback behavior. |

## Success Shape

The clean version is successful when these flows are boring and consistent:

```sh
angee init --yes
angee dev
```

```sh
angee stack init staging-docker --yes --set domain=staging.example.com
angee deploy
angee operator --bind 0.0.0.0 --port 9000
```

```sh
angee workspace init feature-x --branch feature-x --start --yes
angee workspace destroy feature-x
```

```sh
angee agent init reviewer --template agents/angee-developer --branch feature-x --start --yes
angee agent logs reviewer --follow
angee agent stop reviewer
angee agent start reviewer
angee agent destroy reviewer
```

And the same operations work through the operator API so Django, CI, dashboards, and agents can manage Angee without shelling out or duplicating provisioning logic.
