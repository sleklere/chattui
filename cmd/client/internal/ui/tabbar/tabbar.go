// Package tabbar provides a shared tab bar for the main list screens.
package tabbar

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
)

// Render returns a styled tab bar string with the active tab highlighted.
// roomsTotal, dmsTotal and inboxTotal are unread counts shown as badges on inactive tabs.
func Render(active string, roomsTotal, dmsTotal, inboxTotal int64) string {
	t := theme.Current
	totals := map[string]int64{"Rooms": roomsTotal, "DMs": dmsTotal, "Inbox": inboxTotal}
	tabs := []string{"Rooms", "DMs", "Inbox"}
	sep := lipgloss.NewStyle().Foreground(t.Surface).Render("  │  ")

	parts := make([]string, len(tabs))
	for i, tab := range tabs {
		label := tab
		if tab != active && totals[tab] > 0 {
			label = fmt.Sprintf("%s (%d)", tab, totals[tab])
		}
		if tab == active {
			parts[i] = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render("[" + label + "]")
		} else {
			parts[i] = lipgloss.NewStyle().Foreground(t.Subtle).Render(label)
		}
	}
	return strings.Join(parts, sep)
}
