package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
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
	if m.session != nil {
		for _, message := range m.session.DisplaySnapshot() {
			renderMessage(&b, message, false, m.thinkingExpanded)
		}
	}
	if m.task != nil || m.current.terminalTy == agent.EventCancelled || m.current.terminalTy == agent.EventFailed {
		if m.current.prompt != "" {
			renderMessage(&b, provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: m.current.prompt}}}, false, false)
		}
		if m.current.text != "" {
			text := m.current.text
			if m.current.terminal != nil && m.current.terminal.Partial {
				text += "\n（部分输出）"
			}
			renderMessage(&b, provider.Message{Role: provider.RoleAssistant, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: text}}}, m.task != nil, false)
		}
		for _, call := range m.current.toolCalls {
			fmt.Fprintf(&b, "工具调用: %s\n", call.Name)
		}
		for _, result := range m.current.toolResult {
			status := "成功"
			if result.IsError {
				status = "失败"
			}
			fmt.Fprintf(&b, "工具结果: %s %s\n", result.Name, status)
		}
		if m.current.err != nil {
			fmt.Fprintf(&b, "%s\n\n", errorStyle.Render("错误: "+provider.UserError(m.current.err)))
		}
	}
	if m.pendingPermission != nil {
		decision := m.pendingPermission.decision
		fmt.Fprintf(&b, "权限确认: %s\n", decision.Request.Tool)
		if decision.Request.MatchTarget != "" {
			fmt.Fprintf(&b, "对象: %s\n", decision.Request.MatchTarget)
		}
		if decision.Reason != "" {
			fmt.Fprintf(&b, "原因: %s\n", decision.Reason)
		}
		fmt.Fprintln(&b, "操作: o 本次允许 · s 本会话允许 · p 永久允许 · d 拒绝 · Ctrl+C 取消")
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
		case provider.BlockToolCall:
			if block.ToolCall != nil {
				fmt.Fprintf(b, "工具调用: %s\n", block.ToolCall.Name)
			}
		case provider.BlockToolResult:
			if block.ToolResult != nil {
				status := "成功"
				if block.ToolResult.IsError {
					status = "失败"
				}
				content, truncated := truncateRunes(block.ToolResult.Content, 240)
				if truncated {
					content += "..."
				}
				fmt.Fprintf(b, "工具结果: %s %s\n%s\n", block.ToolResult.Name, status, content)
			}
		}
	}
	b.WriteString("\n")
}

func (m *Model) statusText() string {
	if m.pendingPermission != nil {
		return "permission · 等待确认 · o/s/p 允许 · d 拒绝 · Ctrl+C 取消"
	}
	if m.task != nil {
		return fmt.Sprintf("iteration %d · %s · tokens in:%d out:%d · Ctrl+C 取消", m.current.iteration, m.current.phase, m.current.usage.InputTokens, m.current.usage.OutputTokens)
	}
	if m.current.terminal != nil {
		return fmt.Sprintf("%s · %s · %d iterations · tokens in:%d out:%d · 可继续输入", m.current.terminalTy, m.current.terminal.Reason, m.current.terminal.Iterations, m.current.terminal.Usage.InputTokens, m.current.terminal.Usage.OutputTokens)
	}
	if m.current.err != nil {
		return "failed · " + provider.UserError(m.current.err) + " · 可继续输入"
	}
	return "idle · Enter 发送 · Ctrl+C 退出"
}

func (m *Model) hasThinking() bool {
	if m.session == nil {
		return false
	}
	for _, message := range m.session.DisplaySnapshot() {
		for _, block := range message.Blocks {
			if block.Type == provider.BlockThinking && block.Text != "" {
				return true
			}
		}
	}
	return false
}

func truncateRunes(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}
