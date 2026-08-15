package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/command"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type taskView struct {
	prompt      string
	text        string
	iteration   int
	phase       agent.Phase
	usage       provider.Usage
	toolCalls   []provider.ToolCall
	toolResult  []provider.ToolResult
	compactions []agent.CompactionEvent
	terminal    *agent.Summary
	terminalTy  agent.EventType
	err         error
}

type systemMessage struct {
	content string
	after   int
	role    provider.Role
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
	commands                     *command.Registry
	planMode                     bool
	systemMessages               []systemMessage
	systemMessageEpoch           int
	completion                   []string
	completionIndex              int
	memoryClearPending           bool
	exitRequested                bool
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
	m := &Model{runner: runner, session: session, permissionBridge: bridge, textarea: input, viewport: vp, width: 80, height: 20, autoFollow: true, ctx: context.Background(), commands: command.DefaultRegistry()}
	m.refreshContent()
	return m
}

func (m *Model) AddSystemMessage(message string) {
	after := 0
	if m.session != nil {
		after = len(m.session.DisplaySnapshot())
	}
	if m.hasTransientTaskView() {
		after = -1
	}
	m.systemMessages = append(m.systemMessages, systemMessage{content: message, after: after, role: provider.RoleAssistant})
	m.refreshContent()
}

func (m *Model) AddCommandMessage(message string, start int, reset bool) {
	if reset || start < 0 || start > len(m.systemMessages) {
		start = 0
	}
	after := 0
	if start >= 0 && start < len(m.systemMessages) {
		after = m.systemMessages[start].after
	} else if m.session != nil {
		after = len(m.session.DisplaySnapshot())
	}
	m.systemMessages = append(m.systemMessages, systemMessage{})
	copy(m.systemMessages[start+1:], m.systemMessages[start:])
	m.systemMessages[start] = systemMessage{content: message, after: after, role: provider.RoleUser}
	m.refreshContent()
}

func (m *Model) hasTransientTaskView() bool {
	return m.task != nil || m.current.terminalTy == agent.EventCancelled || m.current.terminalTy == agent.EventFailed
}
func (m *Model) SetPlanMode(enabled bool) { m.planMode = enabled; m.RefreshStatus() }
func (m *Model) PlanMode() bool           { return m.planMode }
func (m *Model) RequestExit()             { m.exitRequested = true }
func (m *Model) TokenUsage() provider.Usage {
	if m.session != nil {
		return m.session.Usage()
	}
	return provider.Usage{}
}
func (m *Model) RefreshStatus()                   { m.refreshContent() }
func (m *Model) MemoryClearPending() bool         { return m.memoryClearPending }
func (m *Model) SetMemoryClearPending(value bool) { m.memoryClearPending = value }
func (m *Model) StartAgent(req agent.Request) error {
	if m.runner == nil {
		return fmt.Errorf("agent runner is not configured")
	}
	task, err := m.runner.Start(m.ctx, req)
	if err != nil {
		return err
	}
	m.task, m.current = task, taskView{prompt: m.textarea.Value()}
	m.textarea.Reset()
	m.textarea.Blur()
	return nil
}

func (m *Model) Current() command.SessionMeta {
	count := 0
	title := ""
	if m.session != nil {
		messages := m.session.DisplaySnapshot()
		count = len(messages)
		for _, message := range messages {
			if message.Role != provider.RoleUser {
				continue
			}
			for _, block := range message.Blocks {
				if block.Type == provider.BlockText && strings.TrimSpace(block.Text) != "" {
					title = strings.TrimSpace(block.Text)
					break
				}
			}
			if title != "" {
				break
			}
		}
	}
	return command.SessionMeta{ID: m.runner.SessionID(), Title: title, MessageCount: count}
}

func (m *Model) List() ([]command.SessionMeta, error) {
	store := m.runner.SessionStore()
	if store == nil {
		return nil, fmt.Errorf("session store is not configured")
	}
	metas, err := store.List()
	if err != nil {
		return nil, err
	}
	out := make([]command.SessionMeta, len(metas))
	for i, meta := range metas {
		out[i] = command.SessionMeta{ID: meta.ID, Title: meta.Title, MessageCount: meta.MessageCount}
	}
	return out, nil
}

func (m *Model) New(context.Context) error {
	store := m.runner.SessionStore()
	if store == nil {
		return fmt.Errorf("session store is not configured")
	}
	session, meta, err := store.Create()
	if err != nil {
		return err
	}
	if err := m.runner.ReplaceSession(session, meta.ID); err != nil {
		return err
	}
	m.session, m.current, m.systemMessages, m.planMode = session, taskView{}, nil, false
	m.systemMessageEpoch++
	return nil
}

func (m *Model) Resume(_ context.Context, id string) error {
	store := m.runner.SessionStore()
	if store == nil {
		return fmt.Errorf("session store is not configured")
	}
	session, meta, err := store.Restore(id)
	if err != nil {
		return err
	}
	if err := m.runner.ReplaceSession(session, meta.ID); err != nil {
		return err
	}
	m.session, m.current, m.systemMessages, m.planMode = session, taskView{}, nil, false
	m.systemMessageEpoch++
	return nil
}

func (m *Model) Delete(id string) error {
	store := m.runner.SessionStore()
	if store == nil {
		return fmt.Errorf("session store is not configured")
	}
	if id == m.runner.SessionID() {
		return fmt.Errorf("cannot delete current session")
	}
	return store.Delete(id)
}

func (m *Model) Summary() (command.MemorySummary, error) {
	service := m.runner.MemoryService()
	if service == nil {
		return command.MemorySummary{}, fmt.Errorf("memory service is not configured")
	}
	summary, err := service.CommandSummary()
	return command.MemorySummary{UserCount: summary.UserCount, ProjectCount: summary.ProjectCount}, err
}

func (m *Model) Items() ([]command.MemoryItem, error) {
	service := m.runner.MemoryService()
	if service == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	items, err := service.CommandList()
	if err != nil {
		return nil, err
	}
	out := make([]command.MemoryItem, len(items))
	for i, item := range items {
		out[i] = command.MemoryItem{Kind: string(item.Kind), Name: item.Name, Description: item.Description}
	}
	return out, nil
}

func (m *Model) Add(kind, content string) error {
	service := m.runner.MemoryService()
	if service == nil {
		return fmt.Errorf("memory service is not configured")
	}
	return service.CommandAdd(kind, content)
}
func (m *Model) Clear() error {
	service := m.runner.MemoryService()
	if service == nil {
		return fmt.Errorf("memory service is not configured")
	}
	return service.CommandClear()
}

func (m *Model) Init() tea.Cmd {
	if m.permissionBridge != nil {
		return tea.Batch(m.textarea.Focus(), waitForPermission(m.permissionBridge))
	}
	return m.textarea.Focus()
}
