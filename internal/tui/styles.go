package tui

import "charm.land/lipgloss/v2"

var (
	userStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	assistantStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	thinkingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)
