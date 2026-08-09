package mcp

import (
	"encoding/json"
	"testing"
)

func TestEncodeRequestAndNotification(t *testing.T) {
	request, err := EncodeRequest(7, "tools/call", map[string]any{"name": "search"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(request, &got); err != nil {
		t.Fatal(err)
	}
	if got["jsonrpc"] != "2.0" || got["id"] != float64(7) || got["method"] != "tools/call" {
		t.Fatalf("request=%s", request)
	}
	notification, err := EncodeNotification("notifications/initialized", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]any{}
	if err := json.Unmarshal(notification, &got); err != nil {
		t.Fatal(err)
	}
	if _, exists := got["id"]; exists || got["method"] != "notifications/initialized" {
		t.Fatalf("notification=%s", notification)
	}
}

func TestDecodeResponse(t *testing.T) {
	response, err := DecodeResponse([]byte(`{"jsonrpc":"2.0","id":7,"result":{"tools":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.ID != 7 || string(response.Result) != `{"tools":[]}` || response.Error != nil {
		t.Fatalf("response=%+v", response)
	}
}

func TestDecodeResponseRejectsInvalidMessages(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`not json`),
		[]byte(`{"jsonrpc":"1.0","id":1,"result":{}}`),
		[]byte(`{"jsonrpc":"2.0","result":{}}`),
		[]byte(`{"jsonrpc":"2.0","id":1}`),
	} {
		if _, err := DecodeResponse(raw); err == nil {
			t.Fatalf("expected error for %s", raw)
		}
	}
}

func TestDecodeResponseExposesJSONRPCError(t *testing.T) {
	response, err := DecodeResponse([]byte(`{"jsonrpc":"2.0","id":9,"error":{"code":-32601,"message":"missing method"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != -32601 || response.Error.Message != "missing method" {
		t.Fatalf("response=%+v", response)
	}
}
