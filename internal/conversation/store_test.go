package conversation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func TestSessionStoreCreateUsesUniqueTimestampIDsAndJournal(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 13, 9, 10, 11, 0, time.UTC)
	store := NewSessionStore(dir)
	store.now = func() time.Time { return now }

	first, firstMeta, err := store.Create()
	if err != nil {
		t.Fatal(err)
	}
	second, secondMeta, err := store.Create()
	if err != nil {
		t.Fatal(err)
	}
	if second == nil {
		t.Fatal("second session is nil")
	}
	pattern := regexp.MustCompile(`^20260813-091011-[0-9a-f]{4}$`)
	if !pattern.MatchString(firstMeta.ID) || !pattern.MatchString(secondMeta.ID) {
		t.Fatalf("unexpected IDs: %q, %q", firstMeta.ID, secondMeta.ID)
	}
	if firstMeta.ID == secondMeta.ID {
		t.Fatal("same-second sessions share an ID")
	}
	if !firstMeta.CreatedAt.Equal(now) || !firstMeta.LastActiveAt.Equal(now) {
		t.Fatalf("first metadata=%+v", firstMeta)
	}

	user := provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "remember this"}}}
	assistant := provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "recorded"}}}
	if err := first.CommitRound(&user, assistant, nil); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, firstMeta.ID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(strings.TrimSpace(string(content)), "\n") + 1; got != 2 {
		t.Fatalf("journal records=%d, want 2", got)
	}
}

func TestSessionStoreListScansValidatedJSONLMetadata(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	created := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	writeSessionRecords(t, dir, "20260810-080000-a1b2", []JournalRecord{
		journalText(provider.RoleUser, JournalPurposeHistory, "first request", created),
		journalText(provider.RoleAssistant, JournalPurposeHistory, "answer", created.Add(2*time.Minute)),
	})
	writeSessionRecords(t, dir, "20260811-080000-c3d4", []JournalRecord{
		journalText(provider.RoleUser, JournalPurposePlan, "/plan ship it", created.Add(24*time.Hour)),
		journalText(provider.RoleAssistant, JournalPurposePlan, "ship it", created.Add(24*time.Hour+time.Minute)),
	})
	if err := os.WriteFile(filepath.Join(dir, "not-a-session.jsonl"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20260812-080000-dead.txt"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	metas, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("metas=%+v", metas)
	}
	if metas[0].ID != "20260811-080000-c3d4" || metas[0].Title != "/plan ship it" || metas[0].MessageCount != 2 {
		t.Fatalf("newest meta=%+v", metas[0])
	}
	if metas[1].ID != "20260810-080000-a1b2" || metas[1].Title != "first request" || !metas[1].CreatedAt.Equal(created) || !metas[1].LastActiveAt.Equal(created.Add(2*time.Minute)) || metas[1].MessageCount != 2 {
		t.Fatalf("older meta=%+v", metas[1])
	}
}

func TestSessionStoreRestoreSkipsMalformedLinesAndTruncatesUnpairedCalls(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	store.now = func() time.Time { return time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC) }
	id := "20260813-080000-a1b2"
	path := filepath.Join(dir, id+".jsonl")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	completeCall := ToolUseRecord{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}
	completeResult := ToolResultRecord{CallID: "call-1", Name: "read_file", Content: "contents"}
	records := []JournalRecord{
		journalText(provider.RoleUser, JournalPurposeHistory, "inspect", time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)),
		{Role: provider.RoleAssistant, Content: "reading", ToolUses: []ToolUseRecord{completeCall}, Purpose: JournalPurposeHistory, Timestamp: time.Date(2026, 8, 13, 8, 1, 0, 0, time.UTC).Unix()},
		{Role: provider.RoleUser, ToolResults: []ToolResultRecord{completeResult}, Purpose: JournalPurposeHistory, Timestamp: time.Date(2026, 8, 13, 8, 2, 0, 0, time.UTC).Unix()},
		{Role: provider.RoleUser, Content: "after result", Purpose: JournalPurposeHistory, Timestamp: time.Date(2026, 8, 13, 8, 3, 0, 0, time.UTC).Unix()},
		{Role: provider.RoleAssistant, Content: "unfinished", ToolUses: []ToolUseRecord{{ID: "call-2", Name: "write_file", Arguments: json.RawMessage(`{"path":"x"}`)}}, Purpose: JournalPurposeHistory, Timestamp: time.Date(2026, 8, 13, 8, 4, 0, 0, time.UTC).Unix()},
		journalText(provider.RoleUser, JournalPurposeHistory, "must be discarded", time.Date(2026, 8, 13, 8, 5, 0, 0, time.UTC)),
	}
	writeSessionRecords(t, dir, id, records[:2])
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{broken json}\n"); err != nil {
		t.Fatal(err)
	}
	for _, record := range records[2:] {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(append(encoded, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	session, meta, err := store.Restore(id)
	if err != nil {
		t.Fatal(err)
	}
	if meta.MessageCount != len(records) || meta.Title != "inspect" {
		t.Fatalf("meta=%+v", meta)
	}
	history := session.Snapshot()
	if len(history) != 4 {
		t.Fatalf("history=%+v", history)
	}
	if history[0].Blocks[0].Text != "inspect" || history[3].Blocks[0].Text != "after result" {
		t.Fatalf("unexpected recovered history=%+v", history)
	}
	if history[1].Blocks[1].ToolCall.ID != "call-1" || history[2].Blocks[0].ToolResult.CallID != "call-1" {
		t.Fatalf("tool pair not restored=%+v", history)
	}
}

func TestSessionStoreRestoreRestoresPlansAndAddsTimeGapReminder(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	lastActive := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return lastActive.Add(24*time.Hour + time.Second) }
	id := "20260810-070000-a1b2"
	writeSessionRecords(t, dir, id, []JournalRecord{
		journalText(provider.RoleUser, JournalPurposePlan, "/plan make it safe", lastActive.Add(-time.Minute)),
		journalText(provider.RoleAssistant, JournalPurposePlan, "make it safe", lastActive),
	})

	session, _, err := store.Restore(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Snapshot()) != 1 {
		t.Fatalf("history=%+v", session.Snapshot())
	}
	if got := session.PendingPlans(); len(got) != 1 || got[0] != "make it safe" {
		t.Fatalf("plans=%q", got)
	}
	display := session.DisplaySnapshot()
	if len(display) != 3 || !strings.Contains(display[2].Blocks[0].Text, "More than 24 hours") {
		t.Fatalf("display=%+v", display)
	}
}

func TestSessionStoreDeleteAndCleanupExpiredOnlyTouchValidSessionFiles(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	writeSessionRecords(t, dir, "20260714-115959-a1b2", []JournalRecord{journalText(provider.RoleUser, JournalPurposeHistory, "old", now.Add(-30*24*time.Hour-time.Second))})
	writeSessionRecords(t, dir, "20260714-120000-c3d4", []JournalRecord{journalText(provider.RoleUser, JournalPurposeHistory, "boundary", now.Add(-30*24*time.Hour))})
	writeSessionRecords(t, dir, "20260813-120000-e5f6", []JournalRecord{journalText(provider.RoleUser, JournalPurposeHistory, "new", now)})
	invalid := filepath.Join(dir, "outside.jsonl")
	if err := os.WriteFile(invalid, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	removed, err := store.CleanupExpired(now)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d", removed)
	}
	for _, id := range []string{"20260714-120000-c3d4", "20260813-120000-e5f6"} {
		if _, err := os.Stat(filepath.Join(dir, id+".jsonl")); err != nil {
			t.Fatalf("%s should remain: %v", id, err)
		}
	}
	if content, err := os.ReadFile(invalid); err != nil || string(content) != "keep" {
		t.Fatalf("invalid file changed: content=%q err=%v", content, err)
	}
	if err := store.Delete("../../outside"); err == nil {
		t.Fatal("unsafe ID was accepted")
	}
	if err := store.Delete("20260813-120000-e5f6"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "20260813-120000-e5f6.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("valid session not deleted: %v", err)
	}
}

func journalText(role provider.Role, purpose JournalPurpose, content string, timestamp time.Time) JournalRecord {
	return JournalRecord{Role: role, Content: content, Purpose: purpose, Timestamp: timestamp.Unix()}
}

func writeSessionRecords(t *testing.T, dir, id string, records []JournalRecord) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(dir, id+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(append(encoded, '\n')); err != nil {
			t.Fatal(err)
		}
	}
}
