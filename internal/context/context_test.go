package context

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestDecisionUsesUsageAnchorAndDefaultThresholds(t *testing.T) {
	m := NewManager(DefaultConfig(), nil)
	m.RecordUsage(provider.Usage{InputTokens: 166999}, nil)
	if trigger, ok := m.Decision(nil, false); ok || trigger != "" {
		t.Fatalf("decision=(%q,%v)", trigger, ok)
	}
	m.RecordUsage(provider.Usage{InputTokens: 167000}, nil)
	if trigger, ok := m.Decision(nil, false); !ok || trigger != TriggerAutomatic {
		t.Fatalf("decision=(%q,%v)", trigger, ok)
	}
	m.RecordUsage(provider.Usage{InputTokens: 177000}, nil)
	if trigger, ok := m.Decision(nil, false); !ok || trigger != TriggerForced {
		t.Fatalf("decision=(%q,%v)", trigger, ok)
	}
}

func TestEstimateOnlyCountsMessagesAfterUsageAnchor(t *testing.T) {
	anchor := []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "old"}}}}
	m := NewManager(DefaultConfig(), nil)
	m.RecordUsage(provider.Usage{InputTokens: 100}, anchor)
	messages := append(provider.CloneMessages(anchor), provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "12345678"}}})
	if got := m.Estimate(messages); got != 102 {
		t.Fatalf("estimate=%d want 102", got)
	}
}

func TestPrepareResultsPersistsLargestResultsAndKeepsOrder(t *testing.T) {
	store, err := NewResultStore(t.TempDir(), "session")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SingleResultChars, cfg.MessageResultChars, cfg.PreviewChars = 10, 9, 3
	m := NewManager(cfg, store)
	results, persisted, err := m.PrepareResults([]provider.ToolResult{
		{CallID: "a", Name: "run", Content: "123456"},
		{CallID: "b", Name: "run", Content: "abcdef"},
		{CallID: "c", Name: "run", Content: "xyz"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 || results[0].CallID != "a" || results[1].CallID != "b" || results[2].CallID != "c" {
		t.Fatalf("persisted=%d results=%+v", len(persisted), results)
	}
	if !strings.Contains(results[0].Content+results[1].Content, "123") || !strings.Contains(results[0].Content+results[1].Content, "abc") || results[2].Content != "xyz" {
		t.Fatalf("contents=%q,%q,%q", results[0].Content, results[1].Content, results[2].Content)
	}
	if content, err := os.ReadFile(persisted[0].Path); err != nil || len(content) != 6 {
		t.Fatalf("stored content=%q err=%v", content, err)
	}
}

func TestPrepareResultsShrinksPersistedPreviewsToFitMessageBudget(t *testing.T) {
	store, err := NewResultStore(t.TempDir(), "session")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SingleResultChars = 10
	cfg.MessageResultChars = 650
	cfg.PreviewChars = 120
	m := NewManager(cfg, store)
	results, persisted, err := m.PrepareResults([]provider.ToolResult{
		{CallID: "a", Name: "run", Content: strings.Repeat("a", 80)},
		{CallID: "b", Name: "run", Content: strings.Repeat("b", 80)},
		{CallID: "c", Name: "run", Content: strings.Repeat("c", 80)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 3 {
		t.Fatalf("persisted=%d", len(persisted))
	}
	total := 0
	for _, result := range results {
		total += len(result.Content)
	}
	if total > cfg.MessageResultChars {
		t.Fatalf("final result message len=%d budget=%d contents=%+v", total, cfg.MessageResultChars, results)
	}
}

func TestPrepareResultsDoesNotRepersistReadback(t *testing.T) {
	store, err := NewResultStore(t.TempDir(), "session")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SingleResultChars = 5
	m := NewManager(cfg, store)
	_, persisted, err := m.PrepareResults([]provider.ToolResult{{CallID: "a", Name: "run", Content: "123456"}})
	if err != nil || len(persisted) != 1 {
		t.Fatalf("persisted=%v err=%v", persisted, err)
	}
	content, err := os.ReadFile(persisted[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	results, again, err := m.PrepareResults([]provider.ToolResult{{CallID: "b", Name: "read_file", Content: string(content)}})
	if err != nil || len(again) != 0 || results[0].Content != string(content) {
		t.Fatalf("results=%+v persisted=%v err=%v", results, again, err)
	}
}

func TestPrepareResultsDoesNotRepersistWrappedReadFileContent(t *testing.T) {
	store, err := NewResultStore(t.TempDir(), "session")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SingleResultChars = 5
	m := NewManager(cfg, store)
	_, persisted, err := m.PrepareResults([]provider.ToolResult{{CallID: "a", Name: "run", Content: "123456"}})
	if err != nil || len(persisted) != 1 {
		t.Fatalf("persisted=%v err=%v", persisted, err)
	}
	wrapped := `{"tool_name":"read_file","success":true,"data":{"path":"` + persisted[0].Path + `","content":"123456","truncated":false}}`
	results, again, err := m.PrepareResults([]provider.ToolResult{{CallID: "b", Name: "read_file", Content: wrapped}})
	if err != nil || len(again) != 0 || results[0].Content != wrapped {
		t.Fatalf("results=%+v persisted=%v err=%v", results, again, err)
	}
}

func TestSummaryRequestUsesNoToolsAndDocumentsRequiredContract(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SummaryOutputTokens = 1234
	m := NewManager(cfg, nil)
	req := m.BuildSummaryRequest([]provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "please remember this exact user request"}}}}, TriggerAutomatic)

	if req.MaxTokens != 1234 {
		t.Fatalf("max tokens=%d", req.MaxTokens)
	}
	if len(req.Tools) != 0 {
		t.Fatalf("summary request tools=%d", len(req.Tools))
	}
	contract := req.Prompt.StableSystem + "\n" + messagesText(req.Messages)
	for _, want := range []string{
		"Do not call tools",
		"discard analysis drafts",
		"<summary>",
		"User Request",
		"Current Goal",
		"Important Context",
		"Decisions",
		"Progress",
		"Open Tasks",
		"Files and Symbols",
		"Verification",
		"Risks",
		"please remember this exact user request",
	} {
		if !strings.Contains(contract, want) {
			t.Fatalf("summary contract missing %q in:\n%s", want, contract)
		}
	}
	if len(req.Messages) != 2 || req.Messages[1].Role != provider.RoleUser || strings.Contains(messagesText(req.Messages[1:]), "<summary>") {
		t.Fatalf("internal control message=%+v", req.Messages)
	}
	if !strings.Contains(req.Prompt.StableSystem, "last user message is an internal compression control instruction") {
		t.Fatalf("system prompt missing exclusion rule: %s", req.Prompt.StableSystem)
	}
}

func TestExtractSummarySelectsLastCompleteNonEmptyBlock(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "one", input: "<thinking>draft</thinking><summary>\n  keep me \n</summary>", want: "keep me"},
		{name: "two selects last", input: "<summary>draft</summary>outside<summary>final</summary>", want: "final"},
		{name: "outside text discarded", input: "prefix<summary>final</summary>suffix", want: "final"},
		{name: "missing", input: "plain text", wantErr: true},
		{name: "empty", input: "<summary>   </summary>", wantErr: true},
		{name: "ambiguous nested", input: "<summary>a<summary>b</summary></summary>", wantErr: true},
		{name: "unclosed", input: "<summary>a", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractSummary(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ExtractSummary(%q)=%+v, want error", tc.input, got)
				}
				return
			}
			if err != nil || got.Text != tc.want {
				t.Fatalf("summary=%+v err=%v, want %q", got, err, tc.want)
			}
			if got.CandidateCount == 0 || got.UsedLast != (got.CandidateCount > 1) {
				t.Fatalf("summary metadata=%+v", got)
			}
		})
	}
}

func TestExtractSummaryAcceptsPureNestedWrapper(t *testing.T) {
	got, err := ExtractSummary("<summary><summary>final state</summary></summary>")
	if err != nil || got.Text != "final state" {
		t.Fatalf("summary=%+v err=%v", got, err)
	}
}

func TestExtractSummaryReportsAmbiguousNestedWrapper(t *testing.T) {
	_, err := ExtractSummary("<summary>draft<summary>final</summary></summary>")
	var parseErr *SummaryParseError
	if !errors.As(err, &parseErr) || parseErr.Reason != "summary wrapper contains content" || parseErr.MaxDepth != 2 {
		t.Fatalf("err=%v parseErr=%+v", err, parseErr)
	}
}

func TestRebuildKeepsSummaryBoundaryAndRecentMessages(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RecentTokens = 4
	cfg.RecentMessageMinimum = 5
	m := NewManager(cfg, nil)
	messages := []provider.Message{
		textMessage(provider.RoleUser, "old request that must be summarized or retained"),
		textMessage(provider.RoleAssistant, "old answer"),
		textMessage(provider.RoleUser, "middle one"),
		textMessage(provider.RoleAssistant, "middle two"),
		textMessage(provider.RoleUser, "recent one"),
		textMessage(provider.RoleAssistant, "recent two"),
		textMessage(provider.RoleUser, "recent three"),
		textMessage(provider.RoleAssistant, "recent four"),
	}

	rebuilt := m.Rebuild(messages, "compressed state")
	if len(rebuilt) != 6 {
		t.Fatalf("rebuilt len=%d messages=%+v", len(rebuilt), rebuilt)
	}
	if got := rebuilt[0].Blocks[0].Text; !strings.Contains(got, "Previous conversation summary") || !strings.Contains(got, "compressed state") || !strings.Contains(got, "re-read") {
		t.Fatalf("boundary=%q", got)
	}
	if got := messagesText(rebuilt[1:]); !strings.Contains(got, "middle two") || !strings.Contains(got, "recent four") {
		t.Fatalf("recent messages not retained:\n%s", got)
	}
}

func TestRebuildDoesNotSplitToolCallAndResultPair(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RecentTokens = 1
	cfg.RecentMessageMinimum = 1
	m := NewManager(cfg, nil)
	call := provider.ToolCall{ID: "call-1", Name: "read_file", Arguments: []byte(`{"path":"README.md"}`)}
	messages := []provider.Message{
		textMessage(provider.RoleUser, "older"),
		{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockToolCall, ToolCall: &call}}},
		{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockToolResult, ToolResult: &provider.ToolResult{CallID: "call-1", Name: "read_file", Content: "result"}}}},
	}

	rebuilt := m.Rebuild(messages, "summary")
	if len(rebuilt) != 3 {
		t.Fatalf("rebuilt len=%d messages=%+v", len(rebuilt), rebuilt)
	}
	if rebuilt[1].Blocks[0].ToolCall == nil || rebuilt[2].Blocks[0].ToolResult == nil {
		t.Fatalf("tool pair was split or changed: %+v", rebuilt)
	}
}

func textMessage(role provider.Role, text string) provider.Message {
	return provider.Message{Role: role, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: text}}}
}

func messagesText(messages []provider.Message) string {
	var b strings.Builder
	for _, message := range messages {
		for _, block := range message.Blocks {
			b.WriteString(block.Text)
			if block.ToolCall != nil {
				b.WriteString(block.ToolCall.ID)
				b.WriteString(block.ToolCall.Name)
				b.Write(block.ToolCall.Arguments)
			}
			if block.ToolResult != nil {
				b.WriteString(block.ToolResult.CallID)
				b.WriteString(block.ToolResult.Name)
				b.WriteString(block.ToolResult.Content)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}
