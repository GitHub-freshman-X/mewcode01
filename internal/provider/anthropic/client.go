package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider/sse"
)

type Options struct {
	BaseURL       *url.URL
	APIKey, Model string
	HTTPClient    *http.Client
	Logger        *logging.Logger
}
type Client struct {
	endpoint      string
	apiKey, model string
	httpClient    *http.Client
	logger        *logging.Logger
}

func New(options Options) provider.Provider {
	u := *options.BaseURL
	u.Path = path.Join(u.Path, "/v1/messages")
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	logger := options.Logger
	if logger == nil {
		logger = logging.Nop()
	}
	return &Client{endpoint: u.String(), apiKey: options.APIKey, model: options.Model, httpClient: client, logger: logger}
}

func (c *Client) Stream(ctx context.Context, input provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	events, done := make(chan provider.StreamEvent), make(chan error, 1)
	go func() {
		defer close(events)
		defer close(done)
		body, err := buildRequest(c.model, input)
		if err != nil {
			done <- err
			return
		}
		payload, err := json.Marshal(body)
		if err != nil {
			done <- requestErr("encode Anthropic request", err)
			return
		}
		c.logger.Info("provider request", logging.Fields{"stage": "provider_request", "provider": "anthropic", "request": string(payload)})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
		if err != nil {
			done <- requestErr("create Anthropic request", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				done <- context.Canceled
			} else {
				done <- &provider.AppError{Stage: provider.StageResponse, Message: "connect to Anthropic", Cause: err}
			}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			done <- &provider.AppError{Stage: provider.StageResponse, StatusCode: resp.StatusCode, Message: httpStatusMessage(resp.StatusCode)}
			return
		}
		decoder, completed := sse.NewDecoder(resp.Body, sse.DefaultMaxEventBytes), false
		var usage provider.Usage
		hasUsage := false
		for {
			frame, err := decoder.Next()
			if err != nil {
				if errors.Is(ctx.Err(), context.Canceled) {
					done <- context.Canceled
				} else if err == io.EOF && completed {
					done <- nil
				} else if err == io.EOF {
					done <- &provider.AppError{Stage: provider.StageStream, Message: "Anthropic stream ended before completion"}
				} else {
					done <- &provider.AppError{Stage: provider.StageStream, Message: "read Anthropic stream", Cause: err}
				}
				return
			}
			event, emit, err := parseEvent(frame.Data)
			if err != nil {
				done <- provider.Sanitize(err, c.apiKey)
				return
			}
			if emit {
				if event.Usage != nil {
					usage.Add(*event.Usage)
					hasUsage = true
					event.Usage = nil
				}
				if event.Type == provider.EventUsage {
					continue
				}
				if event.Type == provider.EventCompleted && hasUsage {
					select {
					case events <- provider.StreamEvent{Type: provider.EventUsage, Usage: &usage}:
					case <-ctx.Done():
						done <- context.Canceled
						return
					}
				}
				select {
				case events <- event:
				case <-ctx.Done():
					done <- context.Canceled
					return
				}
				if event.Type == provider.EventCompleted {
					completed = true
				}
			}
		}
	}()
	return events, done
}

func httpStatusMessage(code int) string {
	switch code {
	case 401, 403:
		return "authentication failed; check api_key"
	case 429:
		return "rate limited; try again later"
	default:
		return fmt.Sprintf("Anthropic returned HTTP %d", code)
	}
}
