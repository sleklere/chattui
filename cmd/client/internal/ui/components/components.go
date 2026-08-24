// Package components provides small shared building blocks for the TUI:
// list rows, badges, empty states and section rules.
package components

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
)

const (
	// SelectedBar is the marker drawn on the left edge of the selected row.
	SelectedBar = "▌"
	// gutter is the width reserved for the selection bar plus its trailing space.
	gutter = 2
)

// Row renders a full-width list row: a selection bar, a left block and a
// right-aligned block, with the gap between them padded out. The row is
// highlighted when selected. left and right may contain ANSI styling.
func Row(width int, selected bool, left, right string) string {
	t := theme.Current

	bar := strings.Repeat(" ", gutter)
	if selected {
		bar = lipgloss.NewStyle().Foreground(t.Accent).Render(SelectedBar) + " "
	}

	inner := width - gutter - 1
	if inner < 1 {
		inner = 1
	}

	rightW := lipgloss.Width(right)
	leftW := inner - rightW
	if leftW < 1 {
		leftW = 1
		right = ""
		rightW = 0
	}
	left = ansi.Truncate(left, leftW, "…")

	gap := inner - lipgloss.Width(left) - rightW
	if gap < 0 {
		gap = 0
	}

	line := bar + left + strings.Repeat(" ", gap) + right
	if selected {
		return lipgloss.NewStyle().Background(t.Surface).Width(width).Render(line)
	}
	return lipgloss.NewStyle().Width(width).Render(line)
}

// SubRow renders a secondary line under a Row, indented to align with its text.
func SubRow(width int, selected bool, text string) string {
	t := theme.Current

	inner := width - gutter - 1
	if inner < 1 {
		inner = 1
	}
	line := strings.Repeat(" ", gutter) + ansi.Truncate(text, inner, "…")

	if selected {
		bar := lipgloss.NewStyle().Foreground(t.Accent).Render(SelectedBar) + " "
		line = bar + ansi.Truncate(text, inner, "…")
		return lipgloss.NewStyle().Background(t.Surface).Width(width).Render(line)
	}
	return lipgloss.NewStyle().Width(width).Render(line)
}

// Badge renders an unread counter as a filled pill. Returns "" for zero.
func Badge(count int64) string {
	if count <= 0 {
		return ""
	}
	t := theme.Current
	label := fmt.Sprintf("%d", count)
	if count > 99 {
		label = "99+"
	}
	return lipgloss.NewStyle().
		Foreground(t.Base).
		Background(t.Accent).
		Bold(true).
		Padding(0, 1).
		Render(label)
}

// Dot renders a small colored marker in the speaker color of the given name.
func Dot(name string) string {
	return lipgloss.NewStyle().Foreground(theme.SpeakerColor(name)).Render("●")
}

// Rule renders a horizontal divider of the given width.
func Rule(width int) string {
	if width < 1 {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(theme.Current.Surface).
		Render(strings.Repeat("─", width))
}

// LabeledRule renders a centered label inside a horizontal divider,
// used as a day separator in message history.
func LabeledRule(width int, label string) string {
	t := theme.Current
	text := lipgloss.NewStyle().Foreground(t.Subtle).Render(" " + label + " ")
	side := (width - lipgloss.Width(text)) / 2
	if side < 1 {
		return text
	}
	line := lipgloss.NewStyle().Foreground(t.Surface).Render(strings.Repeat("─", side))
	return line + text + line
}

// Empty renders a centered empty-state block with a title and a hint.
func Empty(width, height int, title, hint string) string {
	t := theme.Current
	body := lipgloss.JoinVertical(
		lipgloss.Center,
		lipgloss.NewStyle().Foreground(t.Subtle).Bold(true).Render(title),
		"",
		lipgloss.NewStyle().Foreground(t.Subtle).Render(hint),
	)
	if height < 3 {
		height = 3
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
}

// InputStyles returns the styling shared by every text input in the app: an
// accent prompt, a subdued placeholder and a plain cursor that only reverses
// the cell it sits on. Bubbles v2 replaced the loose style fields with a struct
// that has to be set as a whole, so the defaults are spelled out here.
func InputStyles() textinput.Styles {
	state := textinput.StyleState{
		Prompt:      lipgloss.NewStyle().Foreground(theme.Current.Accent),
		Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Suggestion:  lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
	return textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textinput.CursorStyle{Blink: true},
	}
}
