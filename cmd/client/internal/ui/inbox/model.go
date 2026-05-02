// Package inbox provides the inbox UI model for the chat client.
package inbox

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/realtime-chat/cmd/client/internal/api"
	"github.com/sleklere/realtime-chat/cmd/client/internal/ui/tabbar"
	"github.com/sleklere/realtime-chat/cmd/client/internal/ui/theme"
)

// LeaveInboxMsg signals that the user wants to go back to rooms.
type LeaveInboxMsg struct{}

// ShowDMsMsg signals that the user wants to navigate to the DMs screen.
type ShowDMsMsg struct{}

// ErrorMsg signals an error while loading inbox events.
type ErrorMsg struct {
	Err error
}

type eventsLoadedMsg struct {
	events []api.InboxEvent
}

type eventItem struct {
	event api.InboxEvent
}

func (i eventItem) FilterValue() string { return i.event.SourceUsername }

type eventItemDelegate struct{}

func (d eventItemDelegate) Height() int                             { return 2 }
func (d eventItemDelegate) Spacing() int                            { return 0 }
func (d eventItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d eventItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(eventItem)
	if !ok {
		return
	}

	t := theme.Current
	line1 := formatEvent(i.event)
	line2 := "  " + timeAgo(i.event.CreatedAt)

	if index == m.Index() {
		mainStyle := lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
		timeStyle := lipgloss.NewStyle().Foreground(t.Gold)
		indicator := lipgloss.NewStyle().Foreground(t.Accent).Render(">")
		_, _ = fmt.Fprintf(w, "%s %s\n%s", indicator, mainStyle.Render(line1), timeStyle.Render(line2))
	} else {
		mainStyle := lipgloss.NewStyle().Foreground(t.Text)
		timeStyle := lipgloss.NewStyle().Foreground(t.Subtle)
		_, _ = fmt.Fprintf(w, "  %s\n%s", mainStyle.Render(line1), timeStyle.Render(line2))
	}
}

func formatEvent(e api.InboxEvent) string {
	switch e.Kind {
	case "room_join":
		return fmt.Sprintf("%s joined room #%d", e.SourceUsername, e.RoomID)
	case "room_leave":
		return fmt.Sprintf("%s left room #%d", e.SourceUsername, e.RoomID)
	default:
		return fmt.Sprintf("%s: %s", e.Kind, e.SourceUsername)
	}
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// Model is the Bubble Tea model for the inbox screen.
type Model struct {
	apiClient *api.Client
	list      list.Model
	err       string
	width     int
	height    int
}

// New creates a new inbox Model.
func New(apiClient *api.Client, width, height int) Model {
	t := theme.Current

	l := list.New([]list.Item{}, eventItemDelegate{}, width, height-6)
	l.Title = "Inbox"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder(), false, false, true, false).
		BorderForeground(t.Surface)

	return Model{
		apiClient: apiClient,
		list:      l,
		width:     width,
		height:    height,
	}
}

// Init initializes the inbox model.
func (m Model) Init() tea.Cmd {
	return m.fetchEvents()
}

// Update handles messages for the inbox model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return LeaveInboxMsg{} }
		case "tab":
			return m, func() tea.Msg { return LeaveInboxMsg{} }
		case "shift+tab":
			return m, func() tea.Msg { return ShowDMsMsg{} }
		case "r":
			return m, m.fetchEvents()
		}

	case eventsLoadedMsg:
		items := make([]list.Item, len(msg.events))
		for i, e := range msg.events {
			items[i] = eventItem{event: e}
		}
		m.list.SetItems(items)
		return m, nil

	case ErrorMsg:
		m.err = msg.Err.Error()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-6)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the inbox model.
func (m Model) View() string {
	t := theme.Current
	helpStyle := lipgloss.NewStyle().Foreground(t.Subtle).Italic(true)
	errorStyle := lipgloss.NewStyle().Foreground(t.Error)

	var b strings.Builder

	b.WriteString(tabbar.Render("Inbox"))
	b.WriteString("\n")
	b.WriteString(m.list.View())
	b.WriteString("\n")

	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err))
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("tab/shift+tab: switch tabs  r: refresh  esc: back to rooms"))

	return b.String()
}

func (m Model) fetchEvents() tea.Cmd {
	return func() tea.Msg {
		events, err := m.apiClient.GetInbox(50)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return eventsLoadedMsg{events: events}
	}
}
