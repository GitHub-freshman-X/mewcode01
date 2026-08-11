package agent

import (
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type taskHistory struct {
	messages []provider.Message
}

func newTaskHistory(base []provider.Message) *taskHistory {
	return &taskHistory{messages: provider.CloneMessages(base)}
}

func (h *taskHistory) Snapshot() []provider.Message {
	if h == nil {
		return nil
	}
	return provider.CloneMessages(h.messages)
}

func (h *taskHistory) Replace(messages []provider.Message) {
	if h == nil {
		return
	}
	h.messages = provider.CloneMessages(messages)
}

func (h *taskHistory) CommitRound(user *provider.Message, assistant provider.Message, results []provider.ToolResult) error {
	additions, err := conversation.BuildRound(user, assistant, results)
	if err != nil {
		return err
	}
	h.messages = append(h.messages, additions...)
	return nil
}
