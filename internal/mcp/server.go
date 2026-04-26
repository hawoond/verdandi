package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const protocolVersion = "2025-11-25"

type Executor interface {
	Execute(name string, args map[string]any) (map[string]any, error)
}

type Server struct {
	executor Executor
	tools    []Tool
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id,omitempty"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func NewServer(executor Executor) *Server {
	return &Server{executor: executor, tools: defaultTools()}
}

func (s *Server) Serve(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	writer := bufio.NewWriter(output)
	defer writer.Flush()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		response, shouldRespond := s.HandleLine([]byte(line))
		if !shouldRespond {
			continue
		}

		encoded, err := json.Marshal(response)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func (s *Server) HandleLine(line []byte) (JSONRPCResponse, bool) {
	var request JSONRPCRequest
	if err := json.Unmarshal(line, &request); err != nil {
		return errorResponse(nil, -32700, "Parse error", err.Error()), true
	}
	if request.JSONRPC != "2.0" {
		return errorResponse(request.ID, -32600, "Invalid Request", "jsonrpc must be 2.0"), true
	}
	if request.ID == nil {
		return JSONRPCResponse{}, false
	}

	switch request.Method {
	case "initialize":
		return resultResponse(request.ID, s.initializeResult(request.Params)), true
	case "ping":
		return resultResponse(request.ID, map[string]any{}), true
	case "tools/list":
		return resultResponse(request.ID, map[string]any{"tools": s.tools}), true
	case "tools/call":
		result, err := s.callTool(request.Params)
		if err != nil {
			var rpcErr *JSONRPCError
			if errors.As(err, &rpcErr) {
				return JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: rpcErr}, true
			}
			return errorResponse(request.ID, -32603, "Internal error", err.Error()), true
		}
		return resultResponse(request.ID, result), true
	default:
		return errorResponse(request.ID, -32601, "Method not found", request.Method), true
	}
}

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
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":        "verdandi-mcp",
			"title":       "Verdandi MCP Server",
			"version":     "0.1.0",
			"description": "Pure Go MCP server for Verdandi orchestration.",
		},
		"instructions": "For ordinary user requests, call run with the user's natural-language request. Use analyze only when the user explicitly asks to inspect the plan before execution.",
	}
}

func (s *Server) callTool(params json.RawMessage) (map[string]any, error) {
	var payload callToolParams
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: err.Error()}
	}
	if payload.Name == "" {
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: "tool name is required"}
	}
	if !s.hasTool(payload.Name) {
		return nil, &JSONRPCError{Code: -32602, Message: "Unknown tool", Data: payload.Name}
	}
	if payload.Arguments == nil {
		payload.Arguments = map[string]any{}
	}

	structured, err := s.executor.Execute(payload.Name, payload.Arguments)
	if err != nil {
		return map[string]any{
			"isError": true,
			"content": []map[string]any{
				{"type": "text", "text": err.Error()},
			},
			"structuredContent": map[string]any{
				"ok":    false,
				"error": err.Error(),
			},
		}, nil
	}

	text, err := json.MarshalIndent(structured, "", "  ")
	if err != nil {
		text = []byte(fmt.Sprintf("%v", structured))
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(text)},
		},
		"structuredContent": structured,
	}, nil
}

func (s *Server) hasTool(name string) bool {
	for _, tool := range s.tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func defaultTools() []Tool {
	return []Tool{
		{
			Name:        "run",
			Title:       "Run Verdandi",
			Description: "Default tool for natural-language task requests. It analyzes the request, creates the needed agent workflow, executes it, and returns the run summary.",
			InputSchema: objectSchema([]string{"request"}, map[string]any{
				"request": map[string]any{"type": "string", "description": "The user's natural-language task request."},
				"options": map[string]any{"type": "object"},
			}),
		},
		{
			Name:        "analyze",
			Title:       "Analyze request",
			Description: "Analyze a natural-language request and return the selected agent contract without executing the workflow.",
			InputSchema: objectSchema([]string{"request"}, map[string]any{
				"request": map[string]any{"type": "string"},
			}),
		},
		{
			Name:        "orchestrate",
			Title:       "Run Verdandi orchestration",
			Description: "Compatibility alias for run.",
			InputSchema: objectSchema([]string{"request"}, map[string]any{
				"request": map[string]any{"type": "string"},
				"options": map[string]any{"type": "object"},
			}),
		},
		{
			Name:        "get_status",
			Title:       "Get run status",
			Description: "Look up a previous Verdandi run by runId.",
			InputSchema: objectSchema([]string{"runId"}, map[string]any{
				"runId": map[string]any{"type": "string"},
			}),
		},
		{
			Name:        "open_output",
			Title:       "Open run output",
			Description: "List generated output files for a previous Verdandi run.",
			InputSchema: objectSchema([]string{"runId"}, map[string]any{
				"runId": map[string]any{"type": "string"},
			}),
		},
		{
			Name:        "list_agents",
			Title:       "List Verdandi agents",
			Description: "List persisted dynamic agent contracts and lifecycle policy options.",
			InputSchema: objectSchema([]string{}, map[string]any{}),
		},
	}
}

func objectSchema(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             required,
		"properties":           properties,
		"additionalProperties": false,
	}
}

func resultResponse(id any, result any) JSONRPCResponse {
	return JSONRPCResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id any, code int, message string, data any) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message, Data: data},
	}
}
