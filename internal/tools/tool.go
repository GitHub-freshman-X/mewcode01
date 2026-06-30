package tools

import (
	"context"
	"encoding/json"
)

type Tool interface {
	Metadata() Metadata
	Execute(context.Context, json.RawMessage) Result
}

type Metadata struct {
	Name        string
	Description string
	Schema      Schema
}

type Schema map[string]any

type Result struct {
	ToolName string         `json:"tool_name"`
	Success  bool           `json:"success"`
	Data     map[string]any `json:"data,omitempty"`
	Error    *ToolError     `json:"error,omitempty"`
}

type ToolError struct {
	Type    ErrorType      `json:"type"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ErrorType string

const (
	ErrorValidation ErrorType = "validation_error"
	ErrorNotFound   ErrorType = "not_found"
	ErrorPermission ErrorType = "permission_error"
	ErrorConflict   ErrorType = "conflict"
	ErrorTimeout    ErrorType = "timeout"
	ErrorExecution  ErrorType = "execution_error"
	ErrorInternal   ErrorType = "internal_error"
)

func Success(name string, data map[string]any) Result {
	return Result{ToolName: name, Success: true, Data: data}
}

func Failure(name string, typ ErrorType, message string, details map[string]any) Result {
	return Result{ToolName: name, Success: false, Error: &ToolError{Type: typ, Message: message, Details: details}}
}

func (r Result) JSON() string {
	b, err := json.Marshal(r)
	if err == nil {
		return string(b)
	}
	fallback := Failure(r.ToolName, ErrorInternal, "failed to encode tool result", map[string]any{"cause": err.Error()})
	b, _ = json.Marshal(fallback)
	return string(b)
}
