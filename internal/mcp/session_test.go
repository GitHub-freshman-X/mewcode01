package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type memoryTransport struct {
	received chan Inbound
	sent     chan []byte
	closed   chan struct{}
	once     sync.Once
}

func newMemoryTransport() *memoryTransport {
	return &memoryTransport{received: make(chan Inbound, 8), sent: make(chan []byte, 8), closed: make(chan struct{})}
}

func (t *memoryTransport) Start(context.Context) error { return nil }
func (t *memoryTransport) Send(_ context.Context, message []byte) error {
	t.sent <- append([]byte(nil), message...)
	return nil
}
func (t *memoryTransport) Receive() <-chan Inbound { return t.received }
func (t *memoryTransport) Close(context.Context) error {
	t.once.Do(func() { close(t.closed) })
	return nil
}

func TestSessionMatchesOutOfOrderResponses(t *testing.T) {
	transport := newMemoryTransport()
	session := NewSession(transport)
	defer session.Close(context.Background())

	results := make(chan string, 2)
	for _, method := range []string{"first", "second"} {
		method := method
		go func() {
			var result struct {
				Value string `json:"value"`
			}
			if err := session.Request(context.Background(), method, map[string]any{}, &result); err != nil {
				t.Errorf("request %s: %v", method, err)
				return
			}
			results <- result.Value
		}()
	}

	type requestRecord struct {
		ID     uint64
		Method string
	}
	requests := make([]requestRecord, 0, 2)
	for len(requests) < 2 {
		var request struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(<-transport.sent, &request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, requestRecord{ID: request.ID, Method: request.Method})
	}
	transport.received <- Inbound{Message: mustResponse(t, requests[1].ID, requests[1].Method)}
	transport.received <- Inbound{Message: mustResponse(t, requests[0].ID, requests[0].Method)}
	got := map[string]bool{<-results: true, <-results: true}
	if !got["first"] || !got["second"] {
		t.Fatalf("results=%v", got)
	}
}

func TestSessionNotificationDoesNotWaitForResponse(t *testing.T) {
	transport := newMemoryTransport()
	session := NewSession(transport)
	defer session.Close(context.Background())
	if err := session.Notify(context.Background(), "notifications/initialized", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	var message map[string]any
	if err := json.Unmarshal(<-transport.sent, &message); err != nil {
		t.Fatal(err)
	}
	if _, ok := message["id"]; ok {
		t.Fatalf("notification has id: %v", message)
	}
}

func TestSessionRequestHonorsCancellation(t *testing.T) {
	transport := newMemoryTransport()
	session := NewSession(transport)
	defer session.Close(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var result map[string]any
	if err := session.Request(ctx, "tools/list", nil, &result); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestSessionCloseUnblocksPendingRequest(t *testing.T) {
	transport := newMemoryTransport()
	session := NewSession(transport)
	done := make(chan error, 1)
	go func() {
		var result map[string]any
		done <- session.Request(context.Background(), "tools/list", nil, &result)
	}()
	<-transport.sent
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected closed session error")
		}
	case <-time.After(time.Second):
		t.Fatal("request did not unblock")
	}
}

func mustResponse(t *testing.T, id uint64, value string) []byte {
	t.Helper()
	message, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]string{"value": value}})
	if err != nil {
		t.Fatal(err)
	}
	return message
}
