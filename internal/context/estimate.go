package context

import "github.com/GitHub-freshman-X/mewcode01/internal/provider"

type State struct {
	UsageAnchorInput  int
	AnchorMessages    []provider.Message
	AutomaticFailures int
}

type Manager struct {
	Config Config
	Store  *ResultStore
	State  State
}

func NewManager(cfg Config, store *ResultStore) *Manager { return &Manager{Config: cfg, Store: store} }

func (m *Manager) RecordUsage(usage provider.Usage, messages []provider.Message) {
	m.State.UsageAnchorInput = usage.InputTokens
	m.State.AnchorMessages = provider.CloneMessages(messages)
}

func (m *Manager) Estimate(messages []provider.Message) int {
	start := len(m.State.AnchorMessages)
	if start > len(messages) {
		start = 0
	}
	chars := 0
	for _, message := range messages[start:] {
		for _, block := range message.Blocks {
			chars += len(block.Text)
			if block.ToolCall != nil {
				chars += len(block.ToolCall.ID) + len(block.ToolCall.Name) + len(block.ToolCall.Arguments)
			}
			if block.ToolResult != nil {
				chars += len(block.ToolResult.CallID) + len(block.ToolResult.Name) + len(block.ToolResult.Content)
			}
		}
	}
	return m.State.UsageAnchorInput + (chars+3)/4
}

func (m *Manager) Decision(messages []provider.Message, manual bool) (Trigger, bool) {
	if manual {
		return TriggerManual, true
	}
	estimate := m.Estimate(messages)
	if estimate >= m.Config.WindowTokens-m.Config.SummaryOutputTokens-m.Config.ManualSafetyTokens {
		return TriggerForced, true
	}
	if m.State.AutomaticFailures < 3 && estimate >= m.Config.WindowTokens-m.Config.SummaryOutputTokens-m.Config.AutoSafetyTokens {
		return TriggerAutomatic, true
	}
	return "", false
}
