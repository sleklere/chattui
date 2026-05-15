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
	"github.com/sleklere/chattui/pkg/dto"
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

// BadgesMsg carries unread counts for all rooms and conversations.
type BadgesMsg struct {
	RoomBadges map[int64]int64
	ConvBadges map[int64]int64
}

type entriesLoadedMsg struct {
	entries []dto.InboxFeed
}

type markedAsReadMsg struct {
	entry dto.InboxFeed
}

type entryItem struct {
	entry dto.InboxFeed
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

func formatEntry(e dto.InboxFeed) string {
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
	apiClient  *api.Client
	list       list.Model
	roomsTotal int64
	dmsTotal   int64
	err        string
	width      int
	height     int
}

// New creates a new inbox Model.
func New(apiClient *api.Client, _ int64, width, height int) Model {
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
		case "m":
			if item, ok := m.list.SelectedItem().(entryItem); ok {
				if item.entry.UnreadCount > 0 {
					return m, markAsReadCmd(m.apiClient, item.entry)
				}
			}
		case "enter":
			if item, ok := m.list.SelectedItem().(entryItem); ok {
				cmds := []tea.Cmd{openEntry(item.entry)}
				if item.entry.UnreadCount > 0 {
					cmds = append(cmds, markAsReadCmd(m.apiClient, item.entry))
				}
				return m, tea.Batch(cmds...)
			}
		}

	case ws.IncomingMsg:
		if msg.Message.Type == ws.TypeInboxUpdated {
			var p ws.InboxUpdatedPayload
			if err := json.Unmarshal(msg.Message.Payload, &p); err == nil {
				entry := inboxEntryFromPayload(p)
				m.list.SetItems(upsertEntry(m.list.Items(), entry))
				badges := computeBadges(m.list.Items())
				m.roomsTotal = badges.RoomsTotal()
				m.dmsTotal = badges.DmsTotal()
				return m, func() tea.Msg { return badges }
			}
			return m, nil
		}

	case markedAsReadMsg:
		items := zeroUnreadCount(m.list.Items(), msg.entry)
		m.list.SetItems(items)
		badges := computeBadges(items)
		m.roomsTotal = badges.RoomsTotal()
		m.dmsTotal = badges.DmsTotal()
		return m, func() tea.Msg { return badges }

	case entriesLoadedMsg:
		items := make([]list.Item, len(msg.entries))
		for i, e := range msg.entries {
			items[i] = entryItem{entry: e}
		}
		m.list.SetItems(items)
		badges := computeBadges(items)
		m.roomsTotal = badges.RoomsTotal()
		m.dmsTotal = badges.DmsTotal()
		return m, func() tea.Msg { return badges }

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

	b.WriteString(tabbar.Render("Inbox", m.roomsTotal, m.dmsTotal, 0))
	b.WriteString("\n")
	b.WriteString(m.list.View())
	b.WriteString("\n")

	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err))
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("tab/shift+tab: switch tabs  r: refresh  m: mark as read  esc: back to rooms"))

	return b.String()
}

func markAsReadCmd(client *api.Client, e dto.InboxFeed) tea.Cmd {
	return func() tea.Msg {
		var convID *int64
		var roomID *int64
		if e.RefConversationID != nil {
			convID = e.RefConversationID
		} else if e.Room != nil {
			roomID = &e.Room.ID
		}
		if convID == nil && roomID == nil {
			return nil
		}
		_ = client.MarkAsRead(convID, roomID, false)
		return markedAsReadMsg{entry: e}
	}
}

func zeroUnreadCount(items []list.Item, entry dto.InboxFeed) []list.Item {
	result := make([]list.Item, len(items))
	for i, item := range items {
		e := item.(entryItem).entry
		if sameEntry(e, entry) {
			e.UnreadCount = 0
			result[i] = entryItem{entry: e}
		} else {
			result[i] = item
		}
	}
	return result
}

func openEntry(e dto.InboxFeed) tea.Cmd {
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

func inboxEntryFromPayload(p ws.InboxUpdatedPayload) dto.InboxFeed {
	e := dto.InboxFeed{
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
		e.Room = &dto.InboxRoom{ID: *p.RefRoomID, Name: name}
	}
	if p.LastMessageBody != nil {
		senderID := int64(0)
		if p.LastMessageSenderID != nil {
			senderID = *p.LastMessageSenderID
		}
		e.LastMessage = &dto.InboxLastMessage{Body: *p.LastMessageBody, SenderID: senderID}
	}
	if p.PeerID > 0 {
		e.SourceUser = &dto.InboxUser{ID: p.PeerID, Username: p.PeerUsername}
	} else if p.SourceUserID > 0 {
		e.SourceUser = &dto.InboxUser{ID: p.SourceUserID, Username: p.SourceUsername}
	}
	return e
}

// upsertEntry puts entry at the top of items, replacing any existing entry
// that refers to the same conversation or room.
func upsertEntry(items []list.Item, entry dto.InboxFeed) []list.Item {
	result := make([]list.Item, 0, len(items)+1)
	result = append(result, entryItem{entry: entry})
	for _, item := range items {
		if !sameEntry(item.(entryItem).entry, entry) {
			result = append(result, item)
		}
	}
	return result
}

func sameEntry(a, b dto.InboxFeed) bool {
	if a.RefConversationID != nil && b.RefConversationID != nil {
		return *a.RefConversationID == *b.RefConversationID
	}
	if a.Room != nil && b.Room != nil {
		return a.Room.ID == b.Room.ID
	}
	return false
}

func computeBadges(items []list.Item) BadgesMsg {
	msg := BadgesMsg{
		RoomBadges: make(map[int64]int64),
		ConvBadges: make(map[int64]int64),
	}
	for _, item := range items {
		e := item.(entryItem).entry
		if e.UnreadCount == 0 {
			continue
		}
		if e.Room != nil {
			msg.RoomBadges[e.Room.ID] = e.UnreadCount
		} else if e.RefConversationID != nil {
			msg.ConvBadges[*e.RefConversationID] = e.UnreadCount
		}
	}
	return msg
}

// RoomsTotal returns the total unread count across all rooms.
func (m BadgesMsg) RoomsTotal() int64 {
	var t int64
	for _, v := range m.RoomBadges {
		t += v
	}
	return t
}

// DmsTotal returns the total unread count across all DM conversations.
func (m BadgesMsg) DmsTotal() int64 {
	var t int64
	for _, v := range m.ConvBadges {
		t += v
	}
	return t
}

// FetchBadgesCmd fetches the inbox and returns a BadgesMsg without requiring an open inbox screen.
func FetchBadgesCmd(apiClient *api.Client) tea.Cmd {
	return func() tea.Msg {
		entries, err := apiClient.GetInbox(50)
		if err != nil {
			return nil
		}
		items := make([]list.Item, len(entries))
		for i, e := range entries {
			items[i] = entryItem{entry: e}
		}
		return computeBadges(items)
	}
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
