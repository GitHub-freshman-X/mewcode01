package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
)

func Run(c *conversation.Conversation) error {
	m := NewModel(c)
	_, err := tea.NewProgram(m).Run()
	if c.IsBusy() {
		c.Cancel()
	}
	return err
}
