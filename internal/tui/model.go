package tui

import (
	"context"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/GitHub-freshman-X/mewcode01/internal/conversation"
	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type Model struct {
	conversation                 *conversation.Conversation
	textarea                     textarea.Model
	viewport                     viewport.Model
	width, height                int
	autoFollow, thinkingExpanded bool
	events                       <-chan provider.StreamEvent
	done                         <-chan error
	ctx                          context.Context
}

func NewModel(c *conversation.Conversation) *Model {
	input := textarea.New()
	input.Placeholder = "输入消息，按 Enter 发送"
	input.Prompt = "> "
	input.ShowLineNumbers = false
	input.SetHeight(3)
	input.SetWidth(80)
	input.Focus()
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(16))
	vp.SoftWrap = true
	m := &Model{conversation: c, textarea: input, viewport: vp, width: 80, height: 20, autoFollow: true, ctx: context.Background()}
	m.refreshContent()
	return m
}

func (m *Model) Init() tea.Cmd { return m.textarea.Focus() }
