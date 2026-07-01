package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
)

func Run(runner *agent.Runner, session *conversation.Session) error {
	m := NewModel(runner, session)
	_, err := tea.NewProgram(m).Run()
	if m.task != nil {
		m.task.Cancel()
	}
	return err
}
