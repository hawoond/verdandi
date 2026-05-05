package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTransportPostReturnsJSONRPCResponse(t *testing.T) {
	server := NewServer(&fakeExecutor{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()

	server.HTTPHandler(HTTPOptions{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("expected JSON content type, got %q", contentType)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, response.Body.String())
	}
	if payload["id"] != "ping" || payload["error"] != nil {
		t.Fatalf("unexpected JSON-RPC response: %#v", payload)
	}
}

func TestHTTPTransportNotificationReturnsAccepted(t *testing.T) {
	server := NewServer(&fakeExecutor{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()

	server.HTTPHandler(HTTPOptions{}).ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected HTTP 202, got %d: %s", response.Code, response.Body.String())
	}
	if strings.TrimSpace(response.Body.String()) != "" {
		t.Fatalf("expected empty notification response body, got %q", response.Body.String())
	}
}

func TestHTTPTransportProgressUsesSSE(t *testing.T) {
	server := NewServer(fixtureReplayExecutor{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"progress","method":"tools/call","params":{"_meta":{"progressToken":"http-progress"},"name":"run_plan","arguments":{"request":"fixture","stages":[{"stage":"code-writer","keyword":"fixture"}]}}}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()

	server.HTTPHandler(HTTPOptions{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", contentType)
	}
	events := decodeSSEMessages(t, response.Body.String())
	if len(events) < 2 {
		t.Fatalf("expected progress and final SSE messages, got %#v", events)
	}
	foundProgress := false
	foundFinal := false
	for _, event := range events {
		if event["method"] == "notifications/progress" {
			params := event["params"].(map[string]any)
			if params["progressToken"] == "http-progress" {
				foundProgress = true
			}
		}
		if event["id"] == "progress" {
			foundFinal = true
		}
	}
	if !foundProgress || !foundFinal {
		t.Fatalf("expected progress notification and final response in SSE events: %#v", events)
	}
}

func TestHTTPTransportRejectsMissingPOSTAccept(t *testing.T) {
	server := NewServer(&fakeExecutor{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`))
	response := httptest.NewRecorder()

	server.HTTPHandler(HTTPOptions{}).ServeHTTP(response, request)

	if response.Code != http.StatusNotAcceptable {
		t.Fatalf("expected HTTP 406, got %d: %s", response.Code, response.Body.String())
	}
}

func TestHTTPTransportRejectsInvalidOrigin(t *testing.T) {
	server := NewServer(&fakeExecutor{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()

	server.HTTPHandler(HTTPOptions{}).ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403, got %d: %s", response.Code, response.Body.String())
	}
}

func TestHTTPTransportAllowsConfiguredOrigin(t *testing.T) {
	server := NewServer(&fakeExecutor{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Origin", "https://trusted.example")
	response := httptest.NewRecorder()

	server.HTTPHandler(HTTPOptions{AllowedOrigins: []string{"https://trusted.example"}}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", response.Code, response.Body.String())
	}
}

func TestHTTPTransportRequiresConfiguredBearerToken(t *testing.T) {
	server := NewServer(&fakeExecutor{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()

	server.HTTPHandler(HTTPOptions{BearerToken: "secret"}).ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected HTTP 401, got %d: %s", response.Code, response.Body.String())
	}
	if challenge := response.Header().Get("WWW-Authenticate"); !strings.HasPrefix(challenge, "Bearer") {
		t.Fatalf("expected Bearer challenge, got %q", challenge)
	}
}

func TestHTTPTransportAcceptsConfiguredBearerToken(t *testing.T) {
	server := NewServer(&fakeExecutor{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`))
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()

	server.HTTPHandler(HTTPOptions{BearerToken: "secret"}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", response.Code, response.Body.String())
	}
}

func TestHTTPTransportSessionLifecycle(t *testing.T) {
	server := NewServer(&fakeExecutor{})
	handler := server.HTTPHandler(HTTPOptions{RequireSession: true})

	initialize := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25"}}`))
	initialize.Header.Set("Accept", "application/json, text/event-stream")
	initializeResponse := httptest.NewRecorder()
	handler.ServeHTTP(initializeResponse, initialize)

	if initializeResponse.Code != http.StatusOK {
		t.Fatalf("expected initialize HTTP 200, got %d: %s", initializeResponse.Code, initializeResponse.Body.String())
	}
	sessionID := initializeResponse.Header().Get(httpHeaderSessionID)
	if sessionID == "" {
		t.Fatal("expected initialize response to include MCP-Session-Id")
	}

	missingSession := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`))
	missingSession.Header.Set("Accept", "application/json, text/event-stream")
	missingSessionResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingSessionResponse, missingSession)
	if missingSessionResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected missing session HTTP 400, got %d: %s", missingSessionResponse.Code, missingSessionResponse.Body.String())
	}

	ping := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`))
	ping.Header.Set("Accept", "application/json, text/event-stream")
	ping.Header.Set(httpHeaderSessionID, sessionID)
	pingResponse := httptest.NewRecorder()
	handler.ServeHTTP(pingResponse, ping)
	if pingResponse.Code != http.StatusOK {
		t.Fatalf("expected session ping HTTP 200, got %d: %s", pingResponse.Code, pingResponse.Body.String())
	}

	terminate := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	terminate.Header.Set(httpHeaderSessionID, sessionID)
	terminateResponse := httptest.NewRecorder()
	handler.ServeHTTP(terminateResponse, terminate)
	if terminateResponse.Code != http.StatusNoContent {
		t.Fatalf("expected session delete HTTP 204, got %d: %s", terminateResponse.Code, terminateResponse.Body.String())
	}

	afterDelete := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"ping2","method":"ping"}`))
	afterDelete.Header.Set("Accept", "application/json, text/event-stream")
	afterDelete.Header.Set(httpHeaderSessionID, sessionID)
	afterDeleteResponse := httptest.NewRecorder()
	handler.ServeHTTP(afterDeleteResponse, afterDelete)
	if afterDeleteResponse.Code != http.StatusNotFound {
		t.Fatalf("expected deleted session HTTP 404, got %d: %s", afterDeleteResponse.Code, afterDeleteResponse.Body.String())
	}
}

func TestHTTPTransportGETReturnsMethodNotAllowed(t *testing.T) {
	server := NewServer(&fakeExecutor{})
	request := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	request.Header.Set("Accept", "text/event-stream")
	response := httptest.NewRecorder()

	server.HTTPHandler(HTTPOptions{}).ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected HTTP 405, got %d: %s", response.Code, response.Body.String())
	}
}

func decodeSSEMessages(t *testing.T, body string) []map[string]any {
	t.Helper()

	messages := []map[string]any{}
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var data bytes.Buffer
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data:") {
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if data.Len() == 0 {
			continue
		}
		var message map[string]any
		if err := json.Unmarshal(data.Bytes(), &message); err != nil {
			t.Fatalf("SSE data is not JSON: %v\n%s", err, data.String())
		}
		messages = append(messages, message)
	}
	return messages
}
