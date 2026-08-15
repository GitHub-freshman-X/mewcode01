package conversation

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestJSONLJournalEncodesProviderNeutralRound(t *testing.T) {
	var output bytes.Buffer
	journal := NewJSONLJournal(&output)
	journal.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "read the file"}}}
	call := provider.ToolCall{ID: "call-1", Name: "read_file", Arguments: []byte(`{"path":"README.md"}`)}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{
		{Type: provider.BlockText, Text: "I will inspect it."},
		{Type: provider.BlockToolCall, ToolCall: &call},
	}}
	result := provider.ToolResult{CallID: "call-1", Name: "read_file", Content: "contents", IsError: true}
	messages, err := BuildRound(&user, assistant, []provider.ToolResult{result})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(messages, JournalPurposeHistory); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("records=%d", len(lines))
	}
	var assistantRecord, resultRecord JournalRecord
	if err := json.Unmarshal([]byte(lines[1]), &assistantRecord); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[2]), &resultRecord); err != nil {
		t.Fatal(err)
	}
	if assistantRecord.Role != provider.RoleAssistant || assistantRecord.Content != "I will inspect it." || assistantRecord.Purpose != JournalPurposeHistory || assistantRecord.Timestamp != 1_700_000_000 {
		t.Fatalf("assistant record=%+v", assistantRecord)
	}
	if len(assistantRecord.ToolUses) != 1 || assistantRecord.ToolUses[0].ID != "call-1" || assistantRecord.ToolUses[0].Name != "read_file" || string(assistantRecord.ToolUses[0].Arguments) != `{"path":"README.md"}` {
		t.Fatalf("tool uses=%+v", assistantRecord.ToolUses)
	}
	if len(resultRecord.ToolResults) != 1 || resultRecord.ToolResults[0].CallID != "call-1" || resultRecord.ToolResults[0].Name != "read_file" || resultRecord.ToolResults[0].Content != "contents" || !resultRecord.ToolResults[0].IsError {
		t.Fatalf("tool results=%+v", resultRecord.ToolResults)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["blocks"]; ok {
		t.Fatalf("provider-specific blocks were persisted: %s", lines[1])
	}
}

func TestSessionRecordUsagePersistsOnlyTokenCounts(t *testing.T) {
	var output bytes.Buffer
	journal := NewJSONLJournal(&output)
	journal.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	session := NewSession(journal)
	if err := session.RecordUsage(provider.Usage{InputTokens: 12, OutputTokens: 5, CacheReadInputTokens: 3}); err != nil {
		t.Fatal(err)
	}
	if err := session.RecordUsage(provider.Usage{InputTokens: 8, OutputTokens: 2}); err != nil {
		t.Fatal(err)
	}
	if got := session.Usage(); got.InputTokens != 20 || got.OutputTokens != 7 {
		t.Fatalf("usage=%+v", got)
	}
	var record JournalRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.Split(output.String(), "\n")[0])), &record); err != nil {
		t.Fatal(err)
	}
	if record.Purpose != JournalPurposeUsage || record.Usage == nil || record.Usage.InputTokens != 12 || record.Usage.OutputTokens != 5 || record.Role != "" {
		t.Fatalf("record=%+v", record)
	}
	if strings.Contains(output.String(), "CacheRead") {
		t.Fatalf("journal contains unsupported usage fields: %s", output.String())
	}
}

func TestSessionRecordUsageJournalFailureLeavesUsageUnchanged(t *testing.T) {
	session := NewSession(&recordingJournal{})
	if err := session.RecordUsage(provider.Usage{InputTokens: 1}); err == nil {
		t.Fatal("usage was accepted without usage journal support")
	}
	if got := session.Usage(); got != (provider.Usage{}) {
		t.Fatalf("usage=%+v", got)
	}
}

func TestSessionCommitPlanWritesPlanJournalRecords(t *testing.T) {
	journal := &recordingJournal{}
	s := NewSession(journal)
	if err := commitTestPlan(s, "create a file"); err != nil {
		t.Fatal(err)
	}
	if len(journal.entries) != 1 {
		t.Fatalf("append calls=%d", len(journal.entries))
	}
	entry := journal.entries[0]
	if entry.purpose != JournalPurposePlan || len(entry.messages) != 2 || entry.messages[0].Blocks[0].Text != "/plan task" || entry.messages[1].Blocks[0].Text != "create a file" {
		t.Fatalf("entry=%+v", entry)
	}
}

func TestSessionJournalFailureLeavesMemoryUnchanged(t *testing.T) {
	s := NewSession(failingJournal{err: errors.New("disk full")})
	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "task"}}}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "answer"}}}
	if err := s.CommitRound(&user, assistant, nil); err == nil {
		t.Fatal("round journal failure was accepted")
	}
	if len(s.Snapshot()) != 0 || len(s.DisplaySnapshot()) != 0 || len(s.PendingPlans()) != 0 {
		t.Fatal("round journal failure changed session memory")
	}
	if err := s.CommitPlan(user, assistant, "make a plan"); err == nil {
		t.Fatal("plan journal failure was accepted")
	}
	if len(s.Snapshot()) != 0 || len(s.DisplaySnapshot()) != 0 || len(s.PendingPlans()) != 0 {
		t.Fatal("plan journal failure changed session memory")
	}
}

func TestSessionReplaceHistoryDoesNotWriteJournal(t *testing.T) {
	journal := &recordingJournal{}
	s := NewSession(journal)
	s.ReplaceHistory([]provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "compressed"}}}})
	if len(journal.entries) != 0 {
		t.Fatalf("replace history wrote %d journal entries", len(journal.entries))
	}
}

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

func TestSessionReplaceHistoryDeepCopiesAndPreservesDisplayAndPlans(t *testing.T) {
	s := NewSession()
	if err := commitTestPlan(s, "keep plan"); err != nil {
		t.Fatal(err)
	}
	replacement := []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "compressed"}}}}
	s.ReplaceHistory(replacement)
	replacement[0].Blocks[0].Text = "mutated"

	snapshot := s.Snapshot()
	if len(snapshot) != 1 || snapshot[0].Blocks[0].Text != "compressed" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	snapshot[0].Blocks[0].Text = "changed"
	if got := s.Snapshot()[0].Blocks[0].Text; got != "compressed" {
		t.Fatalf("history shares replacement or snapshot state: %q", got)
	}
	if got := len(s.DisplaySnapshot()); got != 2 {
		t.Fatalf("display changed during replace: %d", got)
	}
	if plans := s.PendingPlans(); len(plans) != 1 || plans[0] != "keep plan" {
		t.Fatalf("pending plans changed: %q", plans)
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

type journalEntry struct {
	messages []provider.Message
	purpose  JournalPurpose
}

type recordingJournal struct {
	entries []journalEntry
}

func (j *recordingJournal) Append(messages []provider.Message, purpose JournalPurpose) error {
	j.entries = append(j.entries, journalEntry{messages: provider.CloneMessages(messages), purpose: purpose})
	return nil
}

type failingJournal struct {
	err error
}

func (j failingJournal) Append([]provider.Message, JournalPurpose) error {
	return j.err
}
