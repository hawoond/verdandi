package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/genie-cvc/verdandi/internal/verdandi"
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

type slowExecutor struct {
	calls atomic.Int32
	delay time.Duration
}

func (s *slowExecutor) Execute(name string, args map[string]any) (map[string]any, error) {
	s.calls.Add(1)
	time.Sleep(s.delay)
	return map[string]any{
		"ok":     true,
		"action": name,
	}, nil
}

type cancelAwareExecutor struct {
	started  atomic.Bool
	canceled atomic.Bool
}

func (e *cancelAwareExecutor) Execute(name string, args map[string]any) (map[string]any, error) {
	e.started.Store(true)
	time.Sleep(200 * time.Millisecond)
	return map[string]any{"ok": true, "action": name}, nil
}

func (e *cancelAwareExecutor) ExecuteWithContext(ctx context.Context, name string, args map[string]any, reporter verdandi.ProgressReporter) (map[string]any, error) {
	e.started.Store(true)
	<-ctx.Done()
	e.canceled.Store(true)
	return nil, ctx.Err()
}

type fixtureReplayExecutor struct{}

func (e fixtureReplayExecutor) Execute(name string, args map[string]any) (map[string]any, error) {
	switch name {
	case "list_runs":
		return map[string]any{
			"ok":     true,
			"action": name,
			"runs": []map[string]any{
				{"runId": "run_fixture", "request": "Build a calculator app and verify it."},
			},
		}, nil
	case "list_agents":
		return map[string]any{
			"ok":     true,
			"action": name,
			"agents": []map[string]any{
				{"name": "code-writer", "description": "fixture agent"},
			},
		}, nil
	case "get_status":
		return map[string]any{"ok": true, "action": name, "runId": args["runId"]}, nil
	case "list_events":
		return map[string]any{"ok": true, "action": name, "runId": args["runId"], "events": []map[string]any{}}, nil
	case "open_output":
		return map[string]any{"ok": true, "action": name, "runId": args["runId"], "outputs": []map[string]any{}}, nil
	default:
		return map[string]any{"ok": true, "action": name, "echo": args}, nil
	}
}

func (e fixtureReplayExecutor) ExecuteWithContext(ctx context.Context, name string, args map[string]any, reporter verdandi.ProgressReporter) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if reporter != nil {
		reporter(verdandi.ProgressEvent{Progress: 0, Total: 2, Message: name + " started"})
		reporter(verdandi.ProgressEvent{Progress: 2, Total: 2, Message: name + " completed"})
	}
	return e.Execute(name, args)
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

	for _, name := range []string{"run", "analyze", "run_plan", "validate_plan", "orchestrate", "get_status", "open_output", "list_agents"} {
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

func TestRunPlanToolForwardsClientSelectedStages(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"run_plan","arguments":{"request":"build and test a calculator","stages":[{"stage":"code-writer","keyword":"client-llm"},{"stage":"tester","keyword":"client-llm"}]}}}` + "\n"
	var output bytes.Buffer
	executor := &fakeExecutor{}

	server := NewServer(executor)
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	if len(executor.calls) != 1 {
		t.Fatalf("expected 1 executor call, got %d", len(executor.calls))
	}
	if executor.calls[0].Name != "run_plan" {
		t.Fatalf("expected run_plan call, got %q", executor.calls[0].Name)
	}
	stages, ok := executor.calls[0].Args["stages"].([]any)
	if !ok || len(stages) != 2 {
		t.Fatalf("stages were not forwarded: %#v", executor.calls[0].Args)
	}
}

func TestServeHandlesLargeJSONRPCLines(t *testing.T) {
	largeRequest := strings.Repeat("긴 요청 ", 14000)
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      9,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "run",
			"arguments": map[string]any{
				"request": largeRequest,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var output bytes.Buffer
	executor := &fakeExecutor{}

	server := NewServer(executor)
	if err := server.Serve(strings.NewReader(string(payload)+"\n"), &output); err != nil {
		t.Fatalf("serve large request: %v", err)
	}

	if len(executor.calls) != 1 {
		t.Fatalf("expected executor call for large request, got %d", len(executor.calls))
	}
	if executor.calls[0].Args["request"] != largeRequest {
		t.Fatalf("large request was not forwarded")
	}
}

func TestServeHandlesJSONRPCBatchRequests(t *testing.T) {
	input := `[` +
		`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25"}},` +
		`{"jsonrpc":"2.0","method":"notifications/initialized"},` +
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list"},` +
		`{"jsonrpc":"2.0","id":"prompts","method":"prompts/list"}` +
		`]` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve batch: %v", err)
	}

	var responses []map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &responses); err != nil {
		t.Fatalf("batch response is not a JSON array: %v\noutput: %s", err, output.String())
	}
	if len(responses) != 3 {
		t.Fatalf("expected 3 responses for requests with ids, got %d: %#v", len(responses), responses)
	}
	ids := []string{}
	for _, response := range responses {
		ids = append(ids, response["id"].(string))
	}
	if strings.Join(ids, ",") != "init,tools,prompts" {
		t.Fatalf("unexpected batch response ids: %#v", ids)
	}
}

func TestCancellationNotificationProducesNoResponse(t *testing.T) {
	input := `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"missing","reason":"user stopped waiting"}}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve cancellation: %v", err)
	}
	if strings.TrimSpace(output.String()) != "" {
		t.Fatalf("expected no response for cancellation notification, got %q", output.String())
	}
}

func TestCancellationSuppressesInFlightToolResponse(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"cancel-me","method":"tools/call","params":{"name":"run","arguments":{"request":"long request"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"cancel-me","reason":"user stopped waiting"}}` + "\n"
	var output bytes.Buffer
	executor := &slowExecutor{delay: 80 * time.Millisecond}

	server := NewServer(executor)
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve cancellation: %v", err)
	}
	if executor.calls.Load() != 1 {
		t.Fatalf("expected tool call to start once, got %d", executor.calls.Load())
	}
	if strings.TrimSpace(output.String()) != "" {
		t.Fatalf("expected cancelled in-flight request response to be suppressed, got %q", output.String())
	}
}

func TestCancellationPropagatesContextToInFlightTool(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"cancel-context","method":"tools/call","params":{"name":"run","arguments":{"request":"long request"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"cancel-context","reason":"user stopped waiting"}}` + "\n"
	var output bytes.Buffer
	executor := &cancelAwareExecutor{}

	server := NewServer(executor)
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve cancellation: %v", err)
	}
	if !executor.started.Load() {
		t.Fatal("expected tool call to start")
	}
	if !executor.canceled.Load() {
		t.Fatal("expected cancellation to propagate to executor context")
	}
	if strings.TrimSpace(output.String()) != "" {
		t.Fatalf("expected cancelled in-flight request response to be suppressed, got %q", output.String())
	}
}

func TestRunToolReturnsStructuredVerdandiResult(t *testing.T) {
	dataDir := t.TempDir()
	input := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"run","arguments":{"request":"기획 구현 테스트 문서화"}}}` + "\n"
	var output bytes.Buffer

	server := NewServer(verdandi.NewExecutor(verdandi.Options{DataDir: dataDir}))
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	structured := result["structuredContent"].(map[string]any)
	if structured["ok"] != true {
		t.Fatalf("expected ok structured content, got %#v", structured)
	}
	if structured["action"] != "run" {
		t.Fatalf("expected run action, got %#v", structured)
	}
	if structured["runId"] == "" {
		t.Fatalf("expected runId, got %#v", structured)
	}
	summary := structured["summary"].(map[string]any)
	if summary["failed"].(float64) != 0 {
		t.Fatalf("expected no failed stages, got %#v", summary)
	}
}

func TestRunPlanToolReturnsStructuredVerdandiResult(t *testing.T) {
	dataDir := t.TempDir()
	input := `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"run_plan","arguments":{"request":"client selected stages","stages":[{"stage":"code-writer","keyword":"client-llm"}]}}}` + "\n"
	var output bytes.Buffer

	server := NewServer(verdandi.NewExecutor(verdandi.Options{DataDir: dataDir}))
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	structured := result["structuredContent"].(map[string]any)
	if structured["ok"] != true {
		t.Fatalf("expected ok structured content, got %#v", structured)
	}
	if structured["action"] != "run_plan" {
		t.Fatalf("expected run_plan action, got %#v", structured)
	}
	if structured["analyzer"] != "client-plan" {
		t.Fatalf("expected client-plan analyzer marker, got %#v", structured["analyzer"])
	}
	summary := structured["summary"].(map[string]any)
	if summary["totalStages"].(float64) != 2 {
		t.Fatalf("expected code-writer plan to auto-add tester, got %#v", summary)
	}
}

func TestRunPlanWithProgressTokenEmitsProgressNotifications(t *testing.T) {
	dataDir := t.TempDir()
	input := `{"jsonrpc":"2.0","id":"progress-run","method":"tools/call","params":{"_meta":{"progressToken":"progress-1"},"name":"run_plan","arguments":{"request":"client selected stages","stages":[{"stage":"code-writer","keyword":"client-llm"}]}}}` + "\n"
	var output bytes.Buffer

	server := NewServer(verdandi.NewExecutor(verdandi.Options{DataDir: dataDir}))
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	messages := decodeResponses(t, output.String())
	if len(messages) < 3 {
		t.Fatalf("expected progress notifications and final response, got %#v", messages)
	}
	final := messages[len(messages)-1]
	if final["id"] != "progress-run" {
		t.Fatalf("expected final response last, got %#v", final)
	}

	notifications := messages[:len(messages)-1]
	foundStarted := false
	foundCompleted := false
	for _, notification := range notifications {
		if notification["method"] != "notifications/progress" {
			t.Fatalf("expected progress notification before final response, got %#v", notification)
		}
		params := notification["params"].(map[string]any)
		if params["progressToken"] != "progress-1" {
			t.Fatalf("unexpected progress token: %#v", params)
		}
		if params["total"].(float64) != 2 {
			t.Fatalf("expected total stage count 2, got %#v", params)
		}
		message := params["message"].(string)
		if strings.Contains(message, "code-writer started") {
			foundStarted = true
		}
		if strings.Contains(message, "tester completed") {
			foundCompleted = true
		}
	}
	if !foundStarted || !foundCompleted {
		t.Fatalf("missing expected progress messages in %#v", notifications)
	}
}

func TestRunPlanWithoutProgressTokenReturnsOnlyFinalResponse(t *testing.T) {
	dataDir := t.TempDir()
	input := `{"jsonrpc":"2.0","id":"plain-run","method":"tools/call","params":{"name":"run_plan","arguments":{"request":"client selected stages","stages":[{"stage":"code-writer","keyword":"client-llm"}]}}}` + "\n"
	var output bytes.Buffer

	server := NewServer(verdandi.NewExecutor(verdandi.Options{DataDir: dataDir}))
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	messages := decodeResponses(t, output.String())
	if len(messages) != 1 {
		t.Fatalf("expected only final response without progress token, got %#v", messages)
	}
	if messages[0]["id"] != "plain-run" {
		t.Fatalf("unexpected final response: %#v", messages[0])
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
	capabilities := result["capabilities"].(map[string]any)
	for _, name := range []string{"tools", "resources", "prompts", "completions"} {
		if _, ok := capabilities[name].(map[string]any); !ok {
			t.Fatalf("missing %s capability in %#v", name, capabilities)
		}
	}
}

func TestResourcesListExposesVerdandiStateResources(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"resources","method":"resources/list"}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	resources := result["resources"].([]any)
	uris := map[string]bool{}
	for _, rawResource := range resources {
		resource := rawResource.(map[string]any)
		uris[resource["uri"].(string)] = true
		if resource["mimeType"] != "application/json" {
			t.Fatalf("expected JSON resource mime type, got %#v", resource)
		}
	}

	for _, uri := range []string{"verdandi://runs", "verdandi://agents"} {
		if !uris[uri] {
			t.Fatalf("missing resource %q in %#v", uri, uris)
		}
	}
}

func TestResourcesListSupportsCursorPagination(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{"cursor":"0"}}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	resources := result["resources"].([]any)
	if len(resources) != 1 {
		t.Fatalf("expected first paginated resource only, got %#v", resources)
	}
	if result["nextCursor"] != "1" {
		t.Fatalf("expected next cursor 1, got %#v", result)
	}

	input = `{"jsonrpc":"2.0","id":"resources-next","method":"resources/list","params":{"cursor":"1"}}` + "\n"
	output.Reset()
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve next page: %v", err)
	}

	responses = decodeResponses(t, output.String())
	result = responses[0]["result"].(map[string]any)
	resources = result["resources"].([]any)
	if len(resources) != 1 {
		t.Fatalf("expected second paginated resource only, got %#v", resources)
	}
	if _, ok := result["nextCursor"]; ok {
		t.Fatalf("did not expect next cursor on final page: %#v", result)
	}
}

func TestResourceTemplatesListExposesParameterizedVerdandiResources(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"templates","method":"resources/templates/list"}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	templates := result["resourceTemplates"].([]any)
	uris := map[string]bool{}
	for _, rawTemplate := range templates {
		template := rawTemplate.(map[string]any)
		uris[template["uriTemplate"].(string)] = true
	}

	for _, uri := range []string{"verdandi://runs/{runId}", "verdandi://runs/{runId}/events", "verdandi://runs/{runId}/output"} {
		if !uris[uri] {
			t.Fatalf("missing resource template %q in %#v", uri, uris)
		}
	}
}

func TestResourcesReadReturnsStructuredVerdandiState(t *testing.T) {
	dataDir := t.TempDir()
	runInput := `{"jsonrpc":"2.0","id":"run","method":"tools/call","params":{"name":"run","arguments":{"request":"기획 구현 테스트"}}}` + "\n"
	var runOutput bytes.Buffer
	server := NewServer(verdandi.NewExecutor(verdandi.Options{DataDir: dataDir}))
	if err := server.Serve(strings.NewReader(runInput), &runOutput); err != nil {
		t.Fatalf("run serve: %v", err)
	}
	runResponses := decodeResponses(t, runOutput.String())
	runResult := runResponses[0]["result"].(map[string]any)
	runID := runResult["structuredContent"].(map[string]any)["runId"].(string)

	input := `{"jsonrpc":"2.0","id":"read","method":"resources/read","params":{"uri":"verdandi://runs/` + runID + `"}}` + "\n"
	var output bytes.Buffer
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("read serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	contents := result["contents"].([]any)
	content := contents[0].(map[string]any)
	if content["uri"] != "verdandi://runs/"+runID {
		t.Fatalf("unexpected content uri: %#v", content)
	}
	if content["mimeType"] != "application/json" {
		t.Fatalf("unexpected content mime type: %#v", content)
	}
	if !strings.Contains(content["text"].(string), runID) {
		t.Fatalf("resource text does not include run id: %s", content["text"])
	}
}

func TestResourcesReadUnknownVerdandiResourceReturnsNotFound(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"missing","method":"resources/read","params":{"uri":"verdandi://missing"}}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	errPayload := responses[0]["error"].(map[string]any)
	if errPayload["code"].(float64) != -32002 {
		t.Fatalf("expected resource not found code, got %#v", errPayload)
	}
}

func TestPromptsListExposesVerdandiWorkflowPrompts(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"prompts","method":"prompts/list"}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	prompts := result["prompts"].([]any)
	names := map[string]bool{}
	for _, rawPrompt := range prompts {
		prompt := rawPrompt.(map[string]any)
		names[prompt["name"].(string)] = true
	}

	for _, name := range []string{"plan-and-run", "validate-plan", "inspect-run", "inspect-failed-run", "choose-agent-lifecycle"} {
		if !names[name] {
			t.Fatalf("missing prompt %q in %#v", name, names)
		}
	}
}

func TestPromptsListSupportsCursorPagination(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"prompts-page","method":"prompts/list","params":{"cursor":"0"}}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	prompts := result["prompts"].([]any)
	if len(prompts) != 2 {
		t.Fatalf("expected first prompt page size 2, got %#v", prompts)
	}
	if result["nextCursor"] != "2" {
		t.Fatalf("expected next cursor 2, got %#v", result)
	}

	input = `{"jsonrpc":"2.0","id":"prompts-final","method":"prompts/list","params":{"cursor":"4"}}` + "\n"
	output.Reset()
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve final page: %v", err)
	}

	responses = decodeResponses(t, output.String())
	result = responses[0]["result"].(map[string]any)
	prompts = result["prompts"].([]any)
	if len(prompts) != 1 {
		t.Fatalf("expected final prompt page size 1, got %#v", prompts)
	}
	if _, ok := result["nextCursor"]; ok {
		t.Fatalf("did not expect next cursor on final page: %#v", result)
	}
}

func TestPromptsGetRendersWorkflowPrompt(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"prompt","method":"prompts/get","params":{"name":"validate-plan","arguments":{"request":"계산기 앱 구현"}}}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	messages := result["messages"].([]any)
	message := messages[0].(map[string]any)
	content := message["content"].(map[string]any)
	if message["role"] != "user" {
		t.Fatalf("unexpected prompt role: %#v", message)
	}
	if !strings.Contains(content["text"].(string), "validate_plan") {
		t.Fatalf("prompt text should guide validate_plan usage: %s", content["text"])
	}
	if !strings.Contains(content["text"].(string), "계산기 앱 구현") {
		t.Fatalf("prompt text should include request argument: %s", content["text"])
	}
}

func TestCompletionCompleteReturnsEmptyResultForKnownRequestArgument(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"complete","method":"completion/complete","params":{"ref":{"type":"ref/prompt","name":"validate-plan"},"argument":{"name":"request","value":"no-match"}}}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	completion := completionFromResponse(t, output.String())
	if len(completion["values"].([]any)) != 0 {
		t.Fatalf("expected no request suggestions without runs, got %#v", completion)
	}
	if completion["total"].(float64) != 0 || completion["hasMore"].(bool) {
		t.Fatalf("unexpected completion metadata: %#v", completion)
	}
}

func TestCompletionCompleteSuggestsRunIDsForPromptArgument(t *testing.T) {
	dataDir := t.TempDir()
	server := NewServer(verdandi.NewExecutor(verdandi.Options{DataDir: dataDir}))
	runID := createFixtureRun(t, server, "fixture request")

	input := `{"jsonrpc":"2.0","id":"complete","method":"completion/complete","params":{"ref":{"type":"ref/prompt","name":"inspect-run"},"argument":{"name":"runId","value":"run_"}}}` + "\n"
	var output bytes.Buffer
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve completion: %v", err)
	}

	completion := completionFromResponse(t, output.String())
	values := completion["values"].([]any)
	if len(values) != 1 || values[0] != runID {
		t.Fatalf("expected run id completion %q, got %#v", runID, completion)
	}
	if completion["total"].(float64) != 1 || completion["hasMore"].(bool) {
		t.Fatalf("unexpected completion metadata: %#v", completion)
	}
}

func TestCompletionCompleteSuggestsRunIDsForResourceTemplate(t *testing.T) {
	dataDir := t.TempDir()
	server := NewServer(verdandi.NewExecutor(verdandi.Options{DataDir: dataDir}))
	runID := createFixtureRun(t, server, "fixture request")

	input := `{"jsonrpc":"2.0","id":"complete","method":"completion/complete","params":{"ref":{"type":"ref/resource","uri":"verdandi://runs/{runId}/events"},"argument":{"name":"runId","value":"run_"}}}` + "\n"
	var output bytes.Buffer
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve completion: %v", err)
	}

	completion := completionFromResponse(t, output.String())
	values := completion["values"].([]any)
	if len(values) != 1 || values[0] != runID {
		t.Fatalf("expected run id completion %q, got %#v", runID, completion)
	}
}

func TestCompletionCompleteSuggestsRecentRequestsForPromptArgument(t *testing.T) {
	dataDir := t.TempDir()
	server := NewServer(verdandi.NewExecutor(verdandi.Options{DataDir: dataDir}))
	createFixtureRun(t, server, "fixture request")

	input := `{"jsonrpc":"2.0","id":"complete","method":"completion/complete","params":{"ref":{"type":"ref/prompt","name":"validate-plan"},"argument":{"name":"request","value":"zzz"}}}` + "\n"
	var output bytes.Buffer
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve completion: %v", err)
	}

	completion := completionFromResponse(t, output.String())
	if len(completion["values"].([]any)) != 0 {
		t.Fatalf("did not expect nonmatching request suggestions: %#v", completion)
	}

	input = `{"jsonrpc":"2.0","id":"complete2","method":"completion/complete","params":{"ref":{"type":"ref/prompt","name":"validate-plan"},"argument":{"name":"request","value":"fixture"}}}` + "\n"
	output.Reset()
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve completion: %v", err)
	}
	completion = completionFromResponse(t, output.String())
	values := completion["values"].([]any)
	if len(values) != 1 || values[0] != "fixture request" {
		t.Fatalf("expected fixture request completion, got %#v", completion)
	}
}

func TestCompletionCompleteRejectsInvalidReference(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"complete","method":"completion/complete","params":{"ref":{"type":"ref/prompt","name":"unknown"},"argument":{"name":"request","value":""}}}` + "\n"
	var output bytes.Buffer

	server := NewServer(&fakeExecutor{})
	if err := server.Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := decodeResponses(t, output.String())
	errPayload := responses[0]["error"].(map[string]any)
	if errPayload["code"].(float64) != -32602 {
		t.Fatalf("expected invalid params error, got %#v", errPayload)
	}
}

func TestMCPInspectorFixturesAreValidRequests(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "mcp-inspector-fixtures.jsonl")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture file: %v", err)
	}
	defer file.Close()

	server := NewServer(&fakeExecutor{})
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	seen := map[string]bool{}
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		recordFixtureMethods(t, lineNumber, line, seen)

		response, shouldRespond := server.HandleMessage([]byte(line))
		if !shouldRespond {
			if fixtureLineMaySkipResponse(line) {
				continue
			}
			t.Fatalf("fixture line %d should produce a response", lineNumber)
		}
		assertFixtureResponseHasNoError(t, lineNumber, response)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fixture file: %v", err)
	}

	for _, method := range []string{"initialize", "ping", "tools/list", "resources/list", "resources/templates/list", "resources/read", "prompts/list", "prompts/get", "tools/call", "completion/complete"} {
		if !seen[method] {
			t.Fatalf("fixture file does not cover %s; covered %#v", method, seen)
		}
	}
}

func TestMCPInspectorFixturesReplayThroughStdioTransport(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("..", "..", "docs", "mcp-inspector-fixtures.jsonl"))
	if err != nil {
		t.Fatalf("read fixture file: %v", err)
	}

	expectedIDs := fixtureResponseIDs(t, string(input))
	var output bytes.Buffer
	server := NewServer(fixtureReplayExecutor{})
	if err := server.Serve(bytes.NewReader(input), &output); err != nil {
		t.Fatalf("serve fixture stream: %v", err)
	}

	messages := decodeJSONRPCStream(t, output.String())
	seenIDs := map[string]bool{}
	foundProgress := false
	for _, message := range messages {
		if method, ok := message["method"].(string); ok {
			if method != "notifications/progress" {
				t.Fatalf("unexpected server notification in fixture output: %#v", message)
			}
			params := message["params"].(map[string]any)
			if params["progressToken"] == "fixture-progress" {
				foundProgress = true
			}
			continue
		}

		if errPayload, ok := message["error"]; ok {
			t.Fatalf("fixture stream produced JSON-RPC error: %#v", errPayload)
		}
		id, ok := jsonRPCIDString(message["id"])
		if !ok {
			t.Fatalf("fixture response missing id: %#v", message)
		}
		seenIDs[id] = true
	}

	for id := range expectedIDs {
		if !seenIDs[id] {
			t.Fatalf("fixture stream missing response id %q; saw %#v", id, seenIDs)
		}
	}
	if !foundProgress {
		t.Fatalf("fixture stream did not emit progress notification for fixture-progress: %#v", messages)
	}
}

func fixtureLineMaySkipResponse(line string) bool {
	if strings.HasPrefix(line, "[") {
		return false
	}
	var request JSONRPCRequest
	if err := json.Unmarshal([]byte(line), &request); err != nil {
		return false
	}
	return request.ID == nil && strings.HasPrefix(request.Method, "notifications/")
}

func fixtureResponseIDs(t *testing.T, content string) map[string]bool {
	t.Helper()

	expected := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			var requests []JSONRPCRequest
			if err := json.Unmarshal([]byte(line), &requests); err != nil {
				t.Fatalf("fixture line %d is not valid JSON-RPC batch: %v", lineNumber, err)
			}
			for _, request := range requests {
				recordFixtureResponseID(t, lineNumber, request, expected)
			}
			continue
		}

		var request JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			t.Fatalf("fixture line %d is not valid JSON-RPC: %v", lineNumber, err)
		}
		recordFixtureResponseID(t, lineNumber, request, expected)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fixture content: %v", err)
	}
	return expected
}

func recordFixtureResponseID(t *testing.T, lineNumber int, request JSONRPCRequest, expected map[string]bool) {
	t.Helper()

	id, ok := jsonRPCIDString(request.ID)
	if !ok {
		return
	}
	if expected[id] {
		t.Fatalf("fixture line %d duplicates response id %q", lineNumber, id)
	}
	expected[id] = true
}

func decodeJSONRPCStream(t *testing.T, output string) []map[string]any {
	t.Helper()

	messages := []map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			var batch []map[string]any
			if err := json.Unmarshal([]byte(line), &batch); err != nil {
				t.Fatalf("batch response is not JSON: %v\nline: %s", err, line)
			}
			messages = append(messages, batch...)
			continue
		}

		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("response is not JSON: %v\nline: %s", err, line)
		}
		messages = append(messages, message)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan response stream: %v", err)
	}
	return messages
}

func jsonRPCIDString(id any) (string, bool) {
	switch value := id.(type) {
	case nil:
		return "", false
	case string:
		return "s:" + value, true
	case float64:
		return fmt.Sprintf("n:%g", value), true
	case json.Number:
		return "n:" + value.String(), true
	default:
		return fmt.Sprintf("%T:%v", value, value), true
	}
}

func recordFixtureMethods(t *testing.T, lineNumber int, line string, seen map[string]bool) {
	t.Helper()

	if strings.HasPrefix(line, "[") {
		var requests []JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &requests); err != nil {
			t.Fatalf("fixture line %d is not valid JSON-RPC batch: %v", lineNumber, err)
		}
		if len(requests) == 0 {
			t.Fatalf("fixture line %d has empty JSON-RPC batch", lineNumber)
		}
		for _, request := range requests {
			recordFixtureMethod(t, lineNumber, request, seen)
		}
		return
	}

	var request JSONRPCRequest
	if err := json.Unmarshal([]byte(line), &request); err != nil {
		t.Fatalf("fixture line %d is not valid JSON-RPC: %v", lineNumber, err)
	}
	recordFixtureMethod(t, lineNumber, request, seen)
}

func recordFixtureMethod(t *testing.T, lineNumber int, request JSONRPCRequest, seen map[string]bool) {
	t.Helper()

	if request.JSONRPC != "2.0" {
		t.Fatalf("fixture line %d has unexpected jsonrpc version: %q", lineNumber, request.JSONRPC)
	}
	if request.Method == "" {
		t.Fatalf("fixture line %d is missing method", lineNumber)
	}
	seen[request.Method] = true
}

func assertFixtureResponseHasNoError(t *testing.T, lineNumber int, response any) {
	t.Helper()

	switch typed := response.(type) {
	case JSONRPCResponse:
		if typed.Error != nil {
			t.Fatalf("fixture line %d produced JSON-RPC error: %#v", lineNumber, typed.Error)
		}
	case []JSONRPCResponse:
		if len(typed) == 0 {
			t.Fatalf("fixture line %d produced an empty batch response", lineNumber)
		}
		for _, item := range typed {
			if item.Error != nil {
				t.Fatalf("fixture line %d produced JSON-RPC error in batch: %#v", lineNumber, item.Error)
			}
		}
	default:
		t.Fatalf("fixture line %d produced unexpected response type %T", lineNumber, response)
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

func completionFromResponse(t *testing.T, output string) map[string]any {
	t.Helper()

	responses := decodeResponses(t, output)
	result := responses[0]["result"].(map[string]any)
	return result["completion"].(map[string]any)
}

func createFixtureRun(t *testing.T, server *Server, request string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "run",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "run_plan",
			"arguments": map[string]any{
				"request": request,
				"stages": []map[string]any{
					{"stage": "code-writer", "keyword": "fixture"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal fixture run: %v", err)
	}

	var output bytes.Buffer
	if err := server.Serve(strings.NewReader(string(payload)+"\n"), &output); err != nil {
		t.Fatalf("serve fixture run: %v", err)
	}
	responses := decodeResponses(t, output.String())
	result := responses[0]["result"].(map[string]any)
	return result["structuredContent"].(map[string]any)["runId"].(string)
}
