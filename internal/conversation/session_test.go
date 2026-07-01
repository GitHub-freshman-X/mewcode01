package conversation

import (
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestSessionSnapshotIsolation(t *testing.T) {
	s := NewSession()
	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "task"}}}
	call := provider.ToolCall{ID: "1", Name: "read_file", Arguments: []byte(`{"path":"a"}`)}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockToolCall, ToolCall: &call}}}
	result := provider.ToolResult{CallID: "1", Name: "read_file", Content: "ok"}
	if err := s.CommitRound(&user, assistant, []provider.ToolResult{result}); err != nil {
		t.Fatal(err)
	}
	snapshot := s.Snapshot()
	snapshot[0].Blocks[0].Text = "changed"
	snapshot[1].Blocks[0].ToolCall.Arguments[0] = 'x'
	again := s.Snapshot()
	if again[0].Blocks[0].Text != "task" || string(again[1].Blocks[0].ToolCall.Arguments) != `{"path":"a"}` {
		t.Fatal("snapshot shares mutable state")
	}
}

func TestSessionPlanAndAtomicValidation(t *testing.T) {
	s := NewSession()
	if err := s.SavePlan("  first  "); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePlan(" "); err == nil {
		t.Fatal("empty plan accepted")
	}
	if plan, _ := s.LatestPlan(); plan != "first" {
		t.Fatalf("plan=%q", plan)
	}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockToolCall, ToolCall: &provider.ToolCall{ID: "1", Name: "x", Arguments: []byte(`{}`)}}}}
	if err := s.CommitRound(nil, assistant, nil); err == nil {
		t.Fatal("incomplete round accepted")
	}
	if len(s.Snapshot()) != 0 {
		t.Fatal("invalid round was partially committed")
	}
}
