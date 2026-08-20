package hooks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestContextExpandAndCondition(t *testing.T) {
	ctx := Context{Event: EventPreToolUse, Tool: "WriteFile", Args: map[string]any{"path": "main.go"}, FilePath: "main.go"}
	if got := ctx.Expand("$EVENT:$TOOL_NAME:$FILE_PATH:$TOOL_ARGS.path:$TOOL_ARGS.missing"); got != "pre_tool_use:WriteFile:main.go:main.go:" {
		t.Fatalf("expand=%q", got)
	}
	condition, err := ParseCondition(`tool == "WriteFile" && args.path ~= "*.go"`)
	if err != nil {
		t.Fatal(err)
	}
	if !condition.Match(ctx) {
		t.Fatal("condition did not match")
	}
	deep, err := ParseCondition(`args.path ~= "src/**/*.go"`)
	if err != nil || !deep.Match(Context{Args: map[string]any{"path": "src/a/b/main.go"}}) {
		t.Fatalf("double-star glob did not match: %v", err)
	}
	if _, err := ParseCondition(`tool == "WriteFile" && event == "x" || args.path ~= "*.go"`); err == nil {
		t.Fatal("mixed condition was accepted")
	}
}

func TestEngineRejectsFirstMatchingPreToolHook(t *testing.T) {
	rules := []Rule{{ID: "first", Event: EventPreToolUse, Action: Action{Type: ActionPrompt, Message: "blocked"}, Reject: true}, {ID: "second", Event: EventPreToolUse, Action: Action{Type: ActionPrompt, Message: "later"}, Reject: true}}
	engine := NewEngine(rules, Executor{PromptSink: &testSink{}}, nil)
	result, rejected := engine.RunPreTool(context.Background(), Context{Tool: "write_file"})
	if !rejected || result.RuleID != "first" || result.Output != "blocked" {
		t.Fatalf("result=%+v rejected=%v", result, rejected)
	}
}

func TestValidateRuleRejectsAsyncPreTool(t *testing.T) {
	rule := Rule{Event: EventPreToolUse, Async: true, Action: Action{Type: ActionCommand, Command: "true"}}
	if err := ValidateRule(&rule); err == nil {
		t.Fatal("async pre_tool_use was accepted")
	}
}

func TestExecutorActions(t *testing.T) {
	sink := &testSink{}
	executor := Executor{PromptSink: sink}
	if result := executor.Execute(context.Background(), Action{Type: ActionCommand, Command: "printf $TOOL_NAME"}, Context{Tool: "WriteFile"}); result.Err != nil || result.Output != "WriteFile" {
		t.Fatalf("command result=%+v", result)
	}
	if result := executor.Execute(context.Background(), Action{Type: ActionPrompt, Message: "hello $TOOL_NAME"}, Context{Tool: "ReadFile"}); result.Err != nil || sink.messages[0] != "hello ReadFile" {
		t.Fatalf("prompt result=%+v messages=%v", result, sink.messages)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method=%s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if result := executor.Execute(context.Background(), Action{Type: ActionHTTP, URL: server.URL, Method: http.MethodPut}, Context{}); result.Err != nil || result.StatusCode != http.StatusNoContent {
		t.Fatalf("http result=%+v", result)
	}
	if result := executor.Execute(context.Background(), Action{Type: ActionAgent, Prompt: "review"}, Context{}); result.Err == nil || result.Output == "" {
		t.Fatalf("agent result=%+v", result)
	}
}

func TestCommandTimeout(t *testing.T) {
	result := (Executor{}).Execute(context.Background(), Action{Type: ActionCommand, Command: "sleep 1", Timeout: 10 * time.Millisecond}, Context{})
	if result.Err == nil {
		t.Fatal("timeout command succeeded")
	}
}

func TestEngineRunsOnceRuleOnce(t *testing.T) {
	sink := &testSink{}
	engine := NewEngine([]Rule{{ID: "once", Event: EventTurnStart, Once: true, Action: Action{Type: ActionPrompt, Message: "once"}}}, Executor{PromptSink: sink}, nil)
	engine.Run(context.Background(), EventTurnStart, Context{})
	engine.Run(context.Background(), EventTurnStart, Context{})
	if len(sink.messages) != 1 {
		t.Fatalf("messages=%v", sink.messages)
	}
}

type testSink struct{ messages []string }

func (s *testSink) AddHookNotification(message string) { s.messages = append(s.messages, message) }
