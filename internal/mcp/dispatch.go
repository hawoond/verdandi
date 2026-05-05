package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

func (s *Server) HandleMessage(line []byte) (any, bool) {
	response, shouldRespond, _ := s.HandleMessageWithProgress(line, nil)
	return response, shouldRespond
}

func (s *Server) HandleMessageWithProgress(line []byte, emit func(any) error) (any, bool, error) {
	return s.HandleMessageWithProgressContext(context.Background(), line, emit)
}

func (s *Server) HandleMessageWithProgressContext(ctx context.Context, line []byte, emit func(any) error) (any, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if strings.HasPrefix(trimmed, "[") {
		response, shouldRespond := s.handleBatch([]byte(trimmed))
		return response, shouldRespond, nil
	}
	response, shouldRespond, err := s.HandleLineWithProgressContext(ctx, line, emit)
	if !shouldRespond {
		return nil, false, err
	}
	return response, true, err
}

func (s *Server) handleBatch(line []byte) ([]JSONRPCResponse, bool) {
	var rawMessages []json.RawMessage
	if err := json.Unmarshal(line, &rawMessages); err != nil {
		return []JSONRPCResponse{errorResponse(nil, -32700, "Parse error", err.Error())}, true
	}
	if len(rawMessages) == 0 {
		return []JSONRPCResponse{errorResponse(nil, -32600, "Invalid Request", "batch must not be empty")}, true
	}
	responses := []JSONRPCResponse{}
	for _, rawMessage := range rawMessages {
		response, shouldRespond := s.HandleLine(rawMessage)
		if shouldRespond {
			responses = append(responses, response)
		}
	}
	if len(responses) == 0 {
		return nil, false
	}
	return responses, true
}

func (s *Server) HandleLine(line []byte) (JSONRPCResponse, bool) {
	response, shouldRespond, _ := s.HandleLineWithProgress(line, nil)
	return response, shouldRespond
}

func (s *Server) HandleLineWithProgress(line []byte, emit func(any) error) (JSONRPCResponse, bool, error) {
	return s.HandleLineWithProgressContext(context.Background(), line, emit)
}

func (s *Server) HandleLineWithProgressContext(ctx context.Context, line []byte, emit func(any) error) (JSONRPCResponse, bool, error) {
	var request JSONRPCRequest
	if err := json.Unmarshal(line, &request); err != nil {
		return errorResponse(nil, -32700, "Parse error", err.Error()), true, nil
	}
	if request.JSONRPC != "2.0" {
		return errorResponse(request.ID, -32600, "Invalid Request", "jsonrpc must be 2.0"), true, nil
	}
	if request.ID == nil {
		if request.Method == "notifications/cancelled" {
			s.handleCancelled(request.Params)
		}
		return JSONRPCResponse{}, false, nil
	}

	switch request.Method {
	case "initialize":
		return resultResponse(request.ID, s.initializeResult(request.Params)), true, nil
	case "ping":
		return resultResponse(request.ID, map[string]any{}), true, nil
	case "tools/list":
		return resultResponse(request.ID, map[string]any{"tools": s.tools}), true, nil
	case "tools/call":
		result, err := s.callTool(ctx, request.Params, emit)
		if err != nil {
			var rpcErr *JSONRPCError
			if errors.As(err, &rpcErr) {
				return JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: rpcErr}, true, nil
			}
			return errorResponse(request.ID, -32603, "Internal error", err.Error()), true, nil
		}
		return resultResponse(request.ID, result), true, nil
	case "resources/list":
		result, err := listResources(request.Params)
		if err != nil {
			var rpcErr *JSONRPCError
			if errors.As(err, &rpcErr) {
				return JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: rpcErr}, true, nil
			}
			return errorResponse(request.ID, -32603, "Internal error", err.Error()), true, nil
		}
		return resultResponse(request.ID, result), true, nil
	case "resources/templates/list":
		return resultResponse(request.ID, map[string]any{"resourceTemplates": defaultResourceTemplates()}), true, nil
	case "resources/read":
		result, err := s.readResource(request.Params)
		if err != nil {
			var rpcErr *JSONRPCError
			if errors.As(err, &rpcErr) {
				return JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: rpcErr}, true, nil
			}
			return errorResponse(request.ID, -32603, "Internal error", err.Error()), true, nil
		}
		return resultResponse(request.ID, result), true, nil
	case "prompts/list":
		result, err := listPrompts(request.Params)
		if err != nil {
			var rpcErr *JSONRPCError
			if errors.As(err, &rpcErr) {
				return JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: rpcErr}, true, nil
			}
			return errorResponse(request.ID, -32603, "Internal error", err.Error()), true, nil
		}
		return resultResponse(request.ID, result), true, nil
	case "prompts/get":
		result, err := getPrompt(request.Params)
		if err != nil {
			var rpcErr *JSONRPCError
			if errors.As(err, &rpcErr) {
				return JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: rpcErr}, true, nil
			}
			return errorResponse(request.ID, -32603, "Internal error", err.Error()), true, nil
		}
		return resultResponse(request.ID, result), true, nil
	case "completion/complete":
		result, err := s.complete(request.Params)
		if err != nil {
			var rpcErr *JSONRPCError
			if errors.As(err, &rpcErr) {
				return JSONRPCResponse{JSONRPC: "2.0", ID: request.ID, Error: rpcErr}, true, nil
			}
			return errorResponse(request.ID, -32603, "Internal error", err.Error()), true, nil
		}
		return resultResponse(request.ID, result), true, nil
	default:
		return errorResponse(request.ID, -32601, "Method not found", request.Method), true, nil
	}
}
