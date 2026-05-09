package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPContractSnapshot(t *testing.T) {
	got := marshalContractSnapshot(t, currentMCPContractSnapshot(t))
	path := filepath.Join("..", "..", "docs", "mcp-contract-snapshot.json")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read MCP contract snapshot: %v", err)
	}
	if strings.TrimSpace(string(want)) != strings.TrimSpace(string(got)) {
		t.Fatalf("MCP contract snapshot changed; update %s intentionally\n--- want\n%s\n--- got\n%s", path, want, got)
	}
}

func currentMCPContractSnapshot(t *testing.T) map[string]any {
	t.Helper()

	server := NewServer(&fakeExecutor{})
	return map[string]any{
		"protocolVersion": protocolVersion,
		"initialize":      contractResult(t, server, `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25"}}`),
		"tools":           contractResult(t, server, `{"jsonrpc":"2.0","id":"tools","method":"tools/list"}`)["tools"],
		"resources":       contractResult(t, server, `{"jsonrpc":"2.0","id":"resources","method":"resources/list"}`)["resources"],
		"resourceTemplates": contractResult(t, server,
			`{"jsonrpc":"2.0","id":"templates","method":"resources/templates/list"}`)["resourceTemplates"],
		"prompts": contractResult(t, server, `{"jsonrpc":"2.0","id":"prompts","method":"prompts/list"}`)["prompts"],
		"completionReferences": []map[string]any{
			{"ref": map[string]string{"type": "ref/prompt", "name": "plan-and-run"}, "argument": "request"},
			{"ref": map[string]string{"type": "ref/prompt", "name": "validate-plan"}, "argument": "request"},
			{"ref": map[string]string{"type": "ref/prompt", "name": "choose-agent-lifecycle"}, "argument": "request"},
			{"ref": map[string]string{"type": "ref/prompt", "name": "inspect-run"}, "argument": "runId"},
			{"ref": map[string]string{"type": "ref/prompt", "name": "inspect-failed-run"}, "argument": "runId"},
			{"ref": map[string]string{"type": "ref/resource", "uri": "verdandi://runs/{runId}"}, "argument": "runId"},
			{"ref": map[string]string{"type": "ref/resource", "uri": "verdandi://runs/{runId}/events"}, "argument": "runId"},
			{"ref": map[string]string{"type": "ref/resource", "uri": "verdandi://runs/{runId}/output"}, "argument": "runId"},
			{"ref": map[string]string{"type": "ref/resource", "uri": "verdandi://workflows/{runId}"}, "argument": "runId"},
			{"ref": map[string]string{"type": "ref/resource", "uri": "verdandi://workflows/{runId}/handoff"}, "argument": "runId"},
		},
		"transports": map[string]any{
			"stdio": map[string]any{
				"batch":        true,
				"cancellation": true,
				"progress":     true,
			},
			"streamableHTTP": map[string]any{
				"endpoint":            "/mcp",
				"post":                true,
				"sseProgress":         true,
				"originValidation":    true,
				"optionalBearerToken": true,
				"optionalSession":     true,
				"standaloneGET":       false,
			},
		},
	}
}

func contractResult(t *testing.T, server *Server, line string) map[string]any {
	t.Helper()

	response, shouldRespond := server.HandleMessage([]byte(line))
	if !shouldRespond {
		t.Fatalf("contract request should produce a response: %s", line)
	}
	encoded := marshalContractSnapshot(t, response)
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode contract response: %v", err)
	}
	if payload["error"] != nil {
		t.Fatalf("contract request returned error: %#v", payload["error"])
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("contract response missing result: %#v", payload)
	}
	return result
}

func marshalContractSnapshot(t *testing.T, value any) []byte {
	t.Helper()

	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal contract snapshot: %v", err)
	}
	return append(encoded, '\n')
}
