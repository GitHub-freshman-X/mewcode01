package provider

import (
	"fmt"
	"math"
	"strings"
)

type EventType string

const (
	EventStarted        EventType = "started"
	EventThinkingDelta  EventType = "thinking_delta"
	EventTextDelta      EventType = "text_delta"
	EventSignatureDelta EventType = "signature_delta"
	EventToolCallStart  EventType = "tool_call_start"
	EventToolCallDelta  EventType = "tool_call_delta"
	EventToolCallDone   EventType = "tool_call_done"
	EventUsage          EventType = "usage"
	EventCompleted      EventType = "completed"
)

type Usage struct {
	InputTokens  int
	OutputTokens int

	CacheReadInputTokens       int
	CacheCreationInputTokens   int
	CacheTokensIncludedInInput int
	CacheUnavailable           bool
}

func (u *Usage) Add(other Usage) {
	if u == nil {
		return
	}
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheReadInputTokens += other.CacheReadInputTokens
	u.CacheCreationInputTokens += other.CacheCreationInputTokens
	u.CacheTokensIncludedInInput += other.CacheTokensIncludedInInput
	u.CacheUnavailable = u.CacheUnavailable || other.CacheUnavailable
}

func (u Usage) CacheHitRate() (int, bool) {
	if u.CacheUnavailable {
		return 0, false
	}
	denominator := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens - u.CacheTokensIncludedInInput
	if denominator == 0 {
		return 0, false
	}
	return int(math.Round(float64(u.CacheReadInputTokens) * 100 / float64(denominator))), true
}

type StreamEvent struct {
	Type       EventType
	BlockIndex int
	Delta      string
	ToolCall   *ToolCallDelta
	Usage      *Usage
}

type ToolCallDelta struct {
	ID             string
	Name           string
	ArgumentsDelta string
	Arguments      string
}

type ErrorStage string

const (
	StageConfig   ErrorStage = "config"
	StageRequest  ErrorStage = "request"
	StageResponse ErrorStage = "response"
	StageStream   ErrorStage = "stream"
)

type AppError struct {
	Stage      ErrorStage
	StatusCode int
	Message    string
	Cause      error
}

func (e *AppError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Stage, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("%s: %s", e.Stage, e.Message)
}

func (e *AppError) Unwrap() error { return e.Cause }

func UserError(err error) string {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Error()
	}
	if err != nil {
		return "request failed: " + err.Error()
	}
	return "request failed"
}

// Sanitize removes a configured credential from errors that may contain
// provider-supplied text while preserving the structured error metadata.
func Sanitize(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	if appErr, ok := err.(*AppError); ok {
		copy := *appErr
		copy.Message = strings.ReplaceAll(copy.Message, secret, "[redacted]")
		return &copy
	}
	return fmt.Errorf("request failed: %s", strings.ReplaceAll(err.Error(), secret, "[redacted]"))
}
