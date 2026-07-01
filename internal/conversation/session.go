package conversation

import (
	"errors"
	"strings"
	"sync"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type Session struct {
	mu         sync.RWMutex
	history    []provider.Message
	latestPlan string
}

func NewSession() *Session { return &Session{} }

func (s *Session) Snapshot() []provider.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return provider.CloneMessages(s.history)
}

func (s *Session) CommitRound(user *provider.Message, assistant provider.Message, results []provider.ToolResult) error {
	if assistant.Role != provider.RoleAssistant {
		return errors.New("assistant message has invalid role")
	}
	callIDs := make(map[string]struct{})
	for _, block := range assistant.Blocks {
		if block.Type == provider.BlockToolCall && block.ToolCall != nil {
			if block.ToolCall.ID == "" {
				return errors.New("assistant tool call is missing id")
			}
			callIDs[block.ToolCall.ID] = struct{}{}
		}
	}
	if len(callIDs) != len(results) {
		return errors.New("tool call and result counts do not match")
	}
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		if _, ok := callIDs[result.CallID]; !ok {
			return errors.New("tool result does not match an assistant call")
		}
		if _, ok := seen[result.CallID]; ok {
			return errors.New("duplicate tool result call id")
		}
		seen[result.CallID] = struct{}{}
	}
	var additions []provider.Message
	if user != nil {
		if user.Role != provider.RoleUser {
			return errors.New("user message has invalid role")
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
	s.mu.Lock()
	s.history = append(s.history, additions...)
	s.mu.Unlock()
	return nil
}

func (s *Session) LatestPlan() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestPlan, s.latestPlan != ""
}

func (s *Session) SavePlan(plan string) error {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return errors.New("plan is empty")
	}
	s.mu.Lock()
	s.latestPlan = plan
	s.mu.Unlock()
	return nil
}
