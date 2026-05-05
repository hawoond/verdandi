package mcp

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	resourcePageSize = 1
	promptPageSize   = 2
)

type listParams struct {
	Cursor string `json:"cursor"`
}

type pageResult[T any] struct {
	Items      []T
	NextCursor string
}

func decodeListParams(params json.RawMessage) (listParams, error) {
	var payload listParams
	if len(params) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(params, &payload); err != nil {
		return listParams{}, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: err.Error()}
	}
	return payload, nil
}

func paginate[T any](items []T, cursor string, pageSize int) pageResult[T] {
	if strings.TrimSpace(cursor) == "" {
		return pageResult[T]{Items: items}
	}
	start, err := strconv.Atoi(cursor)
	if err != nil || start < 0 {
		start = 0
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	result := pageResult[T]{Items: items[start:end]}
	if end < len(items) {
		result.NextCursor = strconv.Itoa(end)
	}
	return result
}

func listResult[T any](key string, page pageResult[T]) map[string]any {
	result := map[string]any{key: page.Items}
	if page.NextCursor != "" {
		result["nextCursor"] = page.NextCursor
	}
	return result
}
