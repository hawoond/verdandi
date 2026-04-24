package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type fakeExecutor struct {
	calls []ToolCall
}

func (f *fakeExecutor) Execute(name string, args map[string]any) (map[string]any, error) {
	f.calls = append(f.calls, ToolCall{Name: name, Args: args})
	return map[string]any{
		"ok":     true,
		"action": name,
		"echo":   args,
	}, nil
}

type ToolCall struct {
	Name string
	Args map[string]any
}

func TestToolsListExposesRunAsDefaultTool(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"tools","method":"tools/list"}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	tools := result["tools"].([]any)

	names := map[string]bool{}
	for _, rawTool := range tools {
		tool := rawTool.(map[string]any)
		names[tool["name"].(string)] = true
	}

	for _, name := range []string{"run", "analyze", "orchestrate", "get_status", "open_output"} {
		if !names[name] {
			t.Fatalf("missing MCP tool %q in %#v", name, names)
		}
	}
}

func TestRunToolForwardsNaturalLanguageRequest(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"run","arguments":{"request":"계산기 앱을 기획하고 구현하고 테스트해줘"}}}` + "\n"
	var output bytes.Buffer
	executor := &fakeExecutor{}

	server := NewServer(executor)
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	if len(executor.calls) != 1 {
		t.Fatalf("expected 1 executor call, got %d", len(executor.calls))
	}
	if executor.calls[0].Name != "run" {
		t.Fatalf("expected run call, got %q", executor.calls[0].Name)
	}
	if executor.calls[0].Args["request"] != "계산기 앱을 기획하고 구현하고 테스트해줘" {
		t.Fatalf("request was not forwarded: %#v", executor.calls[0].Args)
	}
}

func TestInitializeReturnsCapabilities(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	if result["protocolVersion"] != "2025-11-25" {
		t.Fatalf("unexpected protocol version: %v", result["protocolVersion"])
	}
}

func decodeResponses(t *testing.T, output string) []map[string]any {
	t.Helper()

	var responses []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("response is not JSON: %v\nline: %s", err, line)
		}
		responses = append(responses, response)
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan responses: %v", err)
	}

	return responses
}
