# Angee Substitution Grammar

Status: proposed. Reviewed against Compose, Terraform, GitHub Actions, Helm, Ansible, ytt, DevSpace, Kustomize, Flux CD.

## TL;DR

```
${namespace.path}
${namespace.path | filter}
${namespace.path | filter1 | filter2}
${namespace.path | filter(arg)}
```

- Single `${}` delimiter (not `${{ }}`).
- `.` as namespace separator (not `:`).
- `|` as filter operator (Jinja2/Ansible/Helm style).
- Curated filter stdlib, **not** all of Sprig/Jinja2.

## Why these choices

The grammar is borrowed: namespace taxonomy from GitHub Actions, namespace dot-path from Terraform, filter pipelines from Ansible. Nothing in Angee's substitution layer is novel — all four design knobs match an existing convention that's familiar to most YAML authors.

| Decision | Choice | Why |
|---|---|---|
| Delimiter | `${...}` | Compose, Terraform, DevSpace all use this. `${{ }}` (GitHub Actions) buys nothing in YAML since `${` is already unambiguous, just adds two characters per reference. `{{ }}` (Helm/Ansible/Jinja2) **collides** with Copier's Jinja2 render pass — fatal for us. |
| Namespace separator | `.` | Reads as hierarchical navigation matching Terraform (`var.x`, `module.y.z`), GitHub Actions (`secrets.X`, `steps.x.outputs.y`), and every JSON-path convention. `:` is ambiguous with URIs (`http://`, `docker:`) and harder to spot next to a filter. |
| Filter operator | `\|` | Already understood from Ansible/Jinja2/Helm. Left-to-right reads as the transformation sequence. Function-call style (`f(g(x))`) reads inside-out and is harder to follow for chained string transforms. |
| Filter library | Small curated stdlib | A handful of useful filters (`slug`, `lower`, `upper`, `local_part`, `truncate`, `default`, `required`, `b64encode`). Not Sprig (~100 functions, security audit nightmare), not full Jinja2 (we already have Jinja2 in Copier — keep operator resolution typed and small). |

## Namespaces

Every reference begins with a namespace prefix that picks the resolver. This is the GitHub Actions / Terraform model: namespace = resolver type, path = resolver argument.

| Form | Resolver | Where used |
|---|---|---|
| `${secret.name}` | Secrets backend (`.env` or OpenBao) | Anywhere |
| `${connector.provider.user}` | Externally-registered connector resolver (Django) | Per-user agent templates |
| `${service.name}` | Stack service registry → `host:port`. **Host portion rewritten per consumer location** (host process / container / different network — see OVERVIEW "Address resolution") | Anywhere |
| `${service.name.host}` / `${service.name.port}` | Just the host or just the port (host portion rewritten per consumer) | Anywhere |
| `${ports.name}` | This manifest's `ports:` block | Anywhere |
| `${alloc.pool}` | Operator port pool, allocated at provisioning | Agent templates only |
| `${persist.<key>}` | Absolute path to a template-declared persistent dir (see template's `persist:` block) | Within the declaring template |
| `${operator.url}`, `${operator.domain}` | Operator context. **Host portion rewritten per consumer** (same rule as `service.*`) | Anywhere |
| `${workspace.root}`, `${workspace.name}`, `${workspace.code_path}` | Workspace context | Agent templates |
| `${inputs.key}` | Provisioning-time inputs from CLI/HTTP | Templates with `_angee.inputs` |
| `${name}` | The resolved instance name (no namespace prefix) | Anywhere a name is referenced |

Resolution is **always namespace-discriminated**. There is no implicit fallback chain across namespaces — `${FOO}` without a namespace prefix is an error, not a free env-var lookup. This avoids the Compose/DevSpace anti-pattern where every reference has to be hoisted to a flat env-var namespace and resolver order becomes a configuration footgun.

## Filters

Curated stdlib. Each filter takes the piped value as its first argument; additional arguments use parentheses.

| Filter | Behavior | Example |
|---|---|---|
| `slug` | Lowercase, hyphenate, strip non-alphanumerics | `${inputs.branch \| slug}` → `feat-issue-123` |
| `lower` / `upper` | Case folding | `${inputs.user \| upper}` |
| `local_part` | Email local part (before `@`) | `${inputs.user \| local_part}` → `alex` |
| `truncate(n)` | Truncate to `n` characters | `${inputs.branch \| slug \| truncate(40)}` |
| `default('x')` | Use `'x'` if value is empty/unset | `${inputs.repo \| default('../django-angee')}` |
| `required('msg')` | Fail with `'msg'` if value is empty/unset | `${secret.api-key \| required('set ANTHROPIC_API_KEY')}` |
| `b64encode` | Base64 encode | `${secret.token \| b64encode}` |
| `replace(a,b)` | Replace `a` with `b` | `${name \| replace('-','_')}` |

The set is small on purpose. We add new filters when a real template needs one, not preemptively.

## Pipeline composition

```yaml
# Email-derived agent name capped at 40 chars
agent_name: "${inputs.user | local_part | slug | truncate(40)}"

# Branch slugified, capped, with fallback when no branch given
worktree_dir: "${inputs.branch | slug | truncate(40) | default('default')}"

# Required secret with helpful error
ANTHROPIC_API_KEY: "${secret.anthropic-api-key | required('set ANTHROPIC_API_KEY in env or pass --secret')}"
```

The pipe sends the left value as the first positional argument of the right filter. Filters are pure — same input, same output, no side effects.

## Coexistence with Copier (Jinja2)

`copier.yml` and `template/` files use Jinja2 (`{{ }}` / `{% %}`). The rendered `angee.yaml` then contains `${...}` references the operator resolves later. The two layers don't collide because:

- Jinja2 uses `{{ }}` and `{% %}`, never `${`.
- Angee uses `${...}`, never `{{ }}` or `{% %}`.
- Copier renders first (template-time, static answers), operator resolves after (provision-time, dynamic refs).

If a template needs to emit a literal `${`, escape with `$${`.

## Concrete examples

### Stack template fragment
```yaml
secrets:
  postgres-password: { generated: true, length: 32 }

services:
  web:
    runtime: local
    env:
      DATABASE_URL: "postgres://postgres:${secret.postgres-password}@${service.postgres}/${name | replace('-','_')}"
      ANTHROPIC_API_KEY: "${secret.anthropic-api-key | required('set ANTHROPIC_API_KEY')}"
    ports:
      - { port: "${ports.django}" }
```

### Agent template fragment
```yaml
mcp_servers:
  operator:
    transport: streamable-http
    url: "${operator.url}/mcp"
    auth:
      bearer: "${secret.agent-operator-token}"
  inner-django:
    transport: http
    url: "http://${service.web}/mcp"

services:
  ${name}:
    env:
      ANGEE_AGENT_NAME: "${name}"
      ANGEE_AGENT_USER: "${inputs.user | local_part}"
      ANGEE_OPERATOR_URL: "${operator.url}"
      ANGEE_OPERATOR_TOKEN: "${secret.agent-operator-token}"
```

## Anti-patterns we explicitly reject

| Pattern | From | Why we reject |
|---|---|---|
| `$(VAR)` | Kustomize | Collides with shell command substitution; reads ambiguously. |
| `{{ ns.x }}` | Helm, Ansible | Collides with Copier's Jinja2 rendering pass. |
| `${{ ns.x }}` | GitHub Actions | Buys nothing in YAML; pure punctuation tax. |
| `$(command)` command substitution | bash | Code-injection vector in a config file. |
| `${VAR/foo/bar}` bash substring forms | Flux CD | Opaque to non-bash users; filter pipeline is strictly clearer. |
| Implicit resolver fallback (`${FOO}` tries env, then secret, then …) | none directly, but tempting | Resolution order becomes a hidden config; explicit namespaces are unambiguous. |
| Inline command output (`$(curl ...)`) | bash, devspace `command:` | Same security issue; if a runtime fetch is needed it must be a registered resolver, not a shell. |

## Open questions

1. **YAML quoting**: `${...}` must be inside a YAML string for the parser to be happy when the value starts with `$`. Templates should always quote substituted values: `key: "${ns.x}"`. Operator should validate.
2. **Type preservation**: `${ports.django}` is conceptually a number. After substitution into a string it's `"8100"`. For places that need an int (compose `ports:`, env vars are fine as strings), the operator's compiler must do the right cast based on schema.
3. **Recursive references**: `${secret.x}` where `x` itself contains `${secret.y}`. v1 forbids it — secrets resolve to literal strings, period.
4. **Default values with substitutions inside**: `${inputs.x | default('${operator.url}')}` — allowed or not? Probably not in v1; the default arg is a literal string.

These get nailed down by the resolver implementation, not the grammar.

## Status

- [x] Grammar proposed.
- [ ] Implemented in resolver (`internal/resolve/`).
- [ ] Migrate templates from `${secret:x}` → `${secret.x}`.
- [ ] Validation pass: parser rejects unknown namespaces with a useful error.
- [ ] Filter stdlib: implement the 8 filters above with tests.
