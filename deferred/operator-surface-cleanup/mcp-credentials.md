# MCP Credentials Archive

Removed from active code because Angee does not currently own connector or credential management. Active Angee only resolves template-declared secrets from `$ANGEE_ROOT/angee.yaml` and file-backed state.

Removed active request type:

```go
type CredentialSetRequest struct {
    Name  string `json:"name"`
    Value string `json:"value"`
}
```

Removed active MCP dispatch stubs:

```go
case "credentials_list":
    return nil, fmt.Errorf("credentials backend not configured")
case "credentials_set":
    return nil, fmt.Errorf("credentials backend not configured")
case "credentials_delete":
    return nil, fmt.Errorf("credentials backend not configured")
```

Future shape, if this returns:

- Credential tools must be backed by a real credential backend, not a placeholder operator branch.
- Connector credentials should remain application-managed unless the stack explicitly declares an Angee-owned secret.
- MCP tools should distinguish stack secrets from application connector credentials.
- Secret values must never be returned in `credentials_list`; return metadata only.
- `credentials_set` should support values from `env:VAR`, `file:PATH`, and generated values, matching the existing secret input grammar.
- Auditability should come from the owning control plane: file state for local stacks, application DB/API for app-managed connectors.

Possible future MCP tool names:

```text
stack_secret_list
stack_secret_set
stack_secret_delete
connector_credential_list
connector_credential_set
connector_credential_delete
```

Do not restore the old generic `credentials_*` names unless the implementation makes the ownership boundary explicit.
