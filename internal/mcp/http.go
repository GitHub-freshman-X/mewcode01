package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

var ErrSessionExpired = errors.New("mcp HTTP session expired")

type HTTPStatusError struct{ Status int }

func (e *HTTPStatusError) Error() string { return fmt.Sprintf("mcp HTTP status %d", e.Status) }

type HTTPTransport struct {
	url       string
	headers   map[string]string
	client    *http.Client
	inbound   chan Inbound
	mu        sync.Mutex
	sessionID string
	version   string
	modern    bool
	closed    bool
}

func NewHTTPTransport(url string, headers map[string]string, client *http.Client) *HTTPTransport {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTransport{url: url, headers: cloneStrings(headers), client: client, inbound: make(chan Inbound, 16), version: legacyProtocolVersion}
}

func (t *HTTPTransport) SetModern(modern bool) {
	t.mu.Lock()
	t.modern = modern
	if modern {
		t.sessionID = ""
		t.version = modernProtocolVersion
	} else {
		t.version = legacyProtocolVersion
	}
	t.mu.Unlock()
}

func (t *HTTPTransport) Send(ctx context.Context, message []byte) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrSessionClosed
	}
	sessionID, version, modern := t.sessionID, t.version, t.modern
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
	if modern {
		var envelope struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if err := json.Unmarshal(message, &envelope); err != nil {
			return fmt.Errorf("mcp HTTP request metadata: %w", err)
		}
		req.Header.Set("MCP-Protocol-Version", modernProtocolVersion)
		req.Header.Set("Mcp-Method", envelope.Method)
		if envelope.Method == "tools/call" && envelope.Params.Name != "" {
			req.Header.Set("Mcp-Name", envelope.Params.Name)
		}
	} else if sessionID != "" {
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
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if readErr != nil {
			return fmt.Errorf("mcp HTTP error response: %w", readErr)
		}
		if _, decodeErr := DecodeResponse(body); decodeErr == nil {
			t.inbound <- Inbound{Message: body}
			return nil
		}
		return &HTTPStatusError{Status: resp.StatusCode}
	}
	if !modern && resp.Header.Get("MCP-Session-Id") != "" {
		id := resp.Header.Get("MCP-Session-Id")
		t.mu.Lock()
		t.sessionID = id
		t.mu.Unlock()
	}
	if !modern && resp.Header.Get("MCP-Protocol-Version") != "" {
		version := resp.Header.Get("MCP-Protocol-Version")
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
	sessionID, modern := t.sessionID, t.modern
	t.mu.Unlock()
	if modern || sessionID == "" {
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
