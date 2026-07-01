package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
		m.textarea.SetWidth(m.width)
		m.textarea.SetHeight(min(3, max(1, m.height-2)))
		m.viewport.SetWidth(m.width)
		m.viewport.SetHeight(max(1, m.height-m.textarea.Height()-1))
		m.refreshContent()
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		if key == keyCancelOrQuit {
			if m.task != nil {
				m.task.Cancel()
				return m, waitForAgent(m.task.Events)
			}
			return m, tea.Quit
		}
		if key == keyToggleThinking {
			if m.hasThinking() {
				m.thinkingExpanded = !m.thinkingExpanded
				m.refreshContent()
			}
			return m, nil
		}
		if key == keyPageUp || key == keyPageDown {
			m.viewport, _ = m.viewport.Update(msg)
			m.autoFollow = m.viewport.AtBottom()
			return m, nil
		}
		if m.task == nil && key == keySubmit {
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}
			req, err := parseRequest(input)
			if err != nil {
				m.current = taskView{prompt: input, terminalTy: agent.EventFailed, err: err}
				m.refreshContent()
				return m, nil
			}
			task, err := m.runner.Start(m.ctx, req)
			if err != nil {
				m.current = taskView{prompt: input, terminalTy: agent.EventFailed, err: err}
				m.refreshContent()
				return m, nil
			}
			m.task = task
			m.current = taskView{prompt: input}
			m.textarea.Reset()
			m.textarea.Blur()
			m.refreshContent()
			return m, waitForAgent(task.Events)
		}
	case agentEventMsg:
		m.applyAgentEvent(msg.Event)
		m.refreshContent()
		if m.task != nil {
			return m, waitForAgent(m.task.Events)
		}
		return m, m.textarea.Focus()
	case agentClosedMsg:
		m.task = nil
		m.refreshContent()
		return m, m.textarea.Focus()
	}
	var cmd tea.Cmd
	if m.task != nil {
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func parseRequest(input string) (agent.Request, error) {
	if input == "/do" {
		return agent.Request{Mode: agent.ModeDo}, nil
	}
	if input == "/plan" || strings.HasPrefix(input, "/plan ") {
		prompt := strings.TrimSpace(strings.TrimPrefix(input, "/plan"))
		if prompt == "" {
			return agent.Request{}, &commandError{"/plan requires a task"}
		}
		return agent.Request{Mode: agent.ModePlan, Prompt: prompt}, nil
	}
	return agent.Request{Mode: agent.ModeAct, Prompt: input}, nil
}

type commandError struct{ message string }

func (e *commandError) Error() string { return e.message }

func (m *Model) applyAgentEvent(event agent.Event) {
	m.current.iteration, m.current.phase = event.Iteration, event.Phase
	switch event.Type {
	case agent.EventTextDelta:
		m.current.text += event.Text
	case agent.EventToolCall:
		if event.ToolCall != nil {
			m.current.toolCalls = append(m.current.toolCalls, *event.ToolCall)
		}
	case agent.EventToolResult:
		if event.ToolResult != nil {
			m.current.toolResult = append(m.current.toolResult, *event.ToolResult)
		}
	case agent.EventUsage:
		if event.Usage != nil {
			m.current.usage.Add(*event.Usage)
		}
	case agent.EventCompleted, agent.EventStopped, agent.EventCancelled, agent.EventFailed:
		m.current.terminal, m.current.terminalTy, m.current.err = event.Summary, event.Type, event.Err
		m.task = nil
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
