package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/subagent"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

func subAgentTestRunner(t *testing.T, p *scriptedProvider, definitions []subagent.Definition) *Runner {
	t.Helper()
	registry, err := tools.NewDefaultRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(tools.NewAgentTool()); err != nil {
		t.Fatal(err)
	}
	catalog, err := subagent.NewRegistry(definitions)
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewSubAgentRuntime(catalog, subagent.NewTaskManager())
	runtime.AutoBackground = time.Hour
	return NewRunner(p, conversation.NewSession(), registry, tools.NewExecutor(time.Second), Options{SubAgents: runtime})
}

func TestDefinitionSubAgentRunsToCompletion(t *testing.T) {
	definition := subagent.Definition{Name: "reader", Description: "read", SystemPrompt: "ROLE SYSTEM", MaxTurns: 3}
	p := &scriptedProvider{rounds: []scriptedRound{
		toolRound(provider.ToolCall{ID: "agent-call", Name: "agent", Arguments: []byte(`{"prompt":"inspect","description":"inspect code","subagent_type":"reader"}`)}),
		textRound("child report", provider.Usage{InputTokens: 2, OutputTokens: 1}),
		textRound("parent done", provider.Usage{}),
	}}
	runner := subAgentTestRunner(t, p, []subagent.Definition{definition})
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "delegate"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	if len(p.requests) != 3 || p.requests[1].Prompt.StableSystem != "ROLE SYSTEM" {
		t.Fatalf("requests=%+v", p.requests)
	}
	if len(p.requests[1].Messages) != 1 || messageText(p.requests[1].Messages[0]) != "inspect" {
		t.Fatalf("child messages=%+v", p.requests[1].Messages)
	}
}

func TestForkSubAgentPreservesHistoryAndRunsBackground(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		toolRound(provider.ToolCall{ID: "agent-call", Name: "agent", Arguments: []byte(`{"prompt":"inspect","description":"inspect code"}`)}),
		textRound("child report", provider.Usage{}),
		textRound("parent done", provider.Usage{}),
	}}
	runner := subAgentTestRunner(t, p, nil)
	if err := runner.session.CommitRound(&provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "before"}}}, provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "answer"}}}, nil); err != nil {
		t.Fatal(err)
	}
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "delegate"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	var child *provider.ChatRequest
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		requests := append([]provider.ChatRequest(nil), p.requests...)
		p.mu.Unlock()
		for i := range requests {
			messages := requests[i].Messages
			if len(messages) > 0 && strings.Contains(messageText(messages[len(messages)-1]), "fork_boilerplate") {
				child = &requests[i]
				break
			}
		}
		if child != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if child == nil || len(child.Messages) < 3 || messageText(child.Messages[0]) != "before" || !strings.Contains(messageText(child.Messages[len(child.Messages)-1]), "fork_boilerplate") {
		p.mu.Lock()
		requests := append([]provider.ChatRequest(nil), p.requests...)
		p.mu.Unlock()
		t.Fatalf("fork requests=%+v", requests)
	}
}
