package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

const startupCatBanner = ` /\_/\\
( o.o )
 > ^ <
`

func (m *Model) View() tea.View {
	content := m.viewport.View() + "\n" + m.textarea.View() + "\n" + statusStyle.Render(m.statusText())
	v := tea.NewView(content)
	v.AltScreen = true
	v.Cursor = m.textarea.Cursor()
	return v
}

func (m *Model) refreshContent() {
	wasBottom := m.viewport.AtBottom()
	m.viewport.SetContent(m.renderViewportContent())
	if m.autoFollow || wasBottom {
		m.viewport.GotoBottom()
		m.autoFollow = true
	}
}

func (m *Model) renderViewportContent() string {
	var b strings.Builder
	if m.showStartupCatBanner {
		b.WriteString(startupCatBanner)
		b.WriteString("\n")
	}
	for _, segment := range m.historySegments {
		if segment.content != "" {
			fmt.Fprintln(&b, segment.content)
			b.WriteString("\n")
		}
	}
	if m.sessionBoundary != nil {
		fmt.Fprintln(&b, renderSessionBoundary(*m.sessionBoundary))
		b.WriteString("\n")
	}
	b.WriteString(m.renderCurrentContent())
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) renderCurrentContent() string {
	var b strings.Builder
	var messages []provider.Message
	if m.session != nil {
		messages = m.session.DisplaySnapshot()
	}
	persistedCalls, persistedResults := displayedToolIDs(messages)
	renderSystemMessages(&b, m.systemMessages, 0)
	for i, message := range messages {
		renderMessage(&b, message, false, m.thinkingExpanded)
		renderSystemMessages(&b, m.systemMessages, i+1)
	}
	if len(m.completion) > 1 {
		fmt.Fprintln(&b, "命令候选:")
		for i, item := range m.completion {
			marker := "  "
			if i == m.completionIndex {
				marker = "> "
			}
			fmt.Fprintf(&b, "%s%s\n", marker, item)
		}
	}
	if m.task != nil || m.current.terminalTy == agent.EventCancelled || m.current.terminalTy == agent.EventFailed || len(m.current.compactions) > 0 {
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
			if _, ok := persistedCalls[call.ID]; ok {
				continue
			}
			fmt.Fprintln(&b, toolCallStyle.Render("工具调用: "+call.Name))
		}
		for _, result := range m.current.toolResult {
			if _, ok := persistedResults[result.CallID]; ok {
				continue
			}
			status := "成功"
			if result.IsError {
				status = "失败"
			}
			style := toolResultStyle
			if result.IsError {
				style = toolErrorStyle
			}
			fmt.Fprintln(&b, style.Render(fmt.Sprintf("工具结果: %s %s", result.Name, status)))
		}
		for _, compaction := range m.current.compactions {
			if compaction.Error != "" {
				fmt.Fprintln(&b, toolErrorStyle.Render(fmt.Sprintf("上下文压缩: %s 失败: %s", compaction.Trigger, compaction.Error)))
				continue
			}
			if compaction.BeforeTokens != 0 || compaction.AfterTokens != 0 {
				fmt.Fprintln(&b, toolCallStyle.Render(fmt.Sprintf("上下文压缩: %s %d -> %d tokens", compaction.Trigger, compaction.BeforeTokens, compaction.AfterTokens)))
			} else if len(compaction.Persisted) > 0 {
				fmt.Fprintln(&b, toolCallStyle.Render(fmt.Sprintf("上下文压缩: 工具结果已持久化 %d 项", len(compaction.Persisted))))
			}
		}
		if m.current.err != nil {
			fmt.Fprintf(&b, "%s\n\n", errorStyle.Render("错误: "+provider.UserError(m.current.err)))
		}
	}
	renderSystemMessages(&b, m.systemMessages, -1)
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
	return strings.TrimRight(b.String(), "\n")
}

func displayedToolIDs(messages []provider.Message) (map[string]struct{}, map[string]struct{}) {
	calls := make(map[string]struct{})
	results := make(map[string]struct{})
	for _, message := range messages {
		for _, block := range message.Blocks {
			if block.ToolCall != nil && block.ToolCall.ID != "" {
				calls[block.ToolCall.ID] = struct{}{}
			}
			if block.ToolResult != nil && block.ToolResult.CallID != "" {
				results[block.ToolResult.CallID] = struct{}{}
			}
		}
	}
	return calls, results
}

func renderSystemMessages(b *strings.Builder, messages []systemMessage, after int) {
	for _, message := range messages {
		if message.after == after {
			if message.role == provider.RoleUser {
				renderMessage(b, provider.Message{Role: provider.RoleUser, Blocks: []provider.ContentBlock{{Type: provider.BlockText, Text: message.content}}}, false, false)
				continue
			}
			fmt.Fprintf(b, "%s\n%s\n\n", assistantStyle.Render("系统"), assistantMsgStyle.Render(message.content))
		}
	}
}

func renderMessage(b *strings.Builder, message provider.Message, active, expanded bool) {
	if len(message.Blocks) == 0 {
		return
	}
	label := userStyle.Render("你")
	textStyle := userMessageStyle
	if message.Role == provider.RoleAssistant {
		if active || messageHasToolCall(message) {
			label, textStyle = assistantProgressStyle.Render("MewCode"), assistantProgressMsgStyle
		} else {
			label, textStyle = assistantStyle.Render("MewCode"), assistantMsgStyle
		}
	}
	fmt.Fprintf(b, "%s\n", label)
	for _, block := range message.Blocks {
		switch block.Type {
		case provider.BlockText:
			if block.Text != "" {
				fmt.Fprintln(b, textStyle.Render(block.Text))
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
				fmt.Fprintln(b, toolCallStyle.Render("工具调用: "+block.ToolCall.Name))
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
				style := toolResultStyle
				if block.ToolResult.IsError {
					style = toolErrorStyle
				}
				fmt.Fprintln(b, style.Render(fmt.Sprintf("工具结果: %s %s\n%s", block.ToolResult.Name, status, content)))
			}
		}
	}
	b.WriteString("\n")
}

func messageHasToolCall(message provider.Message) bool {
	for _, block := range message.Blocks {
		if block.Type == provider.BlockToolCall && block.ToolCall != nil {
			return true
		}
	}
	return false
}

func renderSessionBoundary(boundary sessionBoundary) string {
	return sessionMarkerStyle.Render(fmt.Sprintf("会话开始 · %s · %s", boundary.id, displaySessionTitle(boundary.title)))
}

func (m *Model) statusText() string {
	mode := "[DEFAULT]"
	if m.planMode {
		mode = "[PLAN]"
	}
	if m.pendingPermission != nil {
		return mode + " · permission · 等待确认 · o/s/p 允许 · d 拒绝 · Ctrl+C 取消"
	}
	if m.task != nil {
		usage := m.TokenUsage()
		if m.runner != nil && m.runner.HasForegroundSubAgent() {
			return fmt.Sprintf("%s · 子 Agent 前台运行 · ESC 转后台 · Ctrl+C 取消 · tokens in:%d out:%d", mode, usage.InputTokens, usage.OutputTokens)
		}
		return fmt.Sprintf("%s · iteration %d · %s · tokens in:%d out:%d · Ctrl+C 取消", mode, m.current.iteration, m.current.phase, usage.InputTokens, usage.OutputTokens)
	}
	if m.current.terminal != nil {
		usage := m.TokenUsage()
		return fmt.Sprintf("%s · %s · %s · %d iterations · tokens in:%d out:%d · 可继续输入", mode, m.current.terminalTy, m.current.terminal.Reason, m.current.terminal.Iterations, usage.InputTokens, usage.OutputTokens)
	}
	if m.current.err != nil {
		return mode + " · failed · " + provider.UserError(m.current.err) + " · 可继续输入"
	}
	return mode + " · idle · tokens in:" + fmt.Sprint(m.TokenUsage().InputTokens) + " out:" + fmt.Sprint(m.TokenUsage().OutputTokens) + " · Enter 发送 · /help"
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
