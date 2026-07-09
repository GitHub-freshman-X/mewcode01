package tui

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

var (
	userStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	assistantStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	thinkingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

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
