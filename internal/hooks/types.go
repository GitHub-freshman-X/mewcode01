package hooks

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Event string

const (
	EventSessionStart      Event = "session_start"
	EventSessionEnd        Event = "session_end"
	EventTurnStart         Event = "turn_start"
	EventTurnEnd           Event = "turn_end"
	EventPreSend           Event = "pre_send"
	EventPostReceive       Event = "post_receive"
	EventPreToolUse        Event = "pre_tool_use"
	EventPostToolUse       Event = "post_tool_use"
	EventStartup           Event = "startup"
	EventShutdown          Event = "shutdown"
	EventError             Event = "error"
	EventCompact           Event = "compact"
	EventPermissionRequest Event = "permission_request"
	EventFileChange        Event = "file_change"
	EventCommandExecute    Event = "command_execute"
)

func (e Event) Valid() bool {
	switch e {
	case EventSessionStart, EventSessionEnd, EventTurnStart, EventTurnEnd, EventPreSend, EventPostReceive, EventPreToolUse, EventPostToolUse, EventStartup, EventShutdown, EventError, EventCompact, EventPermissionRequest, EventFileChange, EventCommandExecute:
		return true
	default:
		return false
	}
}

type ActionType string

const (
	ActionCommand ActionType = "command"
	ActionPrompt  ActionType = "prompt"
	ActionHTTP    ActionType = "http"
	ActionAgent   ActionType = "agent"
)

func (a ActionType) Valid() bool {
	return a == ActionCommand || a == ActionPrompt || a == ActionHTTP || a == ActionAgent
}

type Action struct {
	Type       ActionType        `yaml:"type"`
	Command    string            `yaml:"command,omitempty"`
	Timeout    time.Duration     `yaml:"-"`
	RawTimeout string            `yaml:"timeout,omitempty"`
	Message    string            `yaml:"message,omitempty"`
	URL        string            `yaml:"url,omitempty"`
	Method     string            `yaml:"method,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty"`
	Body       string            `yaml:"body,omitempty"`
	Prompt     string            `yaml:"prompt,omitempty"`
}

type Rule struct {
	ID        string         `yaml:"id,omitempty"`
	Event     Event          `yaml:"event"`
	If        string         `yaml:"if,omitempty"`
	Condition ConditionGroup `yaml:"-"`
	Action    Action         `yaml:"action"`
	Reject    bool           `yaml:"reject,omitempty"`
	Once      bool           `yaml:"once,omitempty"`
	Async     bool           `yaml:"async,omitempty"`
	Index     int            `yaml:"-"`
}

type Context struct {
	Event    Event
	Tool     string
	Args     map[string]any
	FilePath string
	Message  string
	Error    string
}

func (c Context) Field(name string) string {
	switch name {
	case "event":
		return string(c.Event)
	case "tool":
		return c.Tool
	}
	if strings.HasPrefix(name, "args.") {
		return stringify(c.Args[strings.TrimPrefix(name, "args.")])
	}
	return ""
}

func (c Context) Expand(template string) string {
	replacer := strings.NewReplacer(
		"$EVENT", string(c.Event), "$TOOL_NAME", c.Tool, "$FILE_PATH", c.FilePath,
		"$MESSAGE", c.Message, "$ERROR", c.Error,
	)
	result := replacer.Replace(template)
	keys := make([]string, 0, len(c.Args))
	for key := range c.Args {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, key := range keys {
		result = strings.ReplaceAll(result, "$TOOL_ARGS."+key, stringify(c.Args[key]))
	}
	return toolArgumentVariable.ReplaceAllString(result, "")
}

var toolArgumentVariable = regexp.MustCompile(`\$TOOL_ARGS\.[A-Za-z0-9_]+`)

func stringify(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

type Result struct {
	RuleID     string
	Action     ActionType
	Output     string
	Err        error
	Rejected   bool
	Async      bool
	Duration   time.Duration
	ExitCode   int
	StatusCode int
}

type PromptSink interface{ AddHookNotification(string) }
type AgentRunner interface {
	RunHookAgent(context.Context, string) (string, error)
}
