package tui

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

var (
	userStyle                 = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("24"))
	userMessageStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("24"))
	assistantStyle            = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("28"))
	assistantMsgStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("28"))
	assistantProgressStyle    = assistantStyle
	assistantProgressMsgStyle = assistantMsgStyle
	thinkingStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("238")).Italic(true)
	toolCallStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("237"))
	toolResultStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("236"))
	toolErrorStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("52"))
	errorStyle                = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("52"))
	sessionMarkerStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("14"))
	statusStyle               = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func displaySessionTitle(title string) string {
	const limit = 48
	if title == "" {
		return "（空会话）"
	}
	runes := []rune(title)
	if len(runes) <= limit {
		return title
	}
	return string(runes[:limit]) + "…"
}

func inputStyles() textarea.Styles {
	styles := textarea.DefaultDarkStyles()
	inputText := lipgloss.Color("15")
	inputMuted := lipgloss.Color("8")
	inputPrompt := lipgloss.Color("14")
	inputBackground := lipgloss.Color("0")

	styles.Focused.Text = styles.Focused.Text.Foreground(inputText)
	styles.Focused.CursorLine = styles.Focused.CursorLine.
		Foreground(inputText).
		Background(inputBackground)
	styles.Focused.Prompt = styles.Focused.Prompt.Foreground(inputPrompt)
	styles.Focused.Placeholder = styles.Focused.Placeholder.Foreground(inputMuted)
	styles.Focused.EndOfBuffer = styles.Focused.EndOfBuffer.Foreground(inputBackground)

	styles.Blurred.Text = styles.Blurred.Text.Foreground(inputText)
	styles.Blurred.CursorLine = styles.Blurred.CursorLine.Foreground(inputText)
	styles.Blurred.Prompt = styles.Blurred.Prompt.Foreground(inputMuted)
	styles.Blurred.Placeholder = styles.Blurred.Placeholder.Foreground(inputMuted)

	return styles
}
