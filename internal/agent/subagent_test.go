package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/subagent"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
	"github.com/GitHub-freshman-X/mewcode01/internal/worktree"
)

func subAgentTestRunner(t *testing.T, p *scriptedProvider, definitions []subagent.Definition) *Runner {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "README"}, {"commit", "-m", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	registry, err := tools.NewDefaultRegistry(root)
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
	runtime.Worktrees = worktree.NewManager(root)
	return NewRunner(p, conversation.NewSession(), registry, tools.NewExecutor(time.Second), Options{SubAgents: runtime, Workspace: root})
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

func TestLogSubAgentTerminalWritesSafeMetadata(t *testing.T) {
	root := t.TempDir()
	logger, err := logging.New(root, func() time.Time { return time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC) }, 1)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	for _, status := range []subagent.TaskStatus{subagent.TaskCompleted, subagent.TaskFailed, subagent.TaskCancelled} {
		logSubAgentTerminal(logger, subagent.CreationDefinition, "child-model", subagent.TaskInfo{
			Status: status, Background: true, Result: "result-canary", Failure: "failure-canary",
			StartedAt: started, EndedAt: started.Add(1500 * time.Millisecond), ToolCalls: 2,
			Usage: provider.Usage{InputTokens: 3, OutputTokens: 4, CacheReadInputTokens: 5, CacheCreationInputTokens: 6},
		})
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, "logs", "*", "*", "*", "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines=%d", len(lines))
	}
	for i, line := range lines {
		var event logging.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Message != "subagent finished" || event.Fields["stage"] != "subagent" || event.Fields["status"] != string([]subagent.TaskStatus{subagent.TaskCompleted, subagent.TaskFailed, subagent.TaskCancelled}[i]) || event.Fields["mode"] != "definition" || event.Fields["background"] != true || event.Fields["model"] != "child-model" || event.Fields["tool_calls"] != float64(2) || event.Fields["duration_ms"] != float64(1500) {
			t.Fatalf("event=%+v", event)
		}
		for _, key := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens"} {
			if _, ok := event.Fields[key]; !ok {
				t.Fatalf("missing %s in %+v", key, event.Fields)
			}
		}
		encoded, _ := json.Marshal(event)
		if strings.Contains(string(encoded), "result-canary") || strings.Contains(string(encoded), "failure-canary") {
			t.Fatalf("unsafe terminal log: %s", encoded)
		}
	}
}

func TestDefinitionSubAgentRegistersTerminalLogCallback(t *testing.T) {
	definition := subagent.Definition{Name: "reader", Description: "read", SystemPrompt: "ROLE SYSTEM", MaxTurns: 3}
	p := &scriptedProvider{rounds: []scriptedRound{
		toolRound(provider.ToolCall{ID: "agent-call", Name: "agent", Arguments: []byte(`{"prompt":"inspect","description":"inspect code","subagent_type":"reader"}`)}),
		textRound("child report", provider.Usage{InputTokens: 2, OutputTokens: 1}),
		textRound("parent done", provider.Usage{}),
	}}
	runner := subAgentTestRunner(t, p, []subagent.Definition{definition})
	root := t.TempDir()
	logger, err := logging.New(root, time.Now, 1)
	if err != nil {
		t.Fatal(err)
	}
	runner.options.Logger = logger
	runner.options.Model = "actual-child-model"
	task, err := runner.Start(context.Background(), Request{Mode: ModeAct, Prompt: "delegate"})
	if err != nil {
		t.Fatal(err)
	}
	drainTask(t, task)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		files, _ := filepath.Glob(filepath.Join(root, "logs", "*", "*", "*", "*.jsonl"))
		if len(files) == 1 {
			body, readErr := os.ReadFile(files[0])
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
				var event logging.Event
				if json.Unmarshal([]byte(line), &event) == nil && event.Message == "subagent finished" {
					if event.Fields["status"] != "completed" || event.Fields["mode"] != "definition" || event.Fields["background"] != false || event.Fields["model"] != "actual-child-model" || event.Fields["input_tokens"] != float64(2) || event.Fields["output_tokens"] != float64(1) {
						t.Fatalf("event=%+v", event)
					}
					if err := logger.Close(); err != nil {
						t.Fatal(err)
					}
					return
				}
			}
		}
		time.Sleep(time.Millisecond)
	}
	_ = logger.Close()
	t.Fatal("missing definition subagent terminal log")
}

func TestForegroundSubAgentContextDetachesParentDeadline(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelParent()
	child, cancelChild := foregroundSubAgentContext(parent)
	defer cancelChild()
	if _, ok := child.Deadline(); ok {
		t.Fatal("foreground child inherited the parent deadline")
	}
	<-parent.Done()
	select {
	case <-child.Done():
		t.Fatal("foreground child was cancelled before an explicit foreground cancellation")
	default:
	}
	cancelChild()
	select {
	case <-child.Done():
	case <-time.After(time.Second):
		t.Fatal("explicit foreground cancellation did not stop child")
	}
}

func TestForkTypeRecognizesOmittedAndAlias(t *testing.T) {
	for _, subagentType := range []string{"", " ", "fork", " FoRk "} {
		if !isForkType(subagentType) {
			t.Fatalf("type %q was not recognized as fork", subagentType)
		}
	}
	if isForkType("general-purpose") {
		t.Fatal("definition type was recognized as fork")
	}
}

func TestForkSubAgentPreservesHistoryAndRunsBackground(t *testing.T) {
	p := &scriptedProvider{rounds: []scriptedRound{
		toolRound(provider.ToolCall{ID: "agent-call", Name: "agent", Arguments: []byte(`{"prompt":"inspect","description":"inspect code","subagent_type":"fork"}`)}),
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
