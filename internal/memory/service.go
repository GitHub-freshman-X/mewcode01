package memory

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/logging"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type Mode string

const (
	ModeAct  Mode = "act"
	ModePlan Mode = "plan"
	ModeDo   Mode = "do"
)

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type ModelCaller interface {
	Call(context.Context, provider.ChatRequest) (string, error)
}

type SessionLister interface {
	List() ([]SessionMeta, error)
}

type SessionMeta struct{}

type Logger interface {
	Info(string, logging.Fields)
	Error(string, logging.Fields)
}

type ServiceOptions struct {
	Caller   ModelCaller
	Clock    Clock
	Sessions SessionLister
	Logger   Logger
}

type Service struct {
	paths    Paths
	caller   ModelCaller
	clock    Clock
	sessions SessionLister
	logger   Logger

	mu       sync.Mutex
	lastScan map[string]time.Time
	recent   map[[32]byte]struct{}
	status   StatusSummary
}

const (
	consolidationInterval    = 24 * time.Hour
	consolidationScanDelay   = 10 * time.Minute
	consolidationLockExpiry  = time.Hour
	consolidationMinSessions = 5
	consolidationLockName    = ".consolidate-lock"
	consolidationMarkerName  = ".consolidate-last-success"
)

func NewService(paths Paths, options ServiceOptions) *Service {
	clock := options.Clock
	if clock == nil {
		clock = systemClock{}
	}
	logger := options.Logger
	if logger == nil {
		logger = logging.Nop()
	}
	service := &Service{paths: paths, caller: options.Caller, clock: clock, sessions: options.Sessions, logger: logger, lastScan: make(map[string]time.Time), recent: make(map[[32]byte]struct{})}
	service.refreshStatus()
	return service
}

type StatusSummary struct {
	UserCount, ProjectCount int
	Available               bool
}

type CommandSummary struct{ UserCount, ProjectCount int }
type CommandItem struct {
	Kind              MemoryKind
	Name, Description string
}

// StatusSummary returns cached counts without filesystem access.
func (s *Service) StatusSummary() StatusSummary {
	if s == nil {
		return StatusSummary{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *Service) refreshStatus() {
	if s == nil {
		return
	}
	summary, err := countStatus(s.paths)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.status = StatusSummary{}
		return
	}
	s.status = summary
}

func countStatus(paths Paths) (StatusSummary, error) {
	var summary StatusSummary
	for _, item := range []struct {
		directory string
		project   bool
	}{{paths.UserMemory, false}, {paths.ProjectMemory, true}} {
		entries, err := os.ReadDir(item.directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return StatusSummary{}, err
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == "MEMORY.md" || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			if item.project {
				summary.ProjectCount++
			} else {
				summary.UserCount++
			}
		}
	}
	summary.Available = true
	return summary, nil
}

func (s *Service) CommandSummary() (CommandSummary, error) {
	items, err := s.CommandList()
	if err != nil {
		return CommandSummary{}, err
	}
	var summary CommandSummary
	for _, item := range items {
		if item.Kind == MemoryUser || item.Kind == MemoryFeedback {
			summary.UserCount++
		} else {
			summary.ProjectCount++
		}
	}
	return summary, nil
}

func (s *Service) CommandList() ([]CommandItem, error) {
	if s == nil {
		return nil, errors.New("memory service is nil")
	}
	var items []CommandItem
	for _, kind := range []MemoryKind{MemoryUser, MemoryProject} {
		directory, err := MemoryDirectory(s.paths, kind)
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == "MEMORY.md" || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".md")
			content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
			if err != nil {
				return nil, err
			}
			description := ""
			for _, line := range strings.Split(string(content), "\n") {
				if strings.HasPrefix(line, "description: ") {
					description = strings.Trim(strings.TrimPrefix(line, "description: "), "\"")
					break
				}
			}
			items = append(items, CommandItem{Kind: kind, Name: name, Description: description})
		}
	}
	return items, nil
}

func (s *Service) CommandAdd(kind, content string) error {
	kind = strings.ToLower(strings.TrimSpace(kind))
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("memory content is empty")
	}
	var memoryKind MemoryKind
	switch kind {
	case "user":
		memoryKind = MemoryUser
	case "project":
		memoryKind = MemoryProject
	default:
		return errors.New("memory category must be user or project")
	}
	key := sha256.Sum256([]byte(content))
	name := fmt.Sprintf("manual-%x", key[:6])
	if err := ApplyOperations(s.paths, []MemoryOperation{{Action: "create", Kind: memoryKind, Name: name, Description: "Manual memory", Content: content}}); err != nil {
		return err
	}
	s.refreshStatus()
	return nil
}

func (s *Service) CommandClear() error {
	if s == nil {
		return errors.New("memory service is nil")
	}
	for _, kind := range []MemoryKind{MemoryUser, MemoryProject} {
		directory, err := MemoryDirectory(s.paths, kind)
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
				if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
					return err
				}
			}
		}
	}
	s.refreshStatus()
	return nil
}

func (s *Service) ShouldExtract(transcript []provider.Message) bool {
	for i := len(transcript) - 1; i >= 0; i-- {
		if transcript[i].Role != provider.RoleUser {
			continue
		}
		for _, block := range transcript[i].Blocks {
			if block.Type != provider.BlockText || !isDurableMemoryCandidate(block.Text) {
				continue
			}
			key := sha256.Sum256([]byte(strings.TrimSpace(block.Text)))
			s.mu.Lock()
			_, seen := s.recent[key]
			if !seen {
				s.recent[key] = struct{}{}
			}
			s.mu.Unlock()
			return !seen
		}
		return false
	}
	return false
}

func isDurableMemoryCandidate(text string) bool {
	text = strings.ToLower(text)
	for _, marker := range []string{"请记住", "记住", "长期偏好", "偏好", "习惯", "以后", "始终", "不要", "规则", "preference", "remember", "always", "never"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func (s *Service) Extract(ctx context.Context, mode Mode, transcript []provider.Message) error {
	if s == nil || s.caller == nil {
		return errors.New("memory service caller is not configured")
	}
	started := s.clock.Now()
	indexes, err := LoadIndexes(s.paths)
	if err != nil {
		return err
	}
	response, err := s.caller.Call(ctx, extractionRequest(mode, transcript, indexes))
	if err != nil {
		s.logger.Error("memory extraction failed", logging.Fields{"stage": "memory_extract", "status": "provider_failed", "duration_ms": s.clock.Now().Sub(started).Milliseconds()})
		return err
	}
	operations, err := ParseOperations([]byte(response))
	if err != nil {
		s.logger.Error("memory extraction failed", logging.Fields{"stage": "memory_extract", "status": "response_invalid", "response_bytes": len(response), "duration_ms": s.clock.Now().Sub(started).Milliseconds()})
		return err
	}
	if err := ApplyOperations(s.paths, operations); err != nil {
		s.logger.Error("memory extraction failed", logging.Fields{"stage": "memory_extract", "status": "operation_failed", "operation_count": len(operations), "duration_ms": s.clock.Now().Sub(started).Milliseconds()})
		return err
	}
	s.refreshStatus()
	s.logger.Info("memory extraction completed", logging.Fields{"stage": "memory_extract", "status": "completed", "mode": string(mode), "operation_count": len(operations), "duration_ms": s.clock.Now().Sub(started).Milliseconds()})
	return nil
}

func extractionRequest(mode Mode, transcript []provider.Message, indexes []string) provider.ChatRequest {
	messages := provider.CloneMessages(transcript)
	messages = append(messages, provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "Extract durable memories for mode " + string(mode) + ". Existing memory indexes follow. If a candidate is semantically equivalent to an existing entry, update that entry; never create a duplicate. Return only a JSON array of constrained memory operations.\n\nUser index:\n" + indexes[0] + "\n\nProject index:\n" + indexes[1]}}})
	return provider.ChatRequest{
		Prompt:    provider.PromptBundle{StableSystem: memoryOperationProtocol("extract durable memory from a completed conversation")},
		Messages:  messages,
		MaxTokens: 2048,
	}
}

func (s *Service) MaybeConsolidate(ctx context.Context) error {
	if s == nil || s.caller == nil || s.sessions == nil {
		return errors.New("memory consolidation dependencies are not configured")
	}
	now := s.clock.Now()
	for _, directory := range []string{s.paths.UserMemory, s.paths.ProjectMemory} {
		if err := s.consolidateDirectory(ctx, directory, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) consolidateDirectory(ctx context.Context, directory string, now time.Time) error {
	info, err := os.Stat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("memory path is not a directory")
	}
	if recentlyConsolidated(directory, now) {
		return nil
	}
	if !s.scanAllowed(directory, now) {
		return nil
	}
	metas, err := s.sessions.List()
	if err != nil {
		return err
	}
	if len(metas) < consolidationMinSessions {
		return nil
	}
	release, acquired, err := acquireConsolidationLock(directory, now)
	if err != nil || !acquired {
		return err
	}
	defer release()

	started := s.clock.Now()
	response, err := s.caller.Call(ctx, consolidationRequest(directory))
	if err != nil {
		s.logger.Error("memory consolidation failed", logging.Fields{"stage": "memory_consolidation", "status": "provider_failed", "duration_ms": s.clock.Now().Sub(started).Milliseconds()})
		return err
	}
	operations, err := ParseOperations([]byte(response))
	if err != nil {
		s.logger.Error("memory consolidation failed", logging.Fields{"stage": "memory_consolidation", "status": "response_invalid", "response_bytes": len(response), "duration_ms": s.clock.Now().Sub(started).Milliseconds()})
		return err
	}
	for _, operation := range operations {
		if operation.Action == "noop" {
			continue
		}
		target, err := MemoryDirectory(s.paths, operation.Kind)
		if err != nil || filepath.Clean(target) != filepath.Clean(directory) {
			return errors.New("consolidation operation is outside target memory directory")
		}
	}
	if err := ApplyOperations(s.paths, operations); err != nil {
		s.logger.Error("memory consolidation failed", logging.Fields{"stage": "memory_consolidation", "status": "operation_failed", "operation_count": len(operations), "duration_ms": s.clock.Now().Sub(started).Milliseconds()})
		return err
	}
	s.refreshStatus()
	if err := atomicWrite(filepath.Join(directory, consolidationMarkerName), []byte(strconv.FormatInt(now.Unix(), 10)+"\n"), 0o600); err != nil {
		return err
	}
	s.logger.Info("memory consolidation completed", logging.Fields{"stage": "memory_consolidation", "status": "completed", "operation_count": len(operations), "duration_ms": s.clock.Now().Sub(started).Milliseconds()})
	return nil
}

func (s *Service) scanAllowed(directory string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.lastScan[directory]; ok && now.Sub(previous) < consolidationScanDelay {
		return false
	}
	s.lastScan[directory] = now
	return true
}

func recentlyConsolidated(directory string, now time.Time) bool {
	info, err := os.Stat(filepath.Join(directory, consolidationMarkerName))
	return err == nil && now.Sub(info.ModTime()) < consolidationInterval
}

func acquireConsolidationLock(directory string, now time.Time) (func(), bool, error) {
	path := filepath.Join(directory, consolidationLockName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, writeErr := fmt.Fprintln(file, os.Getpid()); writeErr != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return nil, false, writeErr
		}
		if closeErr := file.Close(); closeErr != nil {
			_ = os.Remove(path)
			return nil, false, closeErr
		}
		return func() { _ = os.Remove(path) }, true, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, false, err
	}
	info, statErr := os.Lstat(path)
	if statErr != nil {
		return nil, false, statErr
	}
	if !info.Mode().IsRegular() || now.Sub(info.ModTime()) < consolidationLockExpiry {
		return nil, false, nil
	}
	if err := os.Remove(path); err != nil {
		return nil, false, err
	}
	return acquireConsolidationLock(directory, now)
}

func consolidationRequest(directory string) provider.ChatRequest {
	index, _ := os.ReadFile(filepath.Join(directory, "MEMORY.md"))
	return provider.ChatRequest{
		Prompt:    provider.PromptBundle{StableSystem: memoryOperationProtocol("consolidate durable memories in one directory")},
		Messages:  []provider.Message{{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: "Consolidate this memory index:\n" + string(index)}}}},
		MaxTokens: 2048,
	}
}

func memoryOperationProtocol(task string) string {
	return task + `. Do not call tools. Return exactly one JSON array and no Markdown or explanatory text.
Each item has action, kind, name, description, and content fields.
action is create, update, delete, or noop. kind is user, feedback, project, or reference.
For create/update, name is a lowercase slug, description is one line, and content is non-empty.
For delete, omit description and content. For noop, return {"action":"noop"}.
Example: [{"action":"create","kind":"user","name":"prefers-any","description":"Prefers any","content":"Use any instead of interface{}."}].`
}

func memoryText(messages []provider.Message) string {
	var text strings.Builder
	for _, message := range messages {
		for _, block := range message.Blocks {
			if block.Type == provider.BlockText {
				text.WriteString(block.Text)
			}
		}
	}
	return text.String()
}
