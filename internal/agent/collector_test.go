package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestCollectorReassemblesResponse(t *testing.T) {
	events := make(chan provider.StreamEvent, 5)
	events <- provider.StreamEvent{Type: provider.EventTextDelta, BlockIndex: 0, Delta: "a"}
	events <- provider.StreamEvent{Type: provider.EventTextDelta, BlockIndex: 0, Delta: "b"}
	events <- provider.StreamEvent{Type: provider.EventToolCallStart, BlockIndex: 1, ToolCall: &provider.ToolCallDelta{ID: "1", Name: "x"}}
	events <- provider.StreamEvent{Type: provider.EventToolCallDelta, BlockIndex: 1, ToolCall: &provider.ToolCallDelta{ArgumentsDelta: `{}`}}
	events <- provider.StreamEvent{Type: provider.EventCompleted}
	close(events)
	done := make(chan error, 1)
	done <- nil
	close(done)
	var text string
	round, err := collectRound(context.Background(), 1, events, done, func(event Event) bool {
		if event.Type == EventTextDelta {
			text += event.Text
		}
		return true
	})
	if err != nil || text != "ab" || messageText(round.Assistant) != "ab" || len(round.ToolCalls) != 1 {
		t.Fatalf("round=%+v text=%q err=%v", round, text, err)
	}
}

func TestCollectorErrorDoesNotReturnRound(t *testing.T) {
	events := make(chan provider.StreamEvent, 2)
	events <- provider.StreamEvent{Type: provider.EventTextDelta, Delta: "partial"}
	close(events)
	done := make(chan error, 1)
	done <- errors.New("broken stream")
	close(done)
	seen := ""
	round, err := collectRound(context.Background(), 1, events, done, func(event Event) bool { seen += event.Text; return true })
	if err == nil || seen != "partial" || len(round.Assistant.Blocks) != 0 {
		t.Fatalf("round=%+v seen=%q err=%v", round, seen, err)
	}
}

func TestCollectorMalformedToolCall(t *testing.T) {
	for name, call := range map[string]*provider.ToolCallDelta{
		"missing id": {Name: "x", Arguments: `{}`}, "missing name": {ID: "1", Arguments: `{}`}, "bad json": {ID: "1", Name: "x", Arguments: `{`},
	} {
		t.Run(name, func(t *testing.T) {
			events := make(chan provider.StreamEvent, 2)
			events <- provider.StreamEvent{Type: provider.EventToolCallStart, ToolCall: call}
			events <- provider.StreamEvent{Type: provider.EventCompleted}
			close(events)
			done := make(chan error, 1)
			done <- nil
			close(done)
			if _, err := collectRound(context.Background(), 1, events, done, func(Event) bool { return true }); err == nil {
				t.Fatal("malformed call accepted")
			}
		})
	}
}

func TestCollectorStreamsImmediately(t *testing.T) {
	events := make(chan provider.StreamEvent)
	done := make(chan error, 1)
	emitted := make(chan string, 1)
	finished := make(chan error, 1)
	go func() {
		_, err := collectRound(context.Background(), 1, events, done, func(event Event) bool {
			if event.Type == EventTextDelta {
				emitted <- event.Text
			}
			return true
		})
		finished <- err
	}()
	events <- provider.StreamEvent{Type: provider.EventTextDelta, Delta: "now"}
	select {
	case text := <-emitted:
		if text != "now" {
			t.Fatalf("text=%q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("text was not forwarded immediately")
	}
	events <- provider.StreamEvent{Type: provider.EventCompleted}
	close(events)
	done <- nil
	close(done)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
