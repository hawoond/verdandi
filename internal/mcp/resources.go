package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type resourceReadParams struct {
	URI string `json:"uri"`
}

func defaultResources() []Resource {
	return []Resource{
		{
			URI:         "verdandi://runs",
			Name:        "runs",
			Title:       "Verdandi Runs",
			Description: "List persisted Verdandi workflow runs.",
			MimeType:    "application/json",
		},
		{
			URI:         "verdandi://agents",
			Name:        "agents",
			Title:       "Verdandi Agents",
			Description: "List persisted agent contracts and lifecycle recommendations.",
			MimeType:    "application/json",
		},
		{
			URI:         "verdandi://assets",
			Name:        "assets",
			Title:       "Verdandi Assets",
			Description: "List persisted agent and skill assets.",
			MimeType:    "application/json",
		},
		{
			URI:         "verdandi://skills",
			Name:        "skills",
			Title:       "Verdandi Skills",
			Description: "List persisted skill assets.",
			MimeType:    "application/json",
		},
	}
}

func listResources(params json.RawMessage) (map[string]any, error) {
	payload, err := decodeListParams(params)
	if err != nil {
		return nil, err
	}
	page := paginate(defaultResources(), payload.Cursor, resourcePageSize)
	return listResult("resources", page), nil
}

func defaultResourceTemplates() []ResourceTemplate {
	return []ResourceTemplate{
		{
			URITemplate: "verdandi://runs/{runId}",
			Name:        "run",
			Title:       "Verdandi Run",
			Description: "Read a single Verdandi run record by runId.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "verdandi://runs/{runId}/events",
			Name:        "run-events",
			Title:       "Verdandi Run Events",
			Description: "Read visualization events for a Verdandi run.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "verdandi://runs/{runId}/output",
			Name:        "run-output",
			Title:       "Verdandi Run Output",
			Description: "Read generated output file metadata for a Verdandi run.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "verdandi://workflows/{runId}",
			Name:        "workflow",
			Title:       "Verdandi Workflow",
			Description: "Read a prepared workflow package by runId.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "verdandi://workflows/{runId}/handoff",
			Name:        "workflow-handoff",
			Title:       "Verdandi Workflow Handoff",
			Description: "Read the Markdown handoff prompt for a prepared workflow.",
			MimeType:    "text/markdown",
		},
	}
}

func (s *Server) readResource(params json.RawMessage) (map[string]any, error) {
	var payload resourceReadParams
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: err.Error()}
	}
	if strings.TrimSpace(payload.URI) == "" {
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: "resource uri is required"}
	}

	action, args, ok := resourceAction(payload.URI)
	if !ok {
		return nil, resourceNotFound(payload.URI)
	}
	structured, err := s.executor.Execute(action, args)
	if err != nil {
		return nil, resourceNotFound(payload.URI)
	}
	mimeType, text := resourceContent(payload.URI, structured)

	return map[string]any{
		"contents": []map[string]any{
			{
				"uri":      payload.URI,
				"mimeType": mimeType,
				"text":     text,
			},
		},
	}, nil
}

func resourceAction(uri string) (string, map[string]any, bool) {
	switch uri {
	case "verdandi://runs":
		return "list_runs", map[string]any{}, true
	case "verdandi://agents":
		return "list_agents", map[string]any{}, true
	case "verdandi://assets":
		return "list_assets", map[string]any{}, true
	case "verdandi://skills":
		return "list_skills", map[string]any{}, true
	}

	const workflowPrefix = "verdandi://workflows/"
	if strings.HasPrefix(uri, workflowPrefix) {
		rest := strings.TrimPrefix(uri, workflowPrefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 1 && parts[0] != "" {
			return "get_workflow", map[string]any{"runId": parts[0]}, true
		}
		if len(parts) == 2 && parts[0] != "" && parts[1] == "handoff" {
			return "get_workflow_handoff", map[string]any{"runId": parts[0]}, true
		}
		return "", nil, false
	}

	const prefix = "verdandi://runs/"
	if !strings.HasPrefix(uri, prefix) {
		return "", nil, false
	}
	rest := strings.TrimPrefix(uri, prefix)
	if rest == "" {
		return "", nil, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 1 && parts[0] != "" {
		return "get_status", map[string]any{"runId": parts[0]}, true
	}
	if len(parts) == 2 && parts[0] != "" {
		switch parts[1] {
		case "events":
			return "list_events", map[string]any{"runId": parts[0]}, true
		case "output":
			return "open_output", map[string]any{"runId": parts[0]}, true
		}
	}
	return "", nil, false
}

func resourceNotFound(uri string) *JSONRPCError {
	return &JSONRPCError{
		Code:    -32002,
		Message: "Resource not found",
		Data:    map[string]string{"uri": uri},
	}
}

func marshalResourceText(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

func resourceContent(uri string, structured map[string]any) (string, string) {
	if strings.HasSuffix(uri, "/handoff") {
		if handoff, ok := structured["handoff"].(string); ok {
			return "text/markdown", handoff
		}
	}
	return "application/json", marshalResourceText(structured)
}
