package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

type scriptedTransport struct {
	received chan Inbound
	handler  func(map[string]any) Inbound
}

func newScriptedTransport(t *testing.T, handler func(map[string]any) Inbound) *scriptedTransport {
	t.Helper()
	return &scriptedTransport{received: make(chan Inbound, 16), handler: handler}
}
func (t *scriptedTransport) Send(_ context.Context, message []byte) error {
	var request map[string]any
	if err := json.Unmarshal(message, &request); err != nil {
		return err
	}
	if inbound := t.handler(request); len(inbound.Message) > 0 || inbound.Err != nil {
		t.received <- inbound
	}
	return nil
}
func (t *scriptedTransport) Receive() <-chan Inbound     { return t.received }
func (t *scriptedTransport) Close(context.Context) error { return nil }
