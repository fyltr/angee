// Package credentials defines the pluggable secrets backend interface.
package credentials

import "context"

// Backend is the pluggable secrets resolution interface.
// The operator uses this to resolve ${secret:name} references and to manage
// credentials via the REST API and MCP tools.
type Backend interface {
	// Get retrieves a secret value by name.
	Get(ctx context.Context, name string) (string, error)

	// Set stores a secret value.
	Set(ctx context.Context, name string, value string) error

	// Delete removes a secret.
	Delete(ctx context.Context, name string) error

	// List returns all secret names (not values) under the configured scope.
	List(ctx context.Context) ([]string, error)

	// Type returns the backend type name ("env" or "openbao").
	Type() string
}
