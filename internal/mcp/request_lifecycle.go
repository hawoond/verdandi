package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type cancelledParams struct {
	RequestID any    `json:"requestId"`
	Reason    string `json:"reason,omitempty"`
}

func asyncRequestKey(line []byte) (string, bool) {
	trimmed := strings.TrimSpace(string(line))
	if strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	var request JSONRPCRequest
	if err := json.Unmarshal([]byte(trimmed), &request); err != nil {
		return "", false
	}
	if request.Method != "tools/call" || request.ID == nil {
		return "", false
	}
	return requestIDKey(request.ID)
}

func requestIDKey(id any) (string, bool) {
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

func (s *Server) markActive(key string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[key] = activeRequest{cancel: cancel}
	delete(s.canceled, key)
}

func (s *Server) clearRequest(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, key)
	delete(s.canceled, key)
}

func (s *Server) isCanceled(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canceled[key] != ""
}

func (s *Server) handleCancelled(params json.RawMessage) {
	var payload cancelledParams
	if err := json.Unmarshal(params, &payload); err != nil {
		return
	}
	key, ok := requestIDKey(payload.RequestID)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.active[key]
	if !ok {
		return
	}
	if active.cancel != nil {
		active.cancel()
	}
	if payload.Reason == "" {
		payload.Reason = "cancelled"
	}
	s.canceled[key] = payload.Reason
}
