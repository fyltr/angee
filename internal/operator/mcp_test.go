package operator

import (
	"encoding/json"
	"testing"
)

func TestMCPToolDefinitions(t *testing.T) {
	tools := mcpToolDefinitions()
	if len(tools) == 0 {
		t.Fatal("expected at least one tool definition")
	}

	// Check that all expected tools are present
	expected := map[string]bool{
		"platform_health": false,
		"platform_status": false,
		"config_get":      false,
		"config_set":      false,
		"deploy":          false,
		"deploy_plan":     false,
		"deploy_rollback": false,
		"service_logs":    false,
		"service_scale":   false,
		"platform_down":   false,
		"agent_list":      false,
		"agent_start":     false,
		"agent_stop":      false,
		"agent_logs":      false,
		"history":         false,
	}

	for _, tool := range tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil input schema", tool.Name)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected tool %q not found in definitions", name)
		}
	}
}

func TestMCPToolDefinitionsSerializable(t *testing.T) {
	tools := mcpToolDefinitions()
	data, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("failed to marshal tool definitions: %v", err)
	}
	if len(data) < 100 {
		t.Errorf("serialized tool definitions suspiciously short: %d bytes", len(data))
	}
}

func TestJSONRPCResponseFormat(t *testing.T) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  map[string]string{"status": "ok"},
		ID:      1,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", parsed["jsonrpc"])
	}
	if parsed["id"] != float64(1) {
		t.Errorf("id = %v, want 1", parsed["id"])
	}
	if parsed["error"] != nil {
		t.Error("expected no error field")
	}
}

func TestJSONRPCErrorFormat(t *testing.T) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		Error:   &jsonRPCError{Code: -32601, Message: "method not found"},
		ID:      "abc",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error object")
	}
	if errObj["code"] != float64(-32601) {
		t.Errorf("error code = %v, want -32601", errObj["code"])
	}
	if errObj["message"] != "method not found" {
		t.Errorf("error message = %v", errObj["message"])
	}
}

func TestMCPToolCallParamsParsing(t *testing.T) {
	raw := `{"name": "deploy", "arguments": {}}`
	var params mcpToolCallParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatal(err)
	}
	if params.Name != "deploy" {
		t.Errorf("name = %q, want %q", params.Name, "deploy")
	}
}
