package mcp

import "encoding/json"

func (s *Server) initializeResult(params json.RawMessage) map[string]any {
	clientVersion := protocolVersion
	if len(params) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(params, &payload); err == nil {
			if value, ok := payload["protocolVersion"].(string); ok && value != "" {
				clientVersion = value
			}
		}
	}

	return map[string]any{
		"protocolVersion": clientVersion,
		"capabilities": map[string]any{
			"tools":       map[string]any{"listChanged": false},
			"resources":   map[string]any{"subscribe": false, "listChanged": false},
			"prompts":     map[string]any{"listChanged": false},
			"completions": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":        "verdandi-mcp",
			"title":       "Verdandi MCP Server",
			"version":     "0.1.0",
			"description": "Pure Go MCP server for Verdandi orchestration.",
		},
		"instructions": "For ordinary user requests, call run with the user's natural-language request. Use analyze only when the user explicitly asks to inspect the plan before execution. Use validate_plan and run_plan when the MCP client chooses workflow stages.",
	}
}
