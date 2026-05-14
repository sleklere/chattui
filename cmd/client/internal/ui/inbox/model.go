// Package inbox provides the inbox UI model for the chat client.
package inbox

import (
	"fmt"
	"io"
	"strings"
	"time"

	"encoding/json"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/ui/tabbar"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
	"github.com/sleklere/chattui/cmd/client/internal/ws"
)

// LeaveInboxMsg signals that the user wants to go back to rooms.
type LeaveInboxMsg struct{}

// ShowDMsMsg signals that the user wants to navigate to the DMs screen.
type ShowDMsMsg struct{}

// OpenRoomMsg signals that the user wants to open a room from the inbox.
type OpenRoomMsg struct {
	RoomID   int64
	RoomName string
}

// OpenDMMsg signals that the user wants to open a DM conversation from the inbox.
type OpenDMMsg struct {
	ConversationID int64
	PeerID         int64
	PeerUsername   string
}

// ErrorMsg signals an error while loading inbox entries.
type ErrorMsg struct {
	Err error
}

type entriesLoadedMsg struct {
	entries []api.InboxEntry
}

type entryItem struct {
	entry api.InboxEntry
}

func (i entryItem) FilterValue() string {
	if i.entry.SourceUser != nil {
		return i.entry.SourceUser.Username
	}
	if i.entry.Room != nil {
		return i.entry.Room.Name
	}
	return ""
}

type entryItemDelegate struct{}

func (d entryItemDelegate) Height() int                             { return 2 }
func (d entryItemDelegate) Spacing() int                            { return 0 }
func (d entryItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d entryItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(entryItem)
	if !ok {
		return
	}

	t := theme.Current
	line1 := formatEntry(i.entry)
	line2 := "  " + timeAgo(i.entry.CreatedAt)

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

func formatEntry(e api.InboxEntry) string {
	if e.EntryType == "event" {
		username := ""
		if e.SourceUser != nil {
			username = e.SourceUser.Username
		}
		roomName := ""
		if e.Room != nil {
			roomName = "#" + e.Room.Name
		}
		switch e.Kind {
		case "room_join":
			return fmt.Sprintf("%s joined %s", username, roomName)
		case "room_leave":
			return fmt.Sprintf("%s left %s", username, roomName)
		default:
			return fmt.Sprintf("%s: %s", e.Kind, username)
		}
	}

	// conversation entry
	unread := ""
	if e.UnreadCount > 0 {
		unread = fmt.Sprintf(" (%d)", e.UnreadCount)
	}
	if e.Room != nil {
		body := ""
		if e.LastMessage != nil {
			body = ": " + e.LastMessage.Body
		}
		return fmt.Sprintf("#%s%s%s", e.Room.Name, body, unread)
	}
	if e.SourceUser != nil {
		body := ""
		if e.LastMessage != nil {
			body = ": " + e.LastMessage.Body
		}
		return fmt.Sprintf("DM %s%s%s", e.SourceUser.Username, body, unread)
	}
	return "conversation"
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

	l := list.New([]list.Item{}, entryItemDelegate{}, width, height-6)
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
	return m.fetchEntries()
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
			return m, m.fetchEntries()
		case "enter":
			if item, ok := m.list.SelectedItem().(entryItem); ok {
				return m, openEntry(item.entry)
			}
		}

	case ws.IncomingMsg:
		if msg.Message.Type == ws.TypeInboxUpdated {
			var p ws.InboxUpdatedPayload
			if err := json.Unmarshal(msg.Message.Payload, &p); err == nil {
				entry := inboxEntryFromPayload(p)
				m.list.SetItems(upsertEntry(m.list.Items(), entry))
			}
			return m, nil
		}

	case entriesLoadedMsg:
		items := make([]list.Item, len(msg.entries))
		for i, e := range msg.entries {
			items[i] = entryItem{entry: e}
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

func openEntry(e api.InboxEntry) tea.Cmd {
	return func() tea.Msg {
		if e.Room != nil {
			return OpenRoomMsg{RoomID: e.Room.ID, RoomName: e.Room.Name}
		}
		if e.RefConversationID != nil && e.SourceUser != nil {
			return OpenDMMsg{
				ConversationID: *e.RefConversationID,
				PeerID:         e.SourceUser.ID,
				PeerUsername:   e.SourceUser.Username,
			}
		}
		return nil
	}
}

func inboxEntryFromPayload(p ws.InboxUpdatedPayload) api.InboxEntry {
	e := api.InboxEntry{
		EntryType:         p.EntryType,
		Kind:              p.Kind,
		RefConversationID: p.RefConversationID,
		UnreadCount:       p.UnreadCount,
		CreatedAt:         time.Now(),
	}
	if p.RefRoomID != nil {
		name := ""
		if p.RefRoomName != nil {
			name = *p.RefRoomName
		}
		e.Room = &api.InboxRoom{ID: *p.RefRoomID, Name: name}
	}
	if p.LastMessageBody != nil {
		senderID := int64(0)
		if p.LastMessageSenderID != nil {
			senderID = *p.LastMessageSenderID
		}
		e.LastMessage = &api.InboxLastMessage{Body: *p.LastMessageBody, SenderID: senderID}
	}
	if p.PeerID > 0 {
		e.SourceUser = &api.InboxUser{ID: p.PeerID, Username: p.PeerUsername}
	} else if p.SourceUserID > 0 {
		e.SourceUser = &api.InboxUser{ID: p.SourceUserID, Username: p.SourceUsername}
	}
	return e
}

// upsertEntry puts entry at the top of items, replacing any existing entry
// that refers to the same conversation or room.
func upsertEntry(items []list.Item, entry api.InboxEntry) []list.Item {
	result := make([]list.Item, 0, len(items)+1)
	result = append(result, entryItem{entry: entry})
	for _, item := range items {
		if !sameEntry(item.(entryItem).entry, entry) {
			result = append(result, item)
		}
	}
	return result
}

func sameEntry(a, b api.InboxEntry) bool {
	if a.RefConversationID != nil && b.RefConversationID != nil {
		return *a.RefConversationID == *b.RefConversationID
	}
	if a.Room != nil && b.Room != nil {
		return a.Room.ID == b.Room.ID
	}
	return false
}

func (m Model) fetchEntries() tea.Cmd {
	return func() tea.Msg {
		entries, err := m.apiClient.GetInbox(50)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return entriesLoadedMsg{entries: entries}
	}
}
