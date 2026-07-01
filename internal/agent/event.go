package agent

import (
	"context"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type Mode string

const (
	ModeAct  Mode = "act"
	ModePlan Mode = "plan"
	ModeDo   Mode = "do"
)

type Request struct {
	Mode   Mode
	Prompt string
}

const (
	DefaultMaxIterations     = 20
	UnknownToolStopThreshold = 3
)

type Options struct {
	MaxIterations int
	MaxTokens     int
	Thinking      provider.ThinkingOptions
}

func (o Options) normalized() Options {
	if o.MaxIterations <= 0 {
		o.MaxIterations = DefaultMaxIterations
	}
	if o.MaxTokens <= 0 {
		o.MaxTokens = 4096
	}
	return o
}

type EventType string

const (
	EventProgress   EventType = "progress"
	EventTextDelta  EventType = "text_delta"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventUsage      EventType = "usage"
	EventCompleted  EventType = "completed"
	EventStopped    EventType = "stopped"
	EventCancelled  EventType = "cancelled"
	EventFailed     EventType = "failed"
)

type Phase string

const (
	PhaseCallingModel Phase = "calling_model"
	PhaseStreaming    Phase = "streaming"
	PhaseRunningTools Phase = "running_tools"
	PhaseFinishing    Phase = "finishing"
)

type StopReason string

const (
	StopFinalAnswer      StopReason = "final_answer"
	StopIterationLimit   StopReason = "iteration_limit"
	StopUnknownToolLimit StopReason = "unknown_tool_limit"
	StopCancelled        StopReason = "cancelled"
	StopStreamError      StopReason = "stream_error"
)

type Summary struct {
	Reason     StopReason
	Iterations int
	Usage      provider.Usage
	Partial    bool
}

type Event struct {
	Type       EventType
	Iteration  int
	Phase      Phase
	Text       string
	ToolCall   *provider.ToolCall
	ToolResult *provider.ToolResult
	Usage      *provider.Usage
	Summary    *Summary
	Err        error
}

type Task struct {
	Events <-chan Event
	Cancel context.CancelFunc
}

func isTerminal(typ EventType) bool {
	return typ == EventCompleted || typ == EventStopped || typ == EventCancelled || typ == EventFailed
}
