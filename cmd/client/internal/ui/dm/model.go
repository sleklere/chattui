// Package dm provides the DM conversation list UI model.
package dm

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/ui/components"
	"github.com/sleklere/chattui/cmd/client/internal/ui/hud"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
	"github.com/sleklere/chattui/pkg/dto"
)

// ConvSelectedMsg signals that an existing conversation was selected.
type ConvSelectedMsg struct {
	Conv dto.Conversation
}

// NewDMMsg signals that the user wants to open a new DM with a specific user.
type NewDMMsg struct {
	PeerID       int64
	PeerUsername string
}

// LeaveDMListMsg signals that the user wants to go back to rooms.
type LeaveDMListMsg struct{}

// ShowInboxMsg signals that the user wants to navigate to the inbox screen.
type ShowInboxMsg struct{}

// ConvBadgeUpdateMsg updates the unread badge for a single conversation and the inbox total.
type ConvBadgeUpdateMsg struct {
	ConvID int64
	Count  int64
	Total  int64
}

// RefreshBadgesMsg replaces all conversation badges at once.
type RefreshBadgesMsg struct {
	Badges     map[int64]int64
	InboxTotal int64
}

type convsLoadedMsg struct {
	convs []dto.Conversation
}

type peerFoundMsg struct {
	user dto.User
}

type dmErrorMsg struct {
	err error
}

type convItem struct {
	conv dto.Conversation
}

func (i convItem) FilterValue() string { return i.conv.PeerUsername }

type convItemDelegate struct {
	badges map[int64]int64
}

func (d convItemDelegate) Height() int                             { return 1 }
func (d convItemDelegate) Spacing() int                            { return 0 }
func (d convItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d convItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(convItem)
	if !ok {
		return
	}

	t := theme.Current
	selected := index == m.Index()
	unread := d.badges[i.conv.ID]

	nameStyle := lipgloss.NewStyle().Foreground(t.Text)
	switch {
	case selected:
		nameStyle = nameStyle.Foreground(t.Accent).Bold(true)
	case unread > 0:
		nameStyle = nameStyle.Bold(true)
	}

	left := components.Dot(i.conv.PeerUsername) + " " + nameStyle.Render(i.conv.PeerUsername)
	_, _ = fmt.Fprint(w, components.Row(m.Width(), selected, left, components.Badge(unread)))
}

// Model is the Bubble Tea model for the DM conversation list screen.
type Model struct {
	apiClient   *api.Client
	list        list.Model
	spinner     spinner.Model
	loading     bool
	badges      map[int64]int64
	inboxTotal  int64
	creating    bool
	createInput textinput.Model
	showingHelp bool
	err         string
	width       int
	height      int
}

// New creates a new DM list Model.
func New(apiClient *api.Client, badges map[int64]int64, inboxTotal int64, width, height int) Model {
	if badges == nil {
		badges = make(map[int64]int64)
	}

	l := list.New([]list.Item{}, convItemDelegate{badges: badges}, width, hud.BodyHeight(height))
	t := theme.Current
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.PaginationStyle = lipgloss.NewStyle().Foreground(t.Overlay).Padding(0, 0, 0, 2)
	l.FilterInput.Prompt = "/ "
	filter := components.InputStyles()
	filter.Cursor.Color = t.Gold
	l.FilterInput.SetStyles(filter)

	input := textinput.New()
	input.Prompt = "❯ "
	input.Placeholder = "username"
	input.CharLimit = 50
	input.SetWidth(28)
	input.SetStyles(components.InputStyles())

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(t.Accent)

	return Model{
		apiClient:   apiClient,
		list:        l,
		spinner:     s,
		loading:     true,
		badges:      badges,
		inboxTotal:  inboxTotal,
		createInput: input,
		width:       width,
		height:      height,
	}
}

// Init initializes the DM list model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchConvs(), m.spinner.Tick)
}

// Update handles messages for the DM list model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.creating {
			return m.updateCreating(msg)
		}
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
		case "esc", "shift+tab":
			return m, func() tea.Msg { return LeaveDMListMsg{} }
		case "tab", "i":
			return m, func() tea.Msg { return ShowInboxMsg{} }
		case "r":
			m.loading = true
			return m, tea.Batch(m.fetchConvs(), m.spinner.Tick)
		case "n":
			m.creating = true
			m.createInput.SetValue("")
			m.createInput.Focus()
			return m, textinput.Blink
		case "enter":
			if item, ok := m.list.SelectedItem().(convItem); ok {
				return m, func() tea.Msg { return ConvSelectedMsg{Conv: item.conv} }
			}
		}

	case tea.PasteMsg:
		// Pastes are their own message in Bubble Tea v2, so the create modal has
		// to claim them or they fall through to the list below.
		if m.creating {
			var cmd tea.Cmd
			m.createInput, cmd = m.createInput.Update(msg)
			return m, cmd
		}

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case ConvBadgeUpdateMsg:
		m.badges[msg.ConvID] = msg.Count
		m.inboxTotal = msg.Total
		m.list.SetDelegate(convItemDelegate{badges: m.badges})
		return m, nil

	case RefreshBadgesMsg:
		m.badges = msg.Badges
		m.inboxTotal = msg.InboxTotal
		m.list.SetDelegate(convItemDelegate{badges: m.badges})
		return m, nil

	case convsLoadedMsg:
		m.loading = false
		items := make([]list.Item, len(msg.convs))
		for i, c := range msg.convs {
			items[i] = convItem{conv: c}
		}
		m.list.SetItems(items)
		return m, nil

	case peerFoundMsg:
		return m, func() tea.Msg {
			return NewDMMsg{PeerID: msg.user.ID, PeerUsername: msg.user.Username}
		}

	case dmErrorMsg:
		m.loading = false
		m.err = msg.err.Error()
		m.creating = false
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

// View renders the DM list model.
func (m Model) View() string {
	keys := []hud.Key{
		{Key: "↵", Label: "open"},
		{Key: "n", Label: "new DM"},
		{Key: "/", Label: "filter"},
		{Key: "tab", Label: "switch"},
		{Key: "?", Label: "help"},
	}
	if m.creating {
		keys = []hud.Key{{Key: "↵", Label: "start chat"}, {Key: "esc", Label: "cancel"}}
	}

	frame := hud.Frame{
		Width:     m.width,
		Height:    m.height,
		ActiveTab: hud.TabDMs,
		Badges:    map[string]int64{hud.TabRooms: m.roomsTotal(), hud.TabInbox: m.inboxTotal},
		Keys:      keys,
		Err:       m.err,
	}
	return frame.Render(m.body())
}

func (m Model) body() string {
	height := hud.BodyHeight(m.height)

	var body string
	switch {
	case m.loading:
		body = components.Empty(m.width, height, m.spinner.View()+" loading conversations", "")
	case len(m.list.Items()) == 0:
		body = components.Empty(m.width, height, "No conversations yet", "press n and type a username to start one")
	default:
		body = m.list.View()
	}

	switch {
	case m.showingHelp:
		return hud.Overlay(body, hud.Help(helpSections()), m.width, height)
	case m.creating:
		return hud.Overlay(body, hud.Modal("New DM", m.createInput.View()), m.width, height)
	}
	return body
}

func helpSections() []hud.HelpSection {
	return []hud.HelpSection{
		{Title: "Navigate", Keys: []hud.Key{
			{Key: "j / ↓", Label: "move down"},
			{Key: "k / ↑", Label: "move up"},
			{Key: "enter", Label: "open chat"},
			{Key: "/", Label: "filter"},
		}},
		{Title: "Screens", Keys: []hud.Key{
			{Key: "tab", Label: "inbox"},
			{Key: "shift+tab", Label: "rooms"},
			{Key: "esc", Label: "back to rooms"},
		}},
		{Title: "Actions", Keys: []hud.Key{
			{Key: "n", Label: "new DM"},
			{Key: "r", Label: "refresh"},
		}},
	}
}

func (m Model) updateCreating(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		username := strings.TrimSpace(m.createInput.Value())
		if username == "" {
			m.creating = false
			return m, nil
		}
		m.creating = false
		return m, m.lookupUser(username)
	case "esc":
		m.creating = false
		return m, nil
	}

	var cmd tea.Cmd
	m.createInput, cmd = m.createInput.Update(msg)
	return m, cmd
}

func (m Model) roomsTotal() int64 {
	var dmsTotal int64
	for _, v := range m.badges {
		dmsTotal += v
	}
	r := m.inboxTotal - dmsTotal
	if r < 0 {
		return 0
	}
	return r
}

func (m Model) fetchConvs() tea.Cmd {
	return func() tea.Msg {
		convs, err := m.apiClient.ListConversations()
		if err != nil {
			return dmErrorMsg{err: err}
		}
		return convsLoadedMsg{convs: convs}
	}
}

func (m Model) lookupUser(username string) tea.Cmd {
	return func() tea.Msg {
		user, err := m.apiClient.GetUserByUsername(username)
		if err != nil {
			return dmErrorMsg{err: err}
		}
		return peerFoundMsg{user: user}
	}
}
