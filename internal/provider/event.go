package provider

import (
	"fmt"
	"strings"
)

type EventType string

const (
	EventStarted        EventType = "started"
	EventThinkingDelta  EventType = "thinking_delta"
	EventTextDelta      EventType = "text_delta"
	EventSignatureDelta EventType = "signature_delta"
	EventCompleted      EventType = "completed"
)

type StreamEvent struct {
	Type       EventType
	BlockIndex int
	Delta      string
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
	return "request failed; check your connection and configuration"
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
