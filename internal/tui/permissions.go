package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/permissions"
)

type PermissionBridge struct {
	requests chan permissionRequestMsg
}

type permissionRequestMsg struct {
	decision permissions.Decision
	reply    chan permissions.Choice
}

type pendingPermission struct {
	decision permissions.Decision
	reply    chan permissions.Choice
}

func NewPermissionBridge() *PermissionBridge {
	return &PermissionBridge{requests: make(chan permissionRequestMsg)}
}

func (b *PermissionBridge) Confirm(ctx context.Context, decision permissions.Decision) (permissions.Confirmation, error) {
	reply := make(chan permissions.Choice, 1)
	req := permissionRequestMsg{decision: decision, reply: reply}
	select {
	case b.requests <- req:
	case <-ctx.Done():
		return permissions.Confirmation{}, ctx.Err()
	}
	select {
	case choice := <-reply:
		return permissions.Confirmation{Decision: decision, Choice: choice}, nil
	case <-ctx.Done():
		return permissions.Confirmation{}, ctx.Err()
	}
}

func (b *PermissionBridge) Wait() permissionRequestMsg {
	if b == nil {
		return permissionRequestMsg{}
	}
	return <-b.requests
}

func waitForPermission(bridge *PermissionBridge) tea.Cmd {
	return func() tea.Msg {
		return bridge.Wait()
	}
}
