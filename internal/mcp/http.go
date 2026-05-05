package mcp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	httpHeaderProtocolVersion = "MCP-Protocol-Version"
	httpHeaderSessionID       = "MCP-Session-Id"
)

type HTTPOptions struct {
	AllowedOrigins []string
	BearerToken    string
	RequireSession bool
}

func (s *Server) HTTPHandler(options HTTPOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAllowedOrigin(r.Header.Get("Origin"), options.AllowedOrigins) {
			writeHTTPError(w, http.StatusForbidden, "origin is not allowed")
			return
		}
		if !isAuthorized(r.Header.Get("Authorization"), options.BearerToken) {
			writeUnauthorized(w)
			return
		}
		if !isSupportedProtocolVersion(r.Header.Get(httpHeaderProtocolVersion)) {
			writeHTTPError(w, http.StatusBadRequest, "unsupported MCP protocol version")
			return
		}

		switch r.Method {
		case http.MethodPost:
			s.handleHTTPPost(w, r, options)
		case http.MethodDelete:
			s.handleHTTPDelete(w, r, options)
		case http.MethodGet:
			w.Header().Set("Allow", http.MethodPost)
			writeHTTPError(w, http.StatusMethodNotAllowed, "method not allowed")
		default:
			w.Header().Set("Allow", http.MethodPost)
			writeHTTPError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
}

func (s *Server) handleHTTPPost(w http.ResponseWriter, r *http.Request, options HTTPOptions) {
	if !acceptsAll(r.Header.Get("Accept"), "application/json", "text/event-stream") {
		writeHTTPError(w, http.StatusNotAcceptable, "Accept must include application/json and text/event-stream")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONRPCLineBytes))
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, "request body is too large or unreadable")
		return
	}
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		writeHTTPError(w, http.StatusBadRequest, "request body is required")
		return
	}

	newSessionID := ""
	if options.RequireSession {
		method := httpMessageMethod(body)
		if method == "initialize" && strings.TrimSpace(r.Header.Get(httpHeaderSessionID)) == "" {
			newSessionID = s.createHTTPSession()
			w.Header().Set(httpHeaderSessionID, newSessionID)
		} else if !s.validateHTTPSession(w, r) {
			return
		}
	}

	notifications := []any{}
	response, shouldRespond, err := s.HandleMessageWithProgressContext(r.Context(), body, func(notification any) error {
		notifications = append(notifications, notification)
		return nil
	})
	if err != nil {
		writeHTTPError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !shouldRespond {
		if newSessionID != "" {
			w.Header().Set(httpHeaderSessionID, newSessionID)
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if len(notifications) > 0 {
		if newSessionID != "" {
			w.Header().Set(httpHeaderSessionID, newSessionID)
		}
		writeSSEMessages(w, append(notifications, response)...)
		return
	}
	if newSessionID != "" {
		w.Header().Set(httpHeaderSessionID, newSessionID)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleHTTPDelete(w http.ResponseWriter, r *http.Request, options HTTPOptions) {
	if !options.RequireSession {
		w.Header().Set("Allow", http.MethodPost)
		writeHTTPError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID := strings.TrimSpace(r.Header.Get(httpHeaderSessionID))
	if sessionID == "" {
		writeHTTPError(w, http.StatusBadRequest, "MCP-Session-Id is required")
		return
	}
	if !s.deleteHTTPSession(sessionID) {
		writeHTTPError(w, http.StatusNotFound, "unknown MCP session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeSSEMessages(w http.ResponseWriter, messages ...any) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	for _, message := range messages {
		encoded, err := json.Marshal(message)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", encoded)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeHTTPError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse(nil, -32000, http.StatusText(status), message))
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="verdandi-mcp"`)
	writeHTTPError(w, http.StatusUnauthorized, "authorization is required")
}

func isAuthorized(header string, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return true
	}
	header = strings.TrimSpace(header)
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	presented := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(token)) == 1
}

func httpMessageMethod(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		return ""
	}
	var request JSONRPCRequest
	if err := json.Unmarshal([]byte(trimmed), &request); err != nil {
		return ""
	}
	return request.Method
}

func (s *Server) createHTTPSession() string {
	sessionID := newHTTPSessionID()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[string]struct{}{}
	}
	s.sessions[sessionID] = struct{}{}
	return sessionID
}

func (s *Server) validateHTTPSession(w http.ResponseWriter, r *http.Request) bool {
	sessionID := strings.TrimSpace(r.Header.Get(httpHeaderSessionID))
	if sessionID == "" {
		writeHTTPError(w, http.StatusBadRequest, "MCP-Session-Id is required")
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sessionID]; !ok {
		writeHTTPError(w, http.StatusNotFound, "unknown MCP session")
		return false
	}
	return true
}

func (s *Server) deleteHTTPSession(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return false
	}
	delete(s.sessions, sessionID)
	return true
}

func newHTTPSessionID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "session-" + randomFallbackID()
	}
	return "session-" + hex.EncodeToString(random[:])
}

func randomFallbackID() string {
	var fallback [8]byte
	for index := range fallback {
		fallback[index] = byte(index + 1)
	}
	return hex.EncodeToString(fallback[:])
}

func acceptsAll(header string, values ...string) bool {
	accepted := acceptedMediaTypes(header)
	if accepted["*/*"] {
		return true
	}
	for _, value := range values {
		if !accepted[value] {
			return false
		}
	}
	return true
}

func acceptedMediaTypes(header string) map[string]bool {
	result := map[string]bool{}
	for _, part := range strings.Split(header, ",") {
		mediaType := strings.TrimSpace(strings.Split(part, ";")[0])
		if mediaType == "" {
			continue
		}
		result[strings.ToLower(mediaType)] = true
	}
	return result
}

func isSupportedProtocolVersion(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == protocolVersion || value == "2025-06-18" || value == "2025-03-26"
}

func isAllowedOrigin(origin string, allowed []string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	for _, value := range allowed {
		if strings.EqualFold(strings.TrimSpace(value), origin) {
			return true
		}
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}
