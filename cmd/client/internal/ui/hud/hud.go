// Package hud renders the persistent layer around every screen: the top bar
// with tabs and connection status, the notice line and the bottom key bar.
// Screens render only their own body and hand it to Frame.Render, so the
// layout stays identical as the user moves between them.
package hud

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
)

// Height is the number of rows the frame takes from the terminal:
// two for the top bar, one for the notice line and one for the key bar.
const Height = 4

// Tab names shared by the three list screens.
const (
	TabRooms = "Rooms"
	TabDMs   = "DMs"
	TabInbox = "Inbox"
)

// Status is the websocket connection state shown in the top bar.
type Status int

// Connection states.
const (
	StatusOffline Status = iota
	StatusConnecting
	StatusLive
)

// Session holds the UI-global bits every screen shows in the top bar.
// It is set once by the app and read while rendering, like theme.Current.
type Session struct {
	Username string
	Status   Status
}

// CurrentSession is the active session shown in the top bar.
var CurrentSession Session

// SetStatus updates the connection indicator shown in the top bar.
func SetStatus(s Status) { CurrentSession.Status = s }

// SetUsername sets the user shown in the top bar.
func SetUsername(name string) { CurrentSession.Username = name }

// Key is a single hint rendered in the bottom key bar.
type Key struct {
	Key   string
	Label string
}

// Frame describes the layer drawn around a screen body.
type Frame struct {
	Width  int
	Height int

	// Tabs mode: ActiveTab is one of TabRooms/TabDMs/TabInbox and Badges holds
	// the unread count per tab. Title mode: Title/Subtitle describe the screen.
	ActiveTab string
	Badges    map[string]int64
	Title     string
	Subtitle  string

	Keys   []Key
	Err    string
	Notice string
}

// BodyHeight returns the rows left for a screen body inside a terminal of the
// given height.
func BodyHeight(height int) int {
	h := height - Height
	if h < 1 {
		return 1
	}
	return h
}

// Render wraps body in the application frame.
func (f Frame) Render(body string) string {
	width := f.Width
	if width < 20 {
		width = 20
	}

	bodyHeight := BodyHeight(f.Height)
	bodyBox := lipgloss.NewStyle().Width(width).Height(bodyHeight).MaxHeight(bodyHeight)

	return strings.Join([]string{
		f.topBar(width),
		lipgloss.NewStyle().Foreground(theme.Current.Surface).Render(strings.Repeat("─", width)),
		bodyBox.Render(body),
		f.noticeLine(width),
		f.keyBar(width),
	}, "\n")
}

func (f Frame) topBar(width int) string {
	t := theme.Current

	brand := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render("◆") +
		lipgloss.NewStyle().Foreground(t.Text).Bold(true).Render(" chattui")

	var center string
	if f.ActiveTab != "" {
		center = f.tabs()
	} else {
		center = lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render(f.Title)
		if f.Subtitle != "" {
			center += "  " + lipgloss.NewStyle().Foreground(t.Subtle).Render(f.Subtitle)
		}
	}

	right := f.statusChip()
	if CurrentSession.Username != "" {
		right += "  " + lipgloss.NewStyle().Foreground(t.Subtle).Render("@"+CurrentSession.Username)
	}

	left := " " + brand + "   " + center
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		// Not enough room for the session info; drop it and keep navigation.
		right = f.statusChip()
		gap = width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	}
	if gap < 1 {
		return ansi.Truncate(left, width, "…")
	}
	return left + strings.Repeat(" ", gap) + right + " "
}

func (f Frame) tabs() string {
	t := theme.Current
	names := []string{TabRooms, TabDMs, TabInbox}

	parts := make([]string, 0, len(names))
	for _, name := range names {
		if name == f.ActiveTab {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(t.Base).
				Background(t.Accent).
				Bold(true).
				Padding(0, 1).
				Render(name))
			continue
		}
		label := lipgloss.NewStyle().Foreground(t.Subtle).Render(name)
		if n := f.Badges[name]; n > 0 {
			label = lipgloss.NewStyle().Foreground(t.Text).Render(name) + " " +
				lipgloss.NewStyle().Foreground(t.Gold).Bold(true).Render(fmt.Sprintf("%d", n))
		}
		parts = append(parts, lipgloss.NewStyle().Padding(0, 1).Render(label))
	}
	return strings.Join(parts, lipgloss.NewStyle().Foreground(t.Surface).Render("·"))
}

func (f Frame) statusChip() string {
	t := theme.Current
	var color lipgloss.Color
	var label string
	switch CurrentSession.Status {
	case StatusLive:
		color, label = t.Success, "live"
	case StatusConnecting:
		color, label = t.Gold, "connecting"
	default:
		color, label = t.Error, "offline"
	}
	return lipgloss.NewStyle().Foreground(color).Render("●") + " " +
		lipgloss.NewStyle().Foreground(t.Subtle).Render(label)
}

func (f Frame) noticeLine(width int) string {
	t := theme.Current
	var text string
	switch {
	case f.Err != "":
		text = lipgloss.NewStyle().Foreground(t.Error).Render("✗ " + f.Err)
	case f.Notice != "":
		text = lipgloss.NewStyle().Foreground(t.Gold).Render("• " + f.Notice)
	default:
		return strings.Repeat(" ", width)
	}
	return lipgloss.NewStyle().Width(width).Render(" " + ansi.Truncate(text, width-2, "…"))
}

func (f Frame) keyBar(width int) string {
	t := theme.Current
	keyStyle := lipgloss.NewStyle().Foreground(t.Accent).Background(t.Surface).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(t.Subtle).Background(t.Surface)

	parts := make([]string, 0, len(f.Keys))
	for _, k := range f.Keys {
		parts = append(parts, keyStyle.Render(k.Key)+labelStyle.Render(" "+k.Label))
	}
	content := " " + strings.Join(parts, labelStyle.Render("   "))

	return lipgloss.NewStyle().
		Background(t.Surface).
		Width(width).
		MaxHeight(1).
		Render(ansi.Truncate(content, width, "…"))
}
