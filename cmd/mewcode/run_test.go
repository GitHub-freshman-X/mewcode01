package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
	"github.com/GitHub-freshman-X/mewcode01/internal/tools"
)

type runProvider struct{ events []provider.StreamEvent }

func (p runProvider) Stream(_ context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	events := make(chan provider.StreamEvent, len(p.events))
	done := make(chan error, 1)
	for _, event := range p.events {
		events <- event
	}
	close(events)
	done <- nil
	close(done)
	return events, done
}

type blockingRunProvider struct{}

func (blockingRunProvider) Stream(ctx context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, <-chan error) {
	events := make(chan provider.StreamEvent)
	done := make(chan error, 1)
	go func() {
		<-ctx.Done()
		done <- ctx.Err()
		close(done)
		close(events)
	}()
	return events, done
}

func newRunTestRunner(t *testing.T, p provider.Provider) *agent.Runner {
	t.Helper()
	root := t.TempDir()
	registry, err := tools.NewDefaultRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	return agent.NewRunner(p, conversation.NewSession(), registry, tools.NewExecutor(time.Second), agent.Options{Workspace: root, MaxIterations: 2, MaxTokens: 32})
}

func TestParseRunOptions(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
		err  bool
	}{
		{name: "prompt", args: []string{"--prompt", " hello "}, want: "hello"},
		{name: "missing", err: true},
		{name: "both", args: []string{"--prompt", "x", "--prompt-file", "task.md"}, err: true},
		{name: "empty", args: []string{"--prompt", "  "}, err: true},
		{name: "negative timeout", args: []string{"--prompt", "x", "--timeout", "-1s"}, err: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			options, err := parseRunOptions(test.args, io.Discard)
			if (err != nil) != test.err {
				t.Fatalf("err=%v", err)
			}
			if options.prompt != test.want {
				t.Fatalf("prompt=%q want=%q", options.prompt, test.want)
			}
		})
	}
}

func TestExecuteNonInteractiveTextAndJSON(t *testing.T) {
	provider := runProvider{events: []provider.StreamEvent{
		{Type: provider.EventTextDelta, Delta: "hello "},
		{Type: provider.EventTextDelta, Delta: "world"},
		{Type: provider.EventUsage, Usage: &provider.Usage{InputTokens: 3, OutputTokens: 2}},
		{Type: provider.EventCompleted},
	}}
	t.Run("text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := executeNonInteractive(newRunTestRunner(t, provider), runOptions{prompt: "task", timeout: time.Minute}, &stdout, &stderr)
		if code != exitCompleted || stdout.String() != "hello world" || stderr.Len() != 0 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := executeNonInteractive(newRunTestRunner(t, provider), runOptions{prompt: "task", timeout: time.Minute, jsonOutput: true}, &stdout, &stderr)
		var result runResult
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if code != exitCompleted || result.Status != "completed" || result.FinalText != "hello world" || result.Usage.InputTokens != 3 || stderr.Len() != 0 {
			t.Fatalf("code=%d result=%+v stderr=%q", code, result, stderr.String())
		}
	})
}

func TestExecuteNonInteractiveTimeout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := executeNonInteractive(newRunTestRunner(t, blockingRunProvider{}), runOptions{prompt: "task", timeout: time.Millisecond, jsonOutput: true}, &stdout, &stderr)
	var result runResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != exitTimedOut || result.Status != "timed_out" || stderr.String() != "agent task timed out\n" {
		t.Fatalf("code=%d result=%+v stderr=%q", code, result, stderr.String())
	}
}
