package tui

import (
	"context"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type taskView struct {
	prompt     string
	text       string
	iteration  int
	phase      agent.Phase
	usage      provider.Usage
	toolCalls  []provider.ToolCall
	toolResult []provider.ToolResult
	terminal   *agent.Summary
	terminalTy agent.EventType
	err        error
}

type Model struct {
	runner                       *agent.Runner
	session                      *conversation.Session
	task                         *agent.Task
	permissionBridge             *PermissionBridge
	pendingPermission            *pendingPermission
	current                      taskView
	textarea                     textarea.Model
	viewport                     viewport.Model
	width, height                int
	autoFollow, thinkingExpanded bool
	ctx                          context.Context
}

func NewModel(runner *agent.Runner, session *conversation.Session) *Model {
	return NewModelWithPermissions(runner, session, nil)
}

func NewModelWithPermissions(runner *agent.Runner, session *conversation.Session, bridge *PermissionBridge) *Model {
	input := textarea.New()
	input.Placeholder = "输入消息，按 Enter 发送"
	input.Prompt = "> "
	input.ShowLineNumbers = false
	input.SetStyles(inputStyles())
	input.SetHeight(3)
	input.SetWidth(80)
	input.Focus()
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(16))
	vp.SoftWrap = true
	m := &Model{runner: runner, session: session, permissionBridge: bridge, textarea: input, viewport: vp, width: 80, height: 20, autoFollow: true, ctx: context.Background()}
	m.refreshContent()
	return m
}

func (m *Model) Init() tea.Cmd {
	if m.permissionBridge != nil {
		return tea.Batch(m.textarea.Focus(), waitForPermission(m.permissionBridge))
	}
	return m.textarea.Focus()
}
