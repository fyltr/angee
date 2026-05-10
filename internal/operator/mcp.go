package operator

func mcpDescriptor() map[string]any {
	return map[string]any{
		"name":    "angee-operator",
		"version": "0.1",
		"tools": []string{
			"stack.status",
			"stack.up",
			"stack.down",
			"services.create",
			"workspaces.create",
			"sources.fetch",
		},
	}
}
