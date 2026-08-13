package conversation

import (
	"errors"
	"strings"
	"sync"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type Session struct {
	mu           sync.RWMutex
	history      []provider.Message
	display      []provider.Message
	pendingPlans []string
	journal      Journal
}

func NewSession(journals ...Journal) *Session {
	session := &Session{}
	if len(journals) > 0 {
		session.journal = journals[0]
	}
	return session
}

func (s *Session) Snapshot() []provider.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return provider.CloneMessages(s.history)
}

func (s *Session) ReplaceHistory(messages []provider.Message) {
	s.mu.Lock()
	s.history = provider.CloneMessages(messages)
	s.mu.Unlock()
}

func (s *Session) DisplaySnapshot() []provider.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return provider.CloneMessages(s.display)
}

func BuildRound(user *provider.Message, assistant provider.Message, results []provider.ToolResult) ([]provider.Message, error) {
	if assistant.Role != provider.RoleAssistant {
		return nil, errors.New("assistant message has invalid role")
	}
	callIDs := make(map[string]struct{})
	for _, block := range assistant.Blocks {
		if block.Type == provider.BlockToolCall && block.ToolCall != nil {
			if block.ToolCall.ID == "" {
				return nil, errors.New("assistant tool call is missing id")
			}
			callIDs[block.ToolCall.ID] = struct{}{}
		}
	}
	if len(callIDs) != len(results) {
		return nil, errors.New("tool call and result counts do not match")
	}
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		if _, ok := callIDs[result.CallID]; !ok {
			return nil, errors.New("tool result does not match an assistant call")
		}
		if _, ok := seen[result.CallID]; ok {
			return nil, errors.New("duplicate tool result call id")
		}
		seen[result.CallID] = struct{}{}
	}
	var additions []provider.Message
	if user != nil {
		if user.Role != provider.RoleUser {
			return nil, errors.New("user message has invalid role")
		}
		additions = append(additions, provider.CloneMessage(*user))
	}
	additions = append(additions, provider.CloneMessage(assistant))
	if len(results) > 0 {
		blocks := make([]provider.ContentBlock, len(results))
		for i := range results {
			result := results[i]
			blocks[i] = provider.ContentBlock{Type: provider.BlockToolResult, ToolResult: &result}
		}
		additions = append(additions, provider.Message{Role: provider.RoleUser, Blocks: blocks})
	}
	return additions, nil
}

func (s *Session) CommitRound(user *provider.Message, assistant provider.Message, results []provider.ToolResult) error {
	additions, err := BuildRound(user, assistant, results)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal != nil {
		if err := s.journal.Append(provider.CloneMessages(additions), JournalPurposeHistory); err != nil {
			return err
		}
	}
	s.history = append(s.history, provider.CloneMessages(additions)...)
	s.display = append(s.display, provider.CloneMessages(additions)...)
	return nil
}

func (s *Session) PendingPlans() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.pendingPlans...)
}

func (s *Session) CommitPlan(user provider.Message, assistant provider.Message, plan string) error {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return errors.New("plan is empty")
	}
	additions, err := BuildRound(&user, assistant, nil)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal != nil {
		if err := s.journal.Append(provider.CloneMessages(additions), JournalPurposePlan); err != nil {
			return err
		}
	}
	s.display = append(s.display, provider.CloneMessages(additions)...)
	s.pendingPlans = append(s.pendingPlans, plan)
	return nil
}

func (s *Session) ConsumePlans(plans []string) error {
	if len(plans) == 0 {
		return errors.New("plan snapshot is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(plans) > len(s.pendingPlans) {
		return errors.New("plan snapshot is longer than pending plans")
	}
	for i, plan := range plans {
		if plan != s.pendingPlans[i] {
			return errors.New("plan snapshot does not match pending plans")
		}
	}
	s.pendingPlans = append([]string(nil), s.pendingPlans[len(plans):]...)
	return nil
}
