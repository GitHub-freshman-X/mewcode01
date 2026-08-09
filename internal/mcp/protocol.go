package mcp

import (
	"encoding/json"
	"fmt"
)

const JSONRPCVersion = "2.0"

type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type Response struct {
	ID     uint64
	Result json.RawMessage
	Error  *JSONRPCError
}

func EncodeRequest(id uint64, method string, params any) ([]byte, error) {
	return encodeMessage(map[string]any{"jsonrpc": JSONRPCVersion, "id": id, "method": method, "params": params})
}

func EncodeNotification(method string, params any) ([]byte, error) {
	return encodeMessage(map[string]any{"jsonrpc": JSONRPCVersion, "method": method, "params": params})
}

func DecodeResponse(raw []byte) (Response, error) {
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *JSONRPCError   `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Response{}, fmt.Errorf("mcp: decode JSON-RPC response: %w", err)
	}
	if envelope.JSONRPC != JSONRPCVersion {
		return Response{}, fmt.Errorf("mcp: unsupported JSON-RPC version %q", envelope.JSONRPC)
	}
	if len(envelope.ID) == 0 {
		return Response{}, fmt.Errorf("mcp: JSON-RPC response is missing id")
	}
	var id uint64
	if err := json.Unmarshal(envelope.ID, &id); err != nil {
		return Response{}, fmt.Errorf("mcp: JSON-RPC response has invalid id")
	}
	if len(envelope.Result) == 0 && envelope.Error == nil {
		return Response{}, fmt.Errorf("mcp: JSON-RPC response is missing result or error")
	}
	if len(envelope.Result) != 0 && envelope.Error != nil {
		return Response{}, fmt.Errorf("mcp: JSON-RPC response contains both result and error")
	}
	return Response{ID: id, Result: append(json.RawMessage(nil), envelope.Result...), Error: envelope.Error}, nil
}

func encodeMessage(message any) ([]byte, error) {
	encoded, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("mcp: encode JSON-RPC message: %w", err)
	}
	return encoded, nil
}
