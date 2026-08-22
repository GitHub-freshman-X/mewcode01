package agent

import (
	"context"
	"os"
	"path/filepath"

	contextmanager "github.com/GitHub-freshman-X/mewcode01/internal/context"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/hooks"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/memory"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
	"github.com/GitHub-freshman-X/mewcode01/internal/prompt"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/skills"
	"github.com/GitHub-freshman-X/mewcode01/internal/subagent"
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
	Skill  *SkillInvocation
}

type SkillInvocation struct {
	Name string
	Args string
}

const (
	DefaultMaxIterations     = 20
	UnknownToolStopThreshold = 3
)

type Options struct {
	MaxIterations   int
	MaxTokens       int
	Model           string
	Thinking        provider.ThinkingOptions
	Workspace       string
	LogDirectory    string
	SessionID       string
	SessionStore    *conversation.SessionStore
	Clock           prompt.Clock
	Injection       prompt.InjectionPolicy
	OptionalModules prompt.OptionalModules
	Memory          *memory.Service
	Permissions     *permissions.Engine
	Confirmer       PermissionBridge
	Context         contextmanager.Config
	Logger          *logging.Logger
	Skills          *skills.Manager
	Hooks           *hooks.Engine
	SubAgents       *SubAgentRuntime
	SystemPrompt    string
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
	if o.LogDirectory == "" {
		o.LogDirectory = filepath.Join(o.Workspace, "logs")
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
	EventSubAgent           EventType = "subagent"
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
	SubAgent               *subagent.TaskInfo
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
