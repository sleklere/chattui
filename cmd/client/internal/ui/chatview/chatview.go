// Package chatview renders the two halves of a conversation screen: the message
// history and the composer box below it. Shared by the room chat and DM screens.
package chatview

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/chattui/cmd/client/internal/ui/components"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
)

// groupWindow is how long consecutive messages from the same sender keep
// getting appended to the same block instead of starting a new one.
const groupWindow = 5 * time.Minute

// Message is a single rendered chat message.
type Message struct {
	SenderID int64
	Sender   string
	Body     string
	At       time.Time
}

// Render turns the message history into the transcript body, grouping
// consecutive messages by sender and inserting day separators.
func Render(msgs []Message, meID int64, width int) string {
	if len(msgs) == 0 {
		return ""
	}

	t := theme.Current
	timeStyle := lipgloss.NewStyle().Foreground(t.Subtle)
	bodyStyle := lipgloss.NewStyle().Foreground(t.Text).Width(bodyWidth(width))

	var lines []string
	var prev *Message

	for i := range msgs {
		msg := msgs[i]

		if prev == nil || !sameDay(prev.At, msg.At) {
			if prev != nil {
				lines = append(lines, "")
			}
			lines = append(lines, components.LabeledRule(width, dayLabel(msg.At)), "")
		} else if !grouped(*prev, msg) {
			lines = append(lines, "")
		}

		if prev == nil || !sameDay(prev.At, msg.At) || !grouped(*prev, msg) {
			stamp := timeStyle.Render(msg.At.Format("15:04"))
			lines = append(lines, header(msg, meID, width, stamp))
		}

		for _, line := range strings.Split(bodyStyle.Render(msg.Body), "\n") {
			lines = append(lines, "   "+line)
		}
		prev = &msgs[i]
	}

	return strings.Join(lines, "\n")
}

func header(msg Message, meID int64, width int, stamp string) string {
	t := theme.Current

	name := msg.Sender
	color := theme.SpeakerColor(msg.Sender)
	if msg.SenderID == meID {
		name = "you"
		color = t.OwnMsg
	}

	left := lipgloss.NewStyle().Foreground(color).Render("●") + " " +
		lipgloss.NewStyle().Foreground(color).Bold(true).Render(name)

	gap := width - lipgloss.Width(left) - lipgloss.Width(stamp) - 1
	if gap < 1 {
		return " " + left
	}
	return " " + left + strings.Repeat(" ", gap) + stamp
}

// grouped reports whether msg continues the block started by prev.
func grouped(prev, msg Message) bool {
	if prev.SenderID != msg.SenderID {
		return false
	}
	// Messages from other clients can arrive slightly out of order; only a
	// forward gap breaks the block.
	return msg.At.Sub(prev.At) < groupWindow && msg.At.After(prev.At.Add(-groupWindow))
}

func bodyWidth(width int) int {
	w := width - 4
	if w < 10 {
		return 10
	}
	return w
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func dayLabel(at time.Time) string {
	now := time.Now()
	switch {
	case sameDay(at, now):
		return "Today"
	case sameDay(at, now.AddDate(0, 0, -1)):
		return "Yesterday"
	default:
		return at.Format("Mon, 02 Jan")
	}
}

// ComposerHeight is the number of rows Composer occupies.
const ComposerHeight = 3

// Composer renders the message input box.
func Composer(input textinput.Model, width int) string {
	t := theme.Current
	prompt := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render("❯ ")

	boxWidth := width - 4
	if boxWidth < 10 {
		boxWidth = 10
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Overlay).
		Padding(0, 1).
		MarginLeft(1).
		Width(boxWidth).
		Render(prompt + input.View())
}
