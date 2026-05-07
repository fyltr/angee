# Removed Operator And Deploy Snippets

## Deploy Message

Removed active CLI/API plumbing:

```go
var deployMessage string

func init() {
    deployCmd.Flags().StringVarP(&deployMessage, "message", "m", "", "Commit message for this deploy")
}

type DeployRequest struct {
    Message string `json:"message,omitempty"`
}

if _, err := apiPost("/deploy", api.DeployRequest{Message: deployMessage}, &result); err != nil {
    return fmt.Errorf("deploying: %w", err)
}
```

Reason removed: `Deploy` no longer creates a git/config commit, so this message had no concrete state record to attach to.

## MCP Credential Stubs

Removed active dispatch cases:

```go
case "credentials_list":
    return nil, fmt.Errorf("credentials backend not configured")
case "credentials_set":
    return nil, fmt.Errorf("credentials backend not configured")
case "credentials_delete":
    return nil, fmt.Errorf("credentials backend not configured")
```

Reason removed: connectors and credentials are application-managed. Angee currently resolves template-declared secrets; it does not expose a fake credentials backend.

## Operator Config Fields

Removed active fields:

```go
type OperatorConfig struct {
    DjangoURL string `yaml:"django_url,omitempty"`
}

type DockerConfig struct {
    Socket string `yaml:"socket,omitempty"`
}
```

Reason removed: no active code read these fields. State sources and backend settings belong in `$ANGEE_ROOT/angee.yaml` or active backend implementations.

## Reconcile Follow Flag

Removed active field:

```go
type ReconcileRequest struct {
    Follow bool `json:"follow,omitempty"`
}
```

Reason removed: `angee dev` now owns foreground log streaming through the dev sink; reconcile itself does not need an unused follow flag.
