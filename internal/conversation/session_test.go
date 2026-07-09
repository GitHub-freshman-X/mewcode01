package conversation

import (
	"strings"
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
	display := s.DisplaySnapshot()
	if len(display) != len(again) {
		t.Fatalf("display=%d history=%d", len(display), len(again))
	}
	display[0].Blocks[0].Text = "display changed"
	if s.Snapshot()[0].Blocks[0].Text != "task" || s.DisplaySnapshot()[0].Blocks[0].Text != "task" {
		t.Fatal("model and display snapshots are not isolated")
	}
}

func TestSessionCommitPlanAndHistoryIsolation(t *testing.T) {
	s := NewSession()
	for _, plan := range []string{"  first  ", "second"} {
		if err := commitTestPlan(s, plan); err != nil {
			t.Fatal(err)
		}
	}
	if err := commitTestPlan(s, " "); err == nil {
		t.Fatal("empty plan accepted")
	}
	plans := s.PendingPlans()
	if len(plans) != 2 || plans[0] != "first" || plans[1] != "second" {
		t.Fatalf("plans=%q", plans)
	}
	plans[0] = "changed"
	if got := s.PendingPlans()[0]; got != "first" {
		t.Fatalf("pending plans share returned slice: %q", got)
	}
	if len(s.Snapshot()) != 0 {
		t.Fatalf("plan leaked into model history: %+v", s.Snapshot())
	}
	if got := len(s.DisplaySnapshot()); got != 4 {
		t.Fatalf("display messages=%d", got)
	}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockToolCall, ToolCall: &provider.ToolCall{ID: "1", Name: "x", Arguments: []byte(`{}`)}}}}
	if err := s.CommitRound(nil, assistant, nil); err == nil {
		t.Fatal("incomplete round accepted")
	}
	if len(s.Snapshot()) != 0 {
		t.Fatal("invalid round was partially committed")
	}
}

func TestSessionPlanConsumption(t *testing.T) {
	s := NewSession()
	for _, plan := range []string{"first", "second"} {
		if err := commitTestPlan(s, plan); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := s.PendingPlans()
	if err := commitTestPlan(s, "third"); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumePlans(snapshot); err != nil {
		t.Fatal(err)
	}
	if got := s.PendingPlans(); len(got) != 1 || got[0] != "third" {
		t.Fatalf("remaining plans=%q", got)
	}

	for name, plans := range map[string][]string{
		"empty":    nil,
		"wrong":    {"other"},
		"too long": {"third", "fourth"},
	} {
		t.Run(name, func(t *testing.T) {
			before := s.PendingPlans()
			if err := s.ConsumePlans(plans); err == nil {
				t.Fatal("invalid snapshot was consumed")
			}
			after := s.PendingPlans()
			if len(after) != len(before) || after[0] != before[0] {
				t.Fatalf("plans changed: before=%q after=%q", before, after)
			}
		})
	}
}

func TestBuildRoundValidation(t *testing.T) {
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockToolCall, ToolCall: &provider.ToolCall{ID: "1", Name: "read_file", Arguments: []byte(`{}`)}}}}
	result := provider.ToolResult{CallID: "1", Name: "read_file", Content: "ok"}
	messages, err := BuildRound(nil, assistant, []provider.ToolResult{result})
	if err != nil || len(messages) != 2 || messages[1].Blocks[0].ToolResult.CallID != "1" {
		t.Fatalf("messages=%+v err=%v", messages, err)
	}
	if _, err := BuildRound(nil, assistant, nil); err == nil {
		t.Fatal("incomplete round accepted")
	}
}

func commitTestPlan(s *Session, plan string) error {
	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "/plan task"}}}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: strings.TrimSpace(plan)}}}
	return s.CommitPlan(user, assistant, plan)
}
