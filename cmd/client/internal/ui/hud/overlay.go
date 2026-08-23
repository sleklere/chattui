package hud

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
)

// Modal wraps content in a bordered, titled box suitable for Overlay.
//
// The box deliberately carries no background fill: nested styles (the key
// reference, a text input, the theme list) reset the background when they end,
// which punches holes in a filled box. The rounded accent border plus the
// opaque field of spaces Overlay writes over the body is what separates it.
func Modal(title, content string) string {
	t := theme.Current
	head := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(title)
	rule := lipgloss.NewStyle().Foreground(t.Surface).Render(strings.Repeat("─", lipgloss.Width(content)))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Padding(1, 3).
		Render(head + "\n" + rule + "\n\n" + content)
}

// Overlay composites box centered on top of body, keeping the surrounding
// content visible. Both are expected to be rendered at the given dimensions.
func Overlay(body, box string, width, height int) string {
	boxLines := strings.Split(box, "\n")
	boxWidth := lipgloss.Width(box)
	boxHeight := len(boxLines)

	// A box that does not fit takes over the body instead of being composited,
	// and is clipped so it never pushes the frame's key bar off screen.
	if boxWidth >= width || boxHeight > height {
		return lipgloss.NewStyle().
			MaxWidth(width).
			MaxHeight(height).
			Render(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box))
	}

	x := (width - boxWidth) / 2
	y := (height - boxHeight) / 2

	lines := strings.Split(lipgloss.NewStyle().Width(width).Height(height).Render(body), "\n")
	for i, boxLine := range boxLines {
		row := y + i
		if row < 0 || row >= len(lines) {
			continue
		}
		left := ansi.Truncate(lines[row], x, "")
		left += strings.Repeat(" ", x-lipgloss.Width(left))
		right := ansi.TruncateLeft(lines[row], x+boxWidth, "")
		lines[row] = left + boxLine + right
	}
	return strings.Join(lines, "\n")
}

// HelpSection is a titled group of key bindings shown in the help overlay.
type HelpSection struct {
	Title string
	Keys  []Key
}

// Help renders the full key reference as a modal box, laying the sections out
// in two columns so it fits in a short terminal.
func Help(sections []HelpSection) string {
	t := theme.Current
	titleStyle := lipgloss.NewStyle().Foreground(t.Gold).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Width(12)
	labelStyle := lipgloss.NewStyle().Foreground(t.Text)

	column := func(group []HelpSection) []string {
		var lines []string
		for i, s := range group {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, titleStyle.Render(s.Title))
			for _, k := range s.Keys {
				lines = append(lines, keyStyle.Render(k.Key)+labelStyle.Render(k.Label))
			}
		}
		return lines
	}

	split := (len(sections) + 1) / 2
	left, right := column(sections[:split]), column(sections[split:])

	body := joinColumns(left, right, "    ")
	hint := lipgloss.NewStyle().Foreground(t.Subtle).Italic(true).Render("? or esc to close")

	return Modal("Keys", body+"\n\n"+hint)
}

// joinColumns lays two blocks of styled lines side by side, padding the left
// one to a uniform width so the right one stays aligned.
func joinColumns(left, right []string, gap string) string {
	width := 0
	for _, l := range left {
		if w := lipgloss.Width(l); w > width {
			width = w
		}
	}

	rows := len(left)
	if len(right) > rows {
		rows = len(right)
	}

	lines := make([]string, rows)
	for i := range lines {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines[i] = l + strings.Repeat(" ", width-lipgloss.Width(l)) + gap + r
	}
	return strings.Join(lines, "\n")
}
