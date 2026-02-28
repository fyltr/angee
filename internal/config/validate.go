package config

import (
	"fmt"
	"strings"
)

// validLifecycles is the set of recognised lifecycle values.
var validLifecycles = map[string]bool{
	"":                 true, // empty is allowed (defaults applied later)
	LifecyclePlatform:  true,
	LifecycleSidecar:   true,
	LifecycleWorker:    true,
	LifecycleSystem:    true,
	LifecycleAgent:     true,
	LifecycleJob:       true,
}

// Validate checks an AngeeConfig for structural errors. It collects all
// problems rather than returning on the first failure.
func (c *AngeeConfig) Validate() error {
	var errs []string

	if c.Name == "" {
		errs = append(errs, "name is required")
	}

	// Service lifecycles
	for name, svc := range c.Services {
		if !validLifecycles[svc.Lifecycle] {
			errs = append(errs, fmt.Sprintf("service %q: invalid lifecycle %q", name, svc.Lifecycle))
		}
	}

	// Agent lifecycles + MCP server references + file mounts
	for name, agent := range c.Agents {
		if !validLifecycles[agent.Lifecycle] {
			errs = append(errs, fmt.Sprintf("agent %q: invalid lifecycle %q", name, agent.Lifecycle))
		}
		for _, ref := range agent.MCPServers {
			if _, ok := c.MCPServers[ref]; !ok {
				errs = append(errs, fmt.Sprintf("agent %q: mcp_server %q is not defined in mcp_servers", name, ref))
			}
		}
		for i, f := range agent.Files {
			hasTemplate := f.Template != ""
			hasSource := f.Source != ""
			if hasTemplate == hasSource {
				errs = append(errs, fmt.Sprintf("agent %q: files[%d] must have exactly one of template or source", name, i))
			}
			if f.Mount == "" {
				errs = append(errs, fmt.Sprintf("agent %q: files[%d] mount is required", name, i))
			}
		}
	}

	// Domain conflicts: same host across services
	type domainKey struct {
		host string
		port int
	}
	domains := make(map[domainKey]string)
	for name, svc := range c.Services {
		for _, d := range svc.Domains {
			port := d.Port
			if port == 0 {
				port = 8000
			}
			key := domainKey{host: d.Host, port: port}
			if other, exists := domains[key]; exists {
				errs = append(errs, fmt.Sprintf("domain conflict: %s:%d claimed by both %q and %q", d.Host, port, other, name))
			} else {
				domains[key] = name
			}
		}
	}

	// Host port conflicts: same host port on two services
	hostPorts := make(map[string]string)
	for name, svc := range c.Services {
		for _, p := range svc.Ports {
			// Port format is "host:container" or just "container"
			hostPort := portHostPart(p)
			if hostPort == "" {
				continue
			}
			if other, exists := hostPorts[hostPort]; exists {
				errs = append(errs, fmt.Sprintf("port conflict: host port %s used by both %q and %q", hostPort, other, name))
			} else {
				hostPorts[hostPort] = name
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// portHostPart extracts the host port from a docker port mapping string.
// "8080:80" → "8080", "80" → "" (no host port), "0.0.0.0:8080:80" → "0.0.0.0:8080".
func portHostPart(mapping string) string {
	parts := strings.Split(mapping, ":")
	switch len(parts) {
	case 1:
		return "" // container-only, no host port
	case 2:
		return parts[0]
	case 3:
		return parts[0] + ":" + parts[1]
	default:
		return ""
	}
}
