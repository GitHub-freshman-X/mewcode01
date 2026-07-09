package agent

import (
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestTaskHistoryMultiRound(t *testing.T) {
	base := []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "existing"}}}}
	history := newTaskHistory(base)
	base[0].Blocks[0].Text = "changed"
	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "plan"}}}
	call := provider.ToolCall{ID: "1", Name: "read_file", Arguments: []byte(`{"path":"README.md"}`)}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockToolCall, ToolCall: &call}}}
	result := provider.ToolResult{CallID: "1", Name: "read_file", Content: "ok"}
	if err := history.CommitRound(&user, assistant, []provider.ToolResult{result}); err != nil {
		t.Fatal(err)
	}
	snapshot := history.Snapshot()
	if len(snapshot) != 4 || snapshot[0].Blocks[0].Text != "existing" || snapshot[3].Blocks[0].ToolResult.CallID != "1" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	snapshot[0].Blocks[0].Text = "mutated"
	if history.Snapshot()[0].Blocks[0].Text != "existing" {
		t.Fatal("task history snapshot shares state")
	}
}
