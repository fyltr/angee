package operator

import (
	"encoding/json"
	"net/http"
)

// handleOpenAPI returns the OpenAPI 3.1 schema for the operator HTTP API.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openapiSpec()) //nolint:errcheck
}

func openapiSpec() map[string]any {
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Angee Operator API",
			"version":     "0.1.0",
			"description": "HTTP API for the angee-operator. Manages services, agents, and platform lifecycle.",
		},
		"servers": []map[string]any{
			{"url": "http://localhost:9000", "description": "Local operator"},
		},
		"security": []map[string]any{
			{"bearerAuth": []any{}},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":   "http",
					"scheme": "bearer",
				},
			},
			"schemas": openapiSchemas(),
		},
		"paths": openapiPaths(),
	}
}

func openapiSchemas() map[string]any {
	return map[string]any{
		"Error": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{"type": "string"},
			},
		},
		"HealthResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":  map[string]any{"type": "string"},
				"root":    map[string]any{"type": "string"},
				"runtime": map[string]any{"type": "string"},
			},
		},
		"ApplyResult": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"services_started": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"services_updated": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"services_removed": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		"ChangeSet": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"add":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"update": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"remove": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		"RollbackRequest": map[string]any{
			"type":     "object",
			"required": []string{"sha"},
			"properties": map[string]any{
				"sha": map[string]any{"type": "string"},
			},
		},
		"RollbackResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rolled_back_to": map[string]any{"type": "string"},
				"deploy":         map[string]any{"$ref": "#/components/schemas/ApplyResult"},
			},
		},
		"ScaleRequest": map[string]any{
			"type":     "object",
			"required": []string{"replicas"},
			"properties": map[string]any{
				"replicas": map[string]any{"type": "integer", "minimum": 0},
			},
		},
		"ScaleResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"service":  map[string]any{"type": "string"},
				"replicas": map[string]any{"type": "integer"},
			},
		},
		"DownResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string"},
			},
		},
		"ServiceStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":             map[string]any{"type": "string"},
				"type":             map[string]any{"type": "string", "enum": []string{"service", "agent"}},
				"status":           map[string]any{"type": "string", "enum": []string{"running", "stopped", "error", "starting"}},
				"health":           map[string]any{"type": "string", "enum": []string{"healthy", "unhealthy", "unknown"}},
				"container_id":     map[string]any{"type": "string"},
				"image":            map[string]any{"type": "string"},
				"replicas_running": map[string]any{"type": "integer"},
				"replicas_desired": map[string]any{"type": "integer"},
				"last_updated":     map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"AgentInfo": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"type": "string"},
				"lifecycle": map[string]any{"type": "string"},
				"role":      map[string]any{"type": "string"},
				"status":    map[string]any{"type": "string"},
				"health":    map[string]any{"type": "string"},
			},
		},
		"AgentActionResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string"},
				"agent":  map[string]any{"type": "string"},
			},
		},
		"ConfigSetRequest": map[string]any{
			"type":     "object",
			"required": []string{"content"},
			"properties": map[string]any{
				"content": map[string]any{"type": "string", "description": "Raw YAML content"},
				"message": map[string]any{"type": "string", "description": "Git commit message"},
				"deploy":  map[string]any{"type": "boolean", "description": "Deploy immediately after saving"},
			},
		},
		"ConfigSetResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sha":     map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
				"deploy":  map[string]any{"$ref": "#/components/schemas/ApplyResult"},
			},
		},
		"CommitInfo": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sha":     map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
				"author":  map[string]any{"type": "string"},
				"date":    map[string]any{"type": "string"},
			},
		},
		"DeployRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
		"JSONRPCRequest": map[string]any{
			"type":     "object",
			"required": []string{"jsonrpc", "method"},
			"properties": map[string]any{
				"jsonrpc": map[string]any{"type": "string", "const": "2.0"},
				"method":  map[string]any{"type": "string"},
				"params":  map[string]any{"type": "object"},
				"id":      map[string]any{},
			},
		},
		"JSONRPCResponse": map[string]any{
			"type":     "object",
			"required": []string{"jsonrpc"},
			"properties": map[string]any{
				"jsonrpc": map[string]any{"type": "string", "const": "2.0"},
				"result":  map[string]any{},
				"error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code":    map[string]any{"type": "integer"},
						"message": map[string]any{"type": "string"},
					},
				},
				"id": map[string]any{},
			},
		},
	}
}

func openapiPaths() map[string]any {
	errResp := map[string]any{
		"description": "Error",
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/Error"},
			},
		},
	}

	return map[string]any{
		"/health": map[string]any{
			"get": map[string]any{
				"summary":     "Health check",
				"operationId": "health",
				"security":    []any{},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Operator is healthy",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/HealthResponse"},
							},
						},
					},
				},
			},
		},
		"/config": map[string]any{
			"get": map[string]any{
				"summary":     "Get current angee.yaml",
				"operationId": "configGet",
				"responses": map[string]any{
					"200": map[string]any{"description": "Current configuration as JSON"},
					"500": errResp,
				},
			},
			"post": map[string]any{
				"summary":     "Set angee.yaml content",
				"operationId": "configSet",
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/ConfigSetRequest"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Config saved",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ConfigSetResponse"},
							},
						},
					},
					"400": errResp,
					"500": errResp,
				},
			},
		},
		"/deploy": map[string]any{
			"post": map[string]any{
				"summary":     "Deploy angee.yaml",
				"operationId": "deploy",
				"requestBody": map[string]any{
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/DeployRequest"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Deployment result",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ApplyResult"},
							},
						},
					},
					"500": errResp,
				},
			},
		},
		"/plan": map[string]any{
			"get": map[string]any{
				"summary":     "Dry-run deploy",
				"operationId": "plan",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Planned changes",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ChangeSet"},
							},
						},
					},
					"500": errResp,
				},
			},
		},
		"/rollback": map[string]any{
			"post": map[string]any{
				"summary":     "Roll back to previous commit",
				"operationId": "rollback",
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/RollbackRequest"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Rollback result",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/RollbackResponse"},
							},
						},
					},
					"400": errResp,
					"500": errResp,
				},
			},
		},
		"/status": map[string]any{
			"get": map[string]any{
				"summary":     "Runtime status of all services",
				"operationId": "status",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Service statuses",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/ServiceStatus"},
								},
							},
						},
					},
					"500": errResp,
				},
			},
		},
		"/logs/{service}": map[string]any{
			"get": map[string]any{
				"summary":     "Service logs",
				"operationId": "logs",
				"parameters": []map[string]any{
					{"name": "service", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
					{"name": "lines", "in": "query", "schema": map[string]any{"type": "integer", "default": 100}},
					{"name": "follow", "in": "query", "schema": map[string]any{"type": "boolean", "default": false}},
					{"name": "since", "in": "query", "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Log output",
						"content":     map[string]any{"text/plain": map[string]any{"schema": map[string]any{"type": "string"}}},
					},
					"500": errResp,
				},
			},
		},
		"/scale/{service}": map[string]any{
			"post": map[string]any{
				"summary":     "Scale a service",
				"operationId": "scale",
				"parameters": []map[string]any{
					{"name": "service", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/ScaleRequest"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Scale result",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ScaleResponse"},
							},
						},
					},
					"400": errResp,
					"500": errResp,
				},
			},
		},
		"/down": map[string]any{
			"post": map[string]any{
				"summary":     "Bring stack down",
				"operationId": "down",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Stack stopped",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/DownResponse"},
							},
						},
					},
					"500": errResp,
				},
			},
		},
		"/agents": map[string]any{
			"get": map[string]any{
				"summary":     "List agents",
				"operationId": "agentList",
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Agent list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/AgentInfo"},
								},
							},
						},
					},
					"500": errResp,
				},
			},
		},
		"/agents/{name}/start": map[string]any{
			"post": map[string]any{
				"summary":     "Start an agent",
				"operationId": "agentStart",
				"parameters": []map[string]any{
					{"name": "name", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Agent started",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/ApplyResult"},
							},
						},
					},
					"404": errResp,
					"500": errResp,
				},
			},
		},
		"/agents/{name}/stop": map[string]any{
			"post": map[string]any{
				"summary":     "Stop an agent",
				"operationId": "agentStop",
				"parameters": []map[string]any{
					{"name": "name", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Agent stopped",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/AgentActionResponse"},
							},
						},
					},
					"500": errResp,
				},
			},
		},
		"/agents/{name}/logs": map[string]any{
			"get": map[string]any{
				"summary":     "Agent logs",
				"operationId": "agentLogs",
				"parameters": []map[string]any{
					{"name": "name", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
					{"name": "lines", "in": "query", "schema": map[string]any{"type": "integer", "default": 200}},
					{"name": "follow", "in": "query", "schema": map[string]any{"type": "boolean", "default": false}},
					{"name": "since", "in": "query", "schema": map[string]any{"type": "string"}},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Log output",
						"content":     map[string]any{"text/plain": map[string]any{"schema": map[string]any{"type": "string"}}},
					},
					"500": errResp,
				},
			},
		},
		"/history": map[string]any{
			"get": map[string]any{
				"summary":     "Config change history",
				"operationId": "history",
				"parameters": []map[string]any{
					{"name": "n", "in": "query", "schema": map[string]any{"type": "integer", "default": 20}},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "Commit list",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":  "array",
									"items": map[string]any{"$ref": "#/components/schemas/CommitInfo"},
								},
							},
						},
					},
				},
			},
		},
		"/mcp": map[string]any{
			"post": map[string]any{
				"summary":     "MCP endpoint (JSON-RPC 2.0)",
				"operationId": "mcp",
				"description": "JSON-RPC 2.0 endpoint for MCP tool calls. Supports initialize, tools/list, and tools/call methods.",
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{"$ref": "#/components/schemas/JSONRPCRequest"},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "JSON-RPC response",
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/JSONRPCResponse"},
							},
						},
					},
				},
			},
		},
		"/openapi.json": map[string]any{
			"get": map[string]any{
				"summary":     "OpenAPI schema",
				"operationId": "openapi",
				"security":    []any{},
				"responses": map[string]any{
					"200": map[string]any{
						"description": "OpenAPI 3.1 schema",
						"content":     map[string]any{"application/json": map[string]any{}},
					},
				},
			},
		},
	}
}
