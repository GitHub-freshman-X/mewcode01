package agent

import (
	"context"
	"os"

	contextmanager "github.com/GitHub-freshman-X/mewcode01/internal/context"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/memory"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/prompt"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type Mode string

const (
	ModeAct     Mode = "act"
	ModePlan    Mode = "plan"
	ModeDo      Mode = "do"
	ModeCompact Mode = "compact"
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
	MaxIterations   int
	MaxTokens       int
	Thinking        provider.ThinkingOptions
	Workspace       string
	SessionID       string
	Clock           prompt.Clock
	Injection       prompt.InjectionPolicy
	OptionalModules prompt.OptionalModules
	Memory          *memory.Service
	Permissions     *permissions.Engine
	Confirmer       PermissionBridge
	Context         contextmanager.Config
	Logger          *logging.Logger
}

type PermissionBridge interface {
	Confirm(context.Context, permissions.Decision) (permissions.Confirmation, error)
}

func (o Options) normalized() Options {
	if o.MaxIterations <= 0 {
		o.MaxIterations = DefaultMaxIterations
	}
	if o.MaxTokens <= 0 {
		o.MaxTokens = 4096
	}
	if o.Workspace == "" {
		if cwd, err := os.Getwd(); err == nil {
			o.Workspace = cwd
		}
	}
	if o.Clock == nil {
		o.Clock = prompt.SystemClock{}
	}
	o.Injection = prompt.NormalizeInjectionPolicy(o.Injection)
	if o.Context.WindowTokens == 0 {
		o.Context = contextmanager.DefaultConfig()
	}
	if o.Logger == nil {
		o.Logger = logging.Nop()
	}
	return o
}

type EventType string

const (
	EventProgress           EventType = "progress"
	EventTextDelta          EventType = "text_delta"
	EventToolCall           EventType = "tool_call"
	EventToolResult         EventType = "tool_result"
	EventPermissionDecision EventType = "permission_decision"
	EventPermissionRequest  EventType = "permission_request"
	EventPermissionResponse EventType = "permission_response"
	EventUsage              EventType = "usage"
	EventContextCompaction  EventType = "context_compaction"
	EventCompleted          EventType = "completed"
	EventStopped            EventType = "stopped"
	EventCancelled          EventType = "cancelled"
	EventFailed             EventType = "failed"
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

type CompactionEvent struct {
	Trigger      contextmanager.Trigger
	BeforeTokens int
	AfterTokens  int
	Persisted    []contextmanager.Persistence
	Error        string
}

type Event struct {
	Type                   EventType
	Iteration              int
	Phase                  Phase
	Text                   string
	ToolCall               *provider.ToolCall
	ToolResult             *provider.ToolResult
	PermissionDecision     *permissions.Decision
	PermissionConfirmation *permissions.Confirmation
	Usage                  *provider.Usage
	ContextCompaction      *CompactionEvent
	Summary                *Summary
	Err                    error
}

type Task struct {
	Events <-chan Event
	Cancel context.CancelFunc
}

func isTerminal(typ EventType) bool {
	return typ == EventCompleted || typ == EventStopped || typ == EventCancelled || typ == EventFailed
}
