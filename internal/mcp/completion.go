package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxCompletionValues = 100

type completeParams struct {
	Ref struct {
		Type string `json:"type"`
		Name string `json:"name,omitempty"`
		URI  string `json:"uri,omitempty"`
	} `json:"ref"`
	Argument struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"argument"`
}

type runCompletionRecord struct {
	RunID   string `json:"runId"`
	Request string `json:"request"`
}

func (s *Server) complete(params json.RawMessage) (map[string]any, error) {
	var payload completeParams
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: err.Error()}
	}

	switch {
	case supportsRunIDCompletion(payload):
		return s.completeRunIDs(payload.Argument.Value)
	case supportsRequestCompletion(payload):
		return s.completeRequests(payload.Argument.Value)
	default:
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid completion reference", Data: payload.Ref}
	}
}

func supportsRunIDCompletion(payload completeParams) bool {
	if payload.Argument.Name != "runId" {
		return false
	}
	switch payload.Ref.Type {
	case "ref/prompt":
		return payload.Ref.Name == "inspect-run" || payload.Ref.Name == "inspect-failed-run"
	case "ref/resource":
		switch payload.Ref.URI {
		case "verdandi://runs/{runId}", "verdandi://runs/{runId}/events", "verdandi://runs/{runId}/output":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func supportsRequestCompletion(payload completeParams) bool {
	if payload.Argument.Name != "request" || payload.Ref.Type != "ref/prompt" {
		return false
	}
	switch payload.Ref.Name {
	case "plan-and-run", "validate-plan", "choose-agent-lifecycle":
		return true
	default:
		return false
	}
}

func (s *Server) completeRunIDs(prefix string) (map[string]any, error) {
	runs, err := s.completionRuns()
	if err != nil {
		return nil, err
	}
	values := []string{}
	for _, run := range runs {
		if strings.HasPrefix(run.RunID, prefix) {
			values = append(values, run.RunID)
		}
	}
	return completionResult(values), nil
}

func (s *Server) completeRequests(prefix string) (map[string]any, error) {
	runs, err := s.completionRuns()
	if err != nil {
		return nil, err
	}
	values := []string{}
	seen := map[string]bool{}
	for _, run := range runs {
		if run.Request == "" || seen[run.Request] || !strings.HasPrefix(run.Request, prefix) {
			continue
		}
		values = append(values, run.Request)
		seen[run.Request] = true
	}
	return completionResult(values), nil
}

func (s *Server) completionRuns() ([]runCompletionRecord, error) {
	structured, err := s.executor.Execute("list_runs", map[string]any{})
	if err != nil {
		return nil, err
	}
	rawRuns, ok := structured["runs"]
	if !ok {
		return []runCompletionRecord{}, nil
	}
	data, err := json.Marshal(rawRuns)
	if err != nil {
		return nil, fmt.Errorf("marshal runs for completion: %w", err)
	}
	var runs []runCompletionRecord
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, fmt.Errorf("decode runs for completion: %w", err)
	}
	return runs, nil
}

func completionResult(values []string) map[string]any {
	total := len(values)
	if len(values) > maxCompletionValues {
		values = values[:maxCompletionValues]
	}
	return map[string]any{
		"completion": map[string]any{
			"values":  values,
			"total":   total,
			"hasMore": total > len(values),
		},
	}
}
