package tui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
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
			if m.conversation.IsBusy() {
				m.conversation.Cancel()
				m.refreshContent()
				return m, waitForStream(m.events, m.done)
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
		if !m.conversation.IsBusy() && key == keySubmit {
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}
			events, done, err := m.conversation.Start(m.ctx, input)
			if err != nil {
				return m, nil
			}
			m.events, m.done = events, done
			m.textarea.Reset()
			m.textarea.Blur()
			m.refreshContent()
			return m, waitForStream(events, done)
		}
	case streamEventMsg:
		if err := m.conversation.Apply(msg.Event); err != nil {
			m.conversation.Fail(err)
			m.textarea.Focus()
			m.refreshContent()
			return m, nil
		}
		if msg.Event.Type == provider.EventCompleted {
			m.thinkingExpanded = false
			if err := m.conversation.Complete(); err != nil {
				m.conversation.Fail(err)
			}
		}
		m.refreshContent()
		return m, waitForStream(m.events, m.done)
	case streamErrorMsg:
		if msg.Err != nil {
			if errors.Is(msg.Err, context.Canceled) {
				m.conversation.Cancel()
			} else {
				m.conversation.Fail(msg.Err)
			}
		}
		m.events, m.done = nil, nil
		cmd := m.textarea.Focus()
		m.refreshContent()
		return m, cmd
	}
	var cmd tea.Cmd
	if m.conversation.IsBusy() {
		before := m.viewport.AtBottom()
		m.viewport, cmd = m.viewport.Update(msg)
		m.autoFollow = before && m.viewport.AtBottom()
		return m, cmd
	}
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
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
