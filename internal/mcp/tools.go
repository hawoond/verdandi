package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/genie-cvc/verdandi/internal/verdandi"
)

type Executor interface {
	Execute(name string, args map[string]any) (map[string]any, error)
}

type progressExecutor interface {
	ExecuteWithProgress(name string, args map[string]any, reporter verdandi.ProgressReporter) (map[string]any, error)
}

type contextExecutor interface {
	ExecuteWithContext(ctx context.Context, name string, args map[string]any, reporter verdandi.ProgressReporter) (map[string]any, error)
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
	Meta      requestMeta    `json:"_meta,omitempty"`
}

type requestMeta struct {
	ProgressToken any `json:"progressToken,omitempty"`
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage, emit func(any) error) (map[string]any, error) {
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

	structured, err := s.executeToolPayload(ctx, payload, emit)
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

func (s *Server) executeToolPayload(ctx context.Context, payload callToolParams, emit func(any) error) (map[string]any, error) {
	progressToken, hasProgressToken := normalizeProgressToken(payload.Meta.ProgressToken)
	if executor, ok := s.executor.(contextExecutor); ok {
		if !hasProgressToken || emit == nil {
			return executor.ExecuteWithContext(ctx, payload.Name, payload.Arguments, nil)
		}
		return executor.ExecuteWithContext(ctx, payload.Name, payload.Arguments, func(event verdandi.ProgressEvent) {
			_ = emit(progressNotification(progressToken, event))
		})
	}
	if !hasProgressToken || emit == nil {
		return s.executor.Execute(payload.Name, payload.Arguments)
	}
	executor, ok := s.executor.(progressExecutor)
	if !ok {
		return s.executor.Execute(payload.Name, payload.Arguments)
	}
	return executor.ExecuteWithProgress(payload.Name, payload.Arguments, func(event verdandi.ProgressEvent) {
		_ = emit(progressNotification(progressToken, event))
	})
}

func normalizeProgressToken(value any) (any, bool) {
	switch token := value.(type) {
	case nil:
		return nil, false
	case string:
		if token == "" {
			return nil, false
		}
		return token, true
	case float64:
		return token, true
	case int:
		return token, true
	case int64:
		return token, true
	case json.Number:
		return token.String(), true
	default:
		return token, true
	}
}

func progressNotification(progressToken any, event verdandi.ProgressEvent) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/progress",
		"params": map[string]any{
			"progressToken": progressToken,
			"progress":      event.Progress,
			"total":         event.Total,
			"message":       event.Message,
		},
	}
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
	stageSchema := stageArraySchema()
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
			Name:        "prepare_workflow",
			Title:       "Prepare Verdandi workflow",
			Description: "Create or reuse persistent agent and skill assets, then return a workflow handoff package for the external LLM coding agent.",
			InputSchema: objectSchema([]string{"request"}, map[string]any{
				"request": map[string]any{"type": "string"},
				"options": map[string]any{"type": "object"},
			}),
		},
		{
			Name:        "recommend_assets",
			Title:       "Recommend Verdandi assets",
			Description: "List existing reusable agent and skill assets relevant to a request.",
			InputSchema: objectSchema([]string{"request"}, map[string]any{
				"request": map[string]any{"type": "string"},
			}),
		},
		{
			Name:        "record_outcome",
			Title:       "Record asset outcome",
			Description: "Record success, failure, and lessons for a persistent agent or skill asset after an external coding agent uses it.",
			InputSchema: objectSchema([]string{"assetId", "kind", "status"}, map[string]any{
				"assetId": map[string]any{"type": "string"},
				"kind":    map[string]any{"type": "string", "enum": []string{"agent", "skill"}},
				"status":  map[string]any{"type": "string", "enum": []string{"success", "error"}},
				"runId":   map[string]any{"type": "string"},
				"error":   map[string]any{"type": "string"},
				"lesson":  map[string]any{"type": "string"},
			}),
		},
		{
			Name:        "run_plan",
			Title:       "Run client-selected plan",
			Description: "Run an ordered Verdandi stage plan selected by the MCP client after Verdandi validates and normalizes it.",
			InputSchema: objectSchema([]string{"request", "stages"}, map[string]any{
				"request": map[string]any{"type": "string"},
				"stages":  stageSchema,
				"options": map[string]any{"type": "object"},
			}),
		},
		{
			Name:        "validate_plan",
			Title:       "Validate client-selected plan",
			Description: "Validate and normalize an ordered Verdandi stage plan without executing it.",
			InputSchema: objectSchema([]string{"request", "stages"}, map[string]any{
				"request": map[string]any{"type": "string"},
				"stages":  stageSchema,
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
		{
			Name:        "list_skills",
			Title:       "List Verdandi skills",
			Description: "List persisted skill assets and lifecycle status.",
			InputSchema: objectSchema([]string{}, map[string]any{}),
		},
	}
}

func stageArraySchema() map[string]any {
	return map[string]any{
		"type":     "array",
		"minItems": 1,
		"items": objectSchema([]string{"stage", "keyword"}, map[string]any{
			"stage": map[string]any{
				"type": "string",
				"enum": []string{"planner", "code-writer", "tester", "documenter", "deployer"},
			},
			"keyword": map[string]any{"type": "string"},
		}),
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
