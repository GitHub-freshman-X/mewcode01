package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

func (m *Model) View() tea.View {
	content := m.viewport.View() + "\n" + m.textarea.View() + "\n" + statusStyle.Render(m.statusText())
	v := tea.NewView(content)
	v.AltScreen = true
	v.Cursor = m.textarea.Cursor()
	return v
}

func (m *Model) refreshContent() {
	wasBottom := m.viewport.AtBottom()
	var b strings.Builder
	for _, message := range m.conversation.History() {
		renderMessage(&b, message, false, m.thinkingExpanded)
	}
	turn := m.conversation.ActiveTurn()
	if turn != nil && turn.State != conversation.TurnCompleted {
		renderMessage(&b, turn.UserMessage, false, false)
		renderMessage(&b, turn.AssistantMessage, turn.State == conversation.TurnThinking || turn.State == conversation.TurnGenerating || turn.State == conversation.TurnConnecting, m.thinkingExpanded)
		if turn.Err != nil {
			fmt.Fprintf(&b, "%s\n\n", errorStyle.Render("错误: "+provider.UserError(turn.Err)))
		}
	}
	m.viewport.SetContent(strings.TrimRight(b.String(), "\n"))
	if m.autoFollow || wasBottom {
		m.viewport.GotoBottom()
		m.autoFollow = true
	}
}

func renderMessage(b *strings.Builder, message provider.Message, active, expanded bool) {
	if len(message.Blocks) == 0 {
		return
	}
	label := userStyle.Render("你")
	if message.Role == provider.RoleAssistant {
		label = assistantStyle.Render("MewCode")
	}
	fmt.Fprintf(b, "%s\n", label)
	for _, block := range message.Blocks {
		switch block.Type {
		case provider.BlockText:
			if block.Text != "" {
				fmt.Fprintln(b, block.Text)
			}
		case provider.BlockThinking:
			if block.Text == "" {
				continue
			}
			if active || expanded {
				fmt.Fprintln(b, thinkingStyle.Render("思考\n"+block.Text))
			} else {
				fmt.Fprintln(b, thinkingStyle.Render("思考（已折叠，Ctrl+T 展开）"))
			}
		}
	}
	b.WriteString("\n")
}

func (m *Model) statusText() string {
	turn := m.conversation.ActiveTurn()
	if turn == nil {
		return "idle · Enter 发送 · Ctrl+C 退出"
	}
	switch turn.State {
	case conversation.TurnConnecting:
		return "connecting · Ctrl+C 取消"
	case conversation.TurnThinking:
		return "thinking · Ctrl+C 取消"
	case conversation.TurnGenerating:
		return "generating · Ctrl+C 取消"
	case conversation.TurnCompleted:
		return "completed · Ctrl+T 展开思考"
	case conversation.TurnCancelled:
		return "cancelled · 可继续输入"
	case conversation.TurnFailed:
		return "failed · 可继续输入"
	default:
		return string(turn.State)
	}
}

func (m *Model) hasThinking() bool {
	for _, message := range m.conversation.History() {
		for _, block := range message.Blocks {
			if block.Type == provider.BlockThinking && block.Text != "" {
				return true
			}
		}
	}
	if turn := m.conversation.ActiveTurn(); turn != nil {
		for _, block := range turn.AssistantMessage.Blocks {
			if block.Type == provider.BlockThinking && block.Text != "" {
				return true
			}
		}
	}
	return false
}
