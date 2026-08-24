// Package inbox provides the inbox UI model for the chat client.
package inbox

import (
	"fmt"
	"io"
	"strings"
	"time"

	"encoding/json"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/ui/components"
	"github.com/sleklere/chattui/cmd/client/internal/ui/hud"
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
	selected := index == m.Index()
	e := i.entry

	titleStyle := lipgloss.NewStyle().Foreground(t.Text)
	switch {
	case selected:
		titleStyle = titleStyle.Foreground(t.Accent).Bold(true)
	case e.UnreadCount > 0:
		titleStyle = titleStyle.Bold(true)
	}

	left := entryIcon(e) + " " + titleStyle.Render(entryTitle(e))
	right := lipgloss.NewStyle().Foreground(t.Subtle).Render(timeAgo(e.CreatedAt))
	if badge := components.Badge(e.UnreadCount); badge != "" {
		right = badge + " " + right
	}

	preview := lipgloss.NewStyle().Foreground(t.Subtle).Render("  " + entryPreview(e))

	_, _ = fmt.Fprint(w, components.Row(m.Width(), selected, left, right))
	_, _ = fmt.Fprint(w, "\n")
	_, _ = fmt.Fprint(w, components.SubRow(m.Width(), selected, preview))
}

// entryIcon returns the source marker: rooms get a hash, DMs a colored dot.
func entryIcon(e dto.InboxFeed) string {
	if e.Room != nil {
		return lipgloss.NewStyle().Foreground(theme.SpeakerColor(e.Room.Name)).Render("#")
	}
	if e.SourceUser != nil {
		return components.Dot(e.SourceUser.Username)
	}
	return " "
}

// entryTitle returns the conversation or room the entry belongs to.
func entryTitle(e dto.InboxFeed) string {
	if e.Room != nil {
		return e.Room.Name
	}
	if e.SourceUser != nil {
		return e.SourceUser.Username
	}
	return "conversation"
}

// entryPreview returns the second line: the event description or the last message.
func entryPreview(e dto.InboxFeed) string {
	username := ""
	if e.SourceUser != nil {
		username = e.SourceUser.Username
	}

	if e.EntryType == "event" {
		switch e.Kind {
		case "room_join":
			return username + " joined the room"
		case "room_leave":
			return username + " left the room"
		default:
			return e.Kind
		}
	}

	if e.LastMessage == nil {
		return "no messages yet"
	}
	body := strings.ReplaceAll(e.LastMessage.Body, "\n", " ")
	if e.Room != nil && username != "" {
		return username + ": " + body
	}
	return body
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
	apiClient   *api.Client
	list        list.Model
	spinner     spinner.Model
	loading     bool
	roomsTotal  int64
	dmsTotal    int64
	showingHelp bool
	err         string
	width       int
	height      int
}

// New creates a new inbox Model.
func New(apiClient *api.Client, _ int64, width, height int) Model {
	t := theme.Current

	l := list.New([]list.Item{}, entryItemDelegate{}, width, hud.BodyHeight(height))
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.PaginationStyle = lipgloss.NewStyle().Foreground(t.Overlay).Padding(0, 0, 0, 2)
	l.FilterInput.Prompt = "/ "
	filter := components.InputStyles()
	filter.Cursor.Color = t.Gold
	l.FilterInput.SetStyles(filter)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(t.Accent)

	return Model{
		apiClient: apiClient,
		list:      l,
		spinner:   s,
		loading:   true,
		width:     width,
		height:    height,
	}
}

// Init initializes the inbox model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchEntries(), m.spinner.Tick)
}

// Update handles messages for the inbox model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.showingHelp {
			if msg.String() == "?" || msg.String() == "esc" {
				m.showingHelp = false
			}
			return m, nil
		}
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "?":
			m.showingHelp = true
			return m, nil
		case "esc", "tab":
			return m, func() tea.Msg { return LeaveInboxMsg{} }
		case "shift+tab":
			return m, func() tea.Msg { return ShowDMsMsg{} }
		case "r":
			m.loading = true
			return m, tea.Batch(m.fetchEntries(), m.spinner.Tick)
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

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

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
		m.loading = false
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
		m.loading = false
		m.err = msg.Err.Error()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, hud.BodyHeight(msg.Height))
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the inbox model.
func (m Model) View() string {
	frame := hud.Frame{
		Width:     m.width,
		Height:    m.height,
		ActiveTab: hud.TabInbox,
		Badges:    map[string]int64{hud.TabRooms: m.roomsTotal, hud.TabDMs: m.dmsTotal},
		Keys: []hud.Key{
			{Key: "↵", Label: "open"},
			{Key: "m", Label: "mark read"},
			{Key: "tab", Label: "switch"},
			{Key: "?", Label: "help"},
		},
		Err: m.err,
	}

	height := hud.BodyHeight(m.height)

	var body string
	switch {
	case m.loading:
		body = components.Empty(m.width, height, m.spinner.View()+" loading inbox", "")
	case len(m.list.Items()) == 0:
		body = components.Empty(m.width, height, "Inbox zero", "new messages and room activity will show up here")
	default:
		body = m.list.View()
	}

	if m.showingHelp {
		body = hud.Overlay(body, hud.Help(helpSections()), m.width, height)
	}
	return frame.Render(body)
}

func helpSections() []hud.HelpSection {
	return []hud.HelpSection{
		{Title: "Navigate", Keys: []hud.Key{
			{Key: "j / ↓", Label: "move down"},
			{Key: "k / ↑", Label: "move up"},
			{Key: "enter", Label: "open and mark read"},
			{Key: "/", Label: "filter"},
		}},
		{Title: "Screens", Keys: []hud.Key{
			{Key: "tab", Label: "rooms"},
			{Key: "shift+tab", Label: "direct messages"},
			{Key: "esc", Label: "back to rooms"},
		}},
		{Title: "Actions", Keys: []hud.Key{
			{Key: "m", Label: "mark as read"},
			{Key: "r", Label: "refresh"},
		}},
	}
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
