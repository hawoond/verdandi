package mcp

import (
	"encoding/json"
	"strings"
)

type Prompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type promptGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

func defaultPrompts() []Prompt {
	return []Prompt{
		{
			Name:        "plan-and-run",
			Title:       "Plan and Run",
			Description: "Choose Verdandi stages, validate the plan, then run it.",
			Arguments: []PromptArgument{
				{Name: "request", Description: "The user's natural-language task request.", Required: true},
			},
		},
		{
			Name:        "validate-plan",
			Title:       "Validate Plan",
			Description: "Create a client-selected Verdandi plan and validate it before execution.",
			Arguments: []PromptArgument{
				{Name: "request", Description: "The user's natural-language task request.", Required: true},
			},
		},
		{
			Name:        "inspect-run",
			Title:       "Inspect Run",
			Description: "Inspect a previous Verdandi run through resources and status tools.",
			Arguments: []PromptArgument{
				{Name: "runId", Description: "The Verdandi runId to inspect.", Required: true},
			},
		},
		{
			Name:        "inspect-failed-run",
			Title:       "Inspect Failed Run",
			Description: "Analyze a failed Verdandi run and identify the failed stage and output.",
			Arguments: []PromptArgument{
				{Name: "runId", Description: "The failed Verdandi runId to inspect.", Required: true},
			},
		},
		{
			Name:        "choose-agent-lifecycle",
			Title:       "Choose Agent Lifecycle",
			Description: "Use agent metrics to choose reuse-enhance, rewrite, or separate for a future run.",
			Arguments: []PromptArgument{
				{Name: "request", Description: "The next task request that needs an agent lifecycle decision.", Required: true},
			},
		},
	}
}

func listPrompts(params json.RawMessage) (map[string]any, error) {
	payload, err := decodeListParams(params)
	if err != nil {
		return nil, err
	}
	page := paginate(defaultPrompts(), payload.Cursor, promptPageSize)
	return listResult("prompts", page), nil
}

func getPrompt(params json.RawMessage) (map[string]any, error) {
	var payload promptGetParams
	if err := json.Unmarshal(params, &payload); err != nil {
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: err.Error()}
	}
	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: "prompt name is required"}
	}
	if payload.Arguments == nil {
		payload.Arguments = map[string]string{}
	}

	text, description, err := promptText(payload.Name, payload.Arguments)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"description": description,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": map[string]string{
					"type": "text",
					"text": text,
				},
			},
		},
	}, nil
}

func promptText(name string, args map[string]string) (string, string, error) {
	switch name {
	case "plan-and-run":
		request, err := requiredPromptArgument(args, "request")
		if err != nil {
			return "", "", err
		}
		return "Choose the Verdandi stages for this request, call validate_plan with the original request and ordered stages, then call run_plan if the validated plan matches the user's intent.\n\nRequest:\n" + request, "Plan and run a Verdandi workflow.", nil
	case "validate-plan":
		request, err := requiredPromptArgument(args, "request")
		if err != nil {
			return "", "", err
		}
		return "Create an ordered Verdandi stage list from planner, code-writer, tester, documenter, and deployer. Call validate_plan with the original request and the selected stages. Do not execute the plan unless the user asks to continue.\n\nRequest:\n" + request, "Validate a client-selected Verdandi plan.", nil
	case "inspect-run":
		runID, err := requiredPromptArgument(args, "runId")
		if err != nil {
			return "", "", err
		}
		return "Read verdandi://runs/" + runID + " and summarize the stage outcomes, generated output, and any follow-up action.", "Inspect a Verdandi run.", nil
	case "inspect-failed-run":
		runID, err := requiredPromptArgument(args, "runId")
		if err != nil {
			return "", "", err
		}
		return "Read verdandi://runs/" + runID + ", verdandi://runs/" + runID + "/events, and verdandi://runs/" + runID + "/output. Identify the failed stage, quote the useful error detail, and recommend the smallest retry plan.", "Inspect a failed Verdandi run.", nil
	case "choose-agent-lifecycle":
		request, err := requiredPromptArgument(args, "request")
		if err != nil {
			return "", "", err
		}
		return "Read verdandi://agents. Compare existing agent metrics against this request, then choose one lifecycle action: reuse-enhance, rewrite, or separate. Explain the decision briefly before running any workflow.\n\nRequest:\n" + request, "Choose a Verdandi agent lifecycle policy.", nil
	default:
		return "", "", &JSONRPCError{Code: -32602, Message: "Invalid prompt name", Data: name}
	}
}

func requiredPromptArgument(args map[string]string, name string) (string, error) {
	value := strings.TrimSpace(args[name])
	if value == "" {
		return "", &JSONRPCError{Code: -32602, Message: "Missing required argument", Data: name}
	}
	return value, nil
}
