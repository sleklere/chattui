// Package tabbar provides a shared tab bar for the main list screens.
package tabbar

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
)

// Render returns a styled tab bar string with the active tab highlighted.
func Render(active string) string {
	t := theme.Current
	tabs := []string{"Rooms", "DMs", "Inbox"}
	sep := lipgloss.NewStyle().Foreground(t.Surface).Render("  │  ")

	parts := make([]string, len(tabs))
	for i, tab := range tabs {
		if tab == active {
			parts[i] = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render("[" + tab + "]")
		} else {
			parts[i] = lipgloss.NewStyle().Foreground(t.Subtle).Render(tab)
		}
	}
	return strings.Join(parts, sep)
}
