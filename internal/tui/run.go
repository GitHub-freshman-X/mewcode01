package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
)

func Run(runner *agent.Runner, session *conversation.Session) error {
	return RunWithPermissions(runner, session, nil)
}

func RunWithPermissions(runner *agent.Runner, session *conversation.Session, bridge *PermissionBridge) error {
	m := NewModelWithPermissions(runner, session, bridge)
	_, err := tea.NewProgram(m).Run()
	if m.task != nil {
		m.task.Cancel()
	}
	return err
}
