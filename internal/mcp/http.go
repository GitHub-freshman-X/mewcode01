package mcp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

var ErrSessionExpired = errors.New("mcp HTTP session expired")

type HTTPTransport struct {
	url       string
	headers   map[string]string
	client    *http.Client
	inbound   chan Inbound
	mu        sync.Mutex
	sessionID string
	version   string
	closed    bool
}

func NewHTTPTransport(url string, headers map[string]string, client *http.Client) *HTTPTransport {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTransport{url: url, headers: cloneStrings(headers), client: client, inbound: make(chan Inbound, 16), version: protocolVersion}
}
func (t *HTTPTransport) Send(ctx context.Context, message []byte) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrSessionClosed
	}
	sessionID, version := t.sessionID, t.version
	t.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(message))
	if err != nil {
		return fmt.Errorf("mcp HTTP request: %w", err)
	}
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("MCP-Session-Id", sessionID)
		req.Header.Set("MCP-Protocol-Version", version)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("mcp HTTP send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound && sessionID != "" {
		return ErrSessionExpired
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp HTTP status %d", resp.StatusCode)
	}
	if id := resp.Header.Get("MCP-Session-Id"); id != "" {
		t.mu.Lock()
		t.sessionID = id
		t.mu.Unlock()
	}
	if version := resp.Header.Get("MCP-Protocol-Version"); version != "" {
		t.mu.Lock()
		t.version = version
		t.mu.Unlock()
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return t.readSSE(resp.Body)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("mcp HTTP response: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	t.inbound <- Inbound{Message: body}
	return nil
}
func (t *HTTPTransport) readSSE(body io.Reader) error {
	scanner := bufio.NewScanner(body)
	var data []string
	emit := func() {
		if len(data) > 0 {
			t.inbound <- Inbound{Message: []byte(strings.Join(data, "\n"))}
			data = nil
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			emit()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	emit()
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp SSE response: %w", err)
	}
	return nil
}
func (t *HTTPTransport) Receive() <-chan Inbound { return t.inbound }
func (t *HTTPTransport) Close(ctx context.Context) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	sessionID := t.sessionID
	t.mu.Unlock()
	if sessionID == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, t.url, nil)
	if err != nil {
		return err
	}
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("MCP-Session-Id", sessionID)
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("mcp HTTP close: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mcp HTTP close status %d", resp.StatusCode)
	}
	return nil
}
