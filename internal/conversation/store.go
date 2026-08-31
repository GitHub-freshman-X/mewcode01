package conversation

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

const (
	sessionFileMode = 0o600
	sessionDirMode  = 0o700
	sessionMaxAge   = 30 * 24 * time.Hour
	sessionGap      = 24 * time.Hour
)

var sessionIDPattern = regexp.MustCompile(`^\d{8}-\d{6}-[0-9a-f]{4}$`)

type SessionMeta struct {
	ID           string
	Title        string
	CreatedAt    time.Time
	LastActiveAt time.Time
	MessageCount int
}

type SessionStore struct {
	sessionsDir string
	contextDir  string
	now         func() time.Time
	Logger      *logging.Logger
	removeAll   func(string) error
	removeFile  func(string) error
}

func NewSessionStore(sessionsDir string) *SessionStore {
	sessions := filepath.Clean(sessionsDir)
	return &SessionStore{
		sessionsDir: sessions,
		contextDir:  filepath.Join(filepath.Dir(sessions), "context"),
		now:         time.Now,
		Logger:      logging.Nop(),
		removeAll:   os.RemoveAll,
		removeFile:  os.Remove,
	}
}

func (s *SessionStore) Create() (*Session, SessionMeta, error) {
	if s == nil {
		return nil, SessionMeta{}, errors.New("session store is nil")
	}
	if err := os.MkdirAll(s.sessionsDir, sessionDirMode); err != nil {
		return nil, SessionMeta{}, err
	}
	created := s.currentTime().UTC()
	for attempts := 0; attempts < 256; attempts++ {
		id, err := newSessionID(created)
		if err != nil {
			return nil, SessionMeta{}, err
		}
		file, err := os.OpenFile(s.pathForID(id), os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_APPEND, sessionFileMode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, SessionMeta{}, err
		}
		journal := NewJSONLJournal(file)
		journal.now = s.now
		return NewSession(journal), SessionMeta{ID: id, CreatedAt: created, LastActiveAt: created}, nil
	}
	return nil, SessionMeta{}, errors.New("could not allocate a unique session ID")
}

func (s *SessionStore) List() ([]SessionMeta, error) {
	if s == nil {
		return nil, errors.New("session store is nil")
	}
	entries, err := os.ReadDir(s.sessionsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	metas := make([]SessionMeta, 0, len(entries))
	for _, entry := range entries {
		id, ok := sessionIDFromFilename(entry.Name())
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		meta, err := s.metadataFor(id)
		if err != nil {
			return nil, err
		}
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool {
		if metas[i].LastActiveAt.Equal(metas[j].LastActiveAt) {
			return metas[i].ID > metas[j].ID
		}
		return metas[i].LastActiveAt.After(metas[j].LastActiveAt)
	})
	return metas, nil
}

func (s *SessionStore) Restore(id string) (*Session, SessionMeta, error) {
	if s == nil {
		return nil, SessionMeta{}, errors.New("session store is nil")
	}
	if !validSessionID(id) {
		return nil, SessionMeta{}, errors.New("session ID is invalid")
	}
	info, err := os.Lstat(s.pathForID(id))
	if err != nil {
		return nil, SessionMeta{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, SessionMeta{}, errors.New("session file is not regular")
	}
	records, err := readJournalRecords(s.pathForID(id))
	if err != nil {
		return nil, SessionMeta{}, err
	}
	meta := sessionMetadata(id, records)
	journalFile, err := os.OpenFile(s.pathForID(id), os.O_WRONLY|os.O_APPEND, sessionFileMode)
	if err != nil {
		return nil, SessionMeta{}, err
	}
	journal := NewJSONLJournal(journalFile)
	journal.now = s.now
	session := NewSession(journal)
	history, display, plans := restoreRecords(records)
	if s.currentTime().Sub(meta.LastActiveAt) > sessionGap {
		reminder := timeGapReminder(meta.LastActiveAt)
		history = append(history, reminder)
		display = append(display, provider.CloneMessage(reminder))
	}
	session.history = history
	session.display = display
	session.pendingPlans = plans
	session.usage = sessionUsage(records)
	return session, meta, nil
}

func (s *SessionStore) Delete(id string) error {
	if s == nil {
		return errors.New("session store is nil")
	}
	if !validSessionID(id) {
		return errors.New("session ID is invalid")
	}
	info, err := os.Lstat(s.pathForID(id))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("session file is not regular")
	}
	return s.removeSessionFile(s.pathForID(id))
}

func (s *SessionStore) CleanupExpired(now time.Time) (int, error) {
	metas, err := s.List()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, meta := range metas {
		if now.Sub(meta.LastActiveAt) <= sessionMaxAge {
			continue
		}
		if err := s.removeExpiredContext(meta.ID); err != nil {
			s.logCleanup("failed", logging.Fields{"count": removed, "error_type": "context_remove"})
			return removed, err
		}
		if err := s.Delete(meta.ID); err != nil {
			s.logCleanup("failed", logging.Fields{"count": removed, "error_type": "session_remove"})
			return removed, err
		}
		removed++
		s.logCleanup("completed", logging.Fields{"count": 1})
	}
	return removed, nil
}

func (s *SessionStore) removeExpiredContext(id string) error {
	path, err := s.contextPath(id)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		s.logCleanup("context_not_found", logging.Fields{"count": 1})
		return nil
	} else if err != nil {
		return err
	}
	if err := s.removeContextDirectory(path); err != nil {
		return err
	}
	s.logCleanup("context_removed", logging.Fields{"count": 1})
	return nil
}

func (s *SessionStore) contextPath(id string) (string, error) {
	if !validSessionID(id) {
		return "", errors.New("session ID is invalid")
	}
	root := filepath.Clean(s.contextDir)
	path := filepath.Join(root, id)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel != id {
		return "", errors.New("context directory is outside session storage")
	}
	return path, nil
}

func (s *SessionStore) removeContextDirectory(path string) error {
	if s.removeAll == nil {
		return os.RemoveAll(path)
	}
	return s.removeAll(path)
}

func (s *SessionStore) removeSessionFile(path string) error {
	if s.removeFile == nil {
		return os.Remove(path)
	}
	return s.removeFile(path)
}

func (s *SessionStore) logCleanup(status string, fields logging.Fields) {
	if s == nil || s.Logger == nil {
		return
	}
	s.Logger.Info("session cleanup", appendCleanupFields(status, fields))
}

func appendCleanupFields(status string, fields logging.Fields) logging.Fields {
	result := logging.Fields{"stage": "session_cleanup", "status": status}
	for key, value := range fields {
		result[key] = value
	}
	return result
}

func (s *SessionStore) currentTime() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

func (s *SessionStore) pathForID(id string) string {
	return filepath.Join(s.sessionsDir, id+".jsonl")
}

func newSessionID(created time.Time) (string, error) {
	suffix := make([]byte, 2)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	return created.Format("20060102-150405") + "-" + hex.EncodeToString(suffix), nil
}

func validSessionID(id string) bool {
	if !sessionIDPattern.MatchString(id) {
		return false
	}
	_, err := time.ParseInLocation("20060102-150405", id[:15], time.UTC)
	return err == nil
}

func sessionIDFromFilename(name string) (string, bool) {
	if filepath.Ext(name) != ".jsonl" {
		return "", false
	}
	id := strings.TrimSuffix(name, ".jsonl")
	return id, validSessionID(id)
}

func sessionMetadata(id string, records []JournalRecord) SessionMeta {
	created, _ := time.ParseInLocation("20060102-150405", id[:15], time.UTC)
	meta := SessionMeta{ID: id, CreatedAt: created, LastActiveAt: created}
	for _, record := range records {
		if record.Purpose != JournalPurposeUsage && meta.Title == "" && record.Role == provider.RoleUser && strings.TrimSpace(record.Content) != "" {
			meta.Title = strings.TrimSpace(record.Content)
		}
		active := time.Unix(record.Timestamp, 0).UTC()
		if active.After(meta.LastActiveAt) {
			meta.LastActiveAt = active
		}
		if record.Purpose != JournalPurposeUsage {
			meta.MessageCount++
		}
	}
	return meta
}

func readJournalRecords(path string) ([]JournalRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var records []JournalRecord
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var record JournalRecord
			if json.Unmarshal(bytes.TrimSpace(line), &record) == nil && validJournalRecord(record) {
				records = append(records, record)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return records, nil
}

func validJournalRecord(record JournalRecord) bool {
	if record.Purpose == JournalPurposeUsage {
		return record.Role == "" && record.Usage != nil && record.Usage.InputTokens >= 0 && record.Usage.OutputTokens >= 0 && record.Usage.CacheReadInputTokens >= 0 && record.Usage.CacheCreationInputTokens >= 0 && (record.Usage.CacheTokensIncludedInInput == nil || *record.Usage.CacheTokensIncludedInInput >= 0) && record.Timestamp > 0
	}
	if (record.Role != provider.RoleUser && record.Role != provider.RoleAssistant) || (record.Purpose != JournalPurposeHistory && record.Purpose != JournalPurposePlan) || record.Timestamp <= 0 {
		return false
	}
	if len(record.ToolUses) > 0 && (record.Role != provider.RoleAssistant || len(record.ToolResults) > 0) {
		return false
	}
	if len(record.ToolResults) > 0 && record.Role != provider.RoleUser {
		return false
	}
	seen := make(map[string]struct{}, len(record.ToolUses)+len(record.ToolResults))
	for _, call := range record.ToolUses {
		if call.ID == "" || call.Name == "" {
			return false
		}
		if _, ok := seen[call.ID]; ok {
			return false
		}
		seen[call.ID] = struct{}{}
	}
	for _, result := range record.ToolResults {
		if result.CallID == "" || result.Name == "" {
			return false
		}
		if _, ok := seen[result.CallID]; ok {
			return false
		}
		seen[result.CallID] = struct{}{}
	}
	return true
}

func (s *SessionStore) metadataFor(id string) (SessionMeta, error) {
	records, err := readJournalRecords(s.pathForID(id))
	if err != nil {
		return SessionMeta{}, err
	}
	return sessionMetadata(id, records), nil
}

type pendingToolCall struct {
	name         string
	historyIndex int
	displayIndex int
}

func restoreRecords(records []JournalRecord) ([]provider.Message, []provider.Message, []string) {
	var history []provider.Message
	var display []provider.Message
	var plans []string
	pending := make(map[string]pendingToolCall)

	truncate := func() {
		firstHistory := len(history)
		firstDisplay := len(display)
		for _, call := range pending {
			if call.historyIndex < firstHistory {
				firstHistory = call.historyIndex
			}
			if call.displayIndex < firstDisplay {
				firstDisplay = call.displayIndex
			}
		}
		history = history[:firstHistory]
		display = display[:firstDisplay]
	}

	for _, record := range records {
		if record.Purpose == JournalPurposeUsage {
			continue
		}
		if len(pending) > 0 && (record.Purpose != JournalPurposeHistory || len(record.ToolResults) == 0) {
			truncate()
			break
		}
		message := recordMessage(record)
		if record.Purpose == JournalPurposePlan {
			display = append(display, message)
			if record.Role == provider.RoleAssistant && strings.TrimSpace(record.Content) != "" {
				plans = append(plans, strings.TrimSpace(record.Content))
			}
			continue
		}

		if len(record.ToolUses) > 0 && len(pending) > 0 {
			truncate()
			break
		}
		if len(record.ToolResults) > 0 {
			matched := true
			for _, result := range record.ToolResults {
				call, ok := pending[result.CallID]
				if !ok || call.name != result.Name {
					matched = false
					break
				}
			}
			if !matched {
				if len(pending) > 0 {
					truncate()
					break
				}
				continue
			}
		}

		history = append(history, message)
		display = append(display, provider.CloneMessage(message))
		for _, call := range record.ToolUses {
			pending[call.ID] = pendingToolCall{name: call.Name, historyIndex: len(history) - 1, displayIndex: len(display) - 1}
		}
		for _, result := range record.ToolResults {
			delete(pending, result.CallID)
		}
	}
	if len(pending) > 0 {
		truncate()
	}
	return history, display, plans
}

func sessionUsage(records []JournalRecord) provider.Usage {
	var usage provider.Usage
	for _, record := range records {
		if record.Purpose == JournalPurposeUsage && record.Usage != nil {
			included := 0
			if record.Usage.CacheTokensIncludedInInput != nil {
				included = *record.Usage.CacheTokensIncludedInInput
			}
			usage.Add(provider.Usage{InputTokens: record.Usage.InputTokens, OutputTokens: record.Usage.OutputTokens, CacheReadInputTokens: record.Usage.CacheReadInputTokens, CacheCreationInputTokens: record.Usage.CacheCreationInputTokens, CacheTokensIncludedInInput: included, CacheUnavailable: record.Usage.CacheUsageKnown == nil || !*record.Usage.CacheUsageKnown || record.Usage.CacheTokensIncludedInInput == nil})
		}
	}
	return usage
}

func recordMessage(record JournalRecord) provider.Message {
	blocks := make([]provider.ContentBlock, 0, 1+len(record.ToolUses)+len(record.ToolResults))
	if record.Content != "" {
		blocks = append(blocks, provider.ContentBlock{Type: provider.BlockText, Text: record.Content})
	}
	for _, call := range record.ToolUses {
		blocks = append(blocks, provider.ContentBlock{Type: provider.BlockToolCall, ToolCall: &provider.ToolCall{ID: call.ID, Name: call.Name, Arguments: append([]byte(nil), call.Arguments...)}})
	}
	for _, result := range record.ToolResults {
		blocks = append(blocks, provider.ContentBlock{Type: provider.BlockToolResult, ToolResult: &provider.ToolResult{CallID: result.CallID, Name: result.Name, Content: result.Content, IsError: result.IsError}})
	}
	return provider.Message{Role: record.Role, Blocks: blocks}
}

func timeGapReminder(lastActive time.Time) provider.Message {
	return provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{
		Type: provider.BlockText,
		Text: fmt.Sprintf("Time gap notice: the previous session was last active at %s. More than 24 hours have passed; project files may have changed. Re-read relevant files before continuing.", lastActive.UTC().Format(time.RFC3339)),
	}}}
}
