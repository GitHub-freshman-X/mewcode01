package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/agent"
)

type agentEventMsg struct{ Event agent.Event }
type agentClosedMsg struct{}

func waitForAgent(events <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return agentClosedMsg{}
		}
		return agentEventMsg{Event: event}
	}
}
