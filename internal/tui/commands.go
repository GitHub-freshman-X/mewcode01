package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type streamEventMsg struct{ Event provider.StreamEvent }
type streamErrorMsg struct{ Err error }

func waitForStream(events <-chan provider.StreamEvent, done <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case event, ok := <-events:
			if ok {
				return streamEventMsg{Event: event}
			}
			events = nil
		case err, ok := <-done:
			if !ok {
				return streamErrorMsg{}
			}
			return streamErrorMsg{Err: err}
		}
		if events == nil {
			err, ok := <-done
			if !ok {
				return streamErrorMsg{}
			}
			return streamErrorMsg{Err: err}
		}
		return streamErrorMsg{Err: context.Canceled}
	}
}
