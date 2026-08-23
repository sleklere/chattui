// Package rooms provides the room list UI model for the chat client.
package rooms

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/ui/components"
	"github.com/sleklere/chattui/cmd/client/internal/ui/hud"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
	"github.com/sleklere/chattui/pkg/dto"
)

// RoomSelectedMsg signals that a room has been selected and joined.
type RoomSelectedMsg struct {
	Room dto.Room
}

// ShowDMsMsg signals that the user wants to navigate to the DM screen.
type ShowDMsMsg struct{}

// ShowInboxMsg signals that the user wants to navigate to the inbox screen.
type ShowInboxMsg struct{}

// RoomErrorMsg signals an error in room operations.
type RoomErrorMsg struct {
	Err error
}

// RoomBadgeUpdateMsg updates the unread badge for a single room and the inbox total.
type RoomBadgeUpdateMsg struct {
	RoomID int64
	Count  int64
	Total  int64
}

// RefreshBadgesMsg replaces all room badges at once.
type RefreshBadgesMsg struct {
	Badges     map[int64]int64
	InboxTotal int64
}

type roomsLoadedMsg struct {
	rooms []dto.Room
}

type roomCreatedMsg struct {
	room dto.Room
}

type roomJoinedMsg struct {
	room dto.Room
}

type roomItem struct {
	room dto.Room
}

func (i roomItem) FilterValue() string { return i.room.Name }

type roomItemDelegate struct {
	badges map[int64]int64
}

func (d roomItemDelegate) Height() int                             { return 1 }
func (d roomItemDelegate) Spacing() int                            { return 0 }
func (d roomItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d roomItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(roomItem)
	if !ok {
		return
	}

	t := theme.Current
	selected := index == m.Index()
	unread := d.badges[i.room.ID]

	hashStyle := lipgloss.NewStyle().Foreground(theme.SpeakerColor(i.room.Slug))
	nameStyle := lipgloss.NewStyle().Foreground(t.Text)
	switch {
	case selected:
		nameStyle = nameStyle.Foreground(t.Accent).Bold(true)
	case unread > 0:
		nameStyle = nameStyle.Bold(true)
	}

	left := hashStyle.Render("#") + " " + nameStyle.Render(i.room.Name)
	_, _ = fmt.Fprint(w, components.Row(m.Width(), selected, left, components.Badge(unread)))
}

// Model is the Bubble Tea model for the room list screen.
type Model struct {
	apiClient    *api.Client
	list         list.Model
	spinner      spinner.Model
	loading      bool
	badges       map[int64]int64
	inboxTotal   int64
	creating     bool
	createInput  textinput.Model
	pickingTheme bool
	themeIndex   int
	showingHelp  bool
	err          string
	width        int
	height       int
}

// New creates a new rooms Model with the given API client and dimensions.
func New(apiClient *api.Client, badges map[int64]int64, inboxTotal int64, width, height int) Model {
	if badges == nil {
		badges = make(map[int64]int64)
	}

	l := list.New([]list.Item{}, roomItemDelegate{badges: badges}, width, hud.BodyHeight(height))
	configureList(&l)

	input := textinput.New()
	input.Prompt = "❯ "
	input.PromptStyle = lipgloss.NewStyle().Foreground(theme.Current.Accent)
	input.Placeholder = "room name"
	input.CharLimit = 50
	input.Width = 28

	return Model{
		apiClient:   apiClient,
		list:        l,
		spinner:     newSpinner(),
		loading:     true,
		badges:      badges,
		inboxTotal:  inboxTotal,
		createInput: input,
		width:       width,
		height:      height,
	}
}

func configureList(l *list.Model) {
	t := theme.Current
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(true)
	l.Styles.PaginationStyle = lipgloss.NewStyle().Foreground(t.Overlay).Padding(0, 0, 0, 2)
	l.FilterInput.Prompt = "/ "
	l.FilterInput.PromptStyle = lipgloss.NewStyle().Foreground(t.Accent)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(t.Accent)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(t.Gold)
}

func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.Current.Accent)
	return s
}

// HasOverlay reports whether a modal is open, so the app knows esc should
// close it instead of quitting.
func (m Model) HasOverlay() bool {
	return m.creating || m.pickingTheme || m.showingHelp || m.list.FilterState() != list.Unfiltered
}

// Init initializes the rooms model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchRooms(), m.spinner.Tick)
}

// Update handles messages for the rooms model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.creating {
			return m.updateCreating(msg)
		}
		if m.pickingTheme {
			return m.updateThemePicker(msg)
		}
		if m.showingHelp {
			if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
				m.showingHelp = false
			}
			return m, nil
		}
		// While filtering, every key belongs to the filter input.
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "n":
			m.creating = true
			m.createInput.SetValue("")
			m.createInput.Focus()
			return m, textinput.Blink
		case "t":
			m.pickingTheme = true
			for i, name := range theme.Names {
				if name == theme.Current.Name {
					m.themeIndex = i
					break
				}
			}
			return m, nil
		case "?":
			m.showingHelp = true
			return m, nil
		case "tab", "d":
			return m, func() tea.Msg { return ShowDMsMsg{} }
		case "shift+tab", "i":
			return m, func() tea.Msg { return ShowInboxMsg{} }
		case "r":
			m.loading = true
			return m, tea.Batch(m.fetchRooms(), m.spinner.Tick)
		case "enter":
			if item, ok := m.list.SelectedItem().(roomItem); ok {
				return m, m.joinAndSelect(item.room)
			}
		}

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case RoomBadgeUpdateMsg:
		m.badges[msg.RoomID] = msg.Count
		m.inboxTotal = msg.Total
		m.list.SetDelegate(roomItemDelegate{badges: m.badges})
		return m, nil

	case RefreshBadgesMsg:
		m.badges = msg.Badges
		m.inboxTotal = msg.InboxTotal
		m.list.SetDelegate(roomItemDelegate{badges: m.badges})
		return m, nil

	case roomsLoadedMsg:
		m.loading = false
		items := make([]list.Item, len(msg.rooms))
		for i, r := range msg.rooms {
			items[i] = roomItem{room: r}
		}
		m.list.SetItems(items)
		return m, nil

	case roomCreatedMsg:
		m.creating = false
		m.loading = true
		return m, tea.Batch(m.fetchRooms(), m.spinner.Tick)

	case roomJoinedMsg:
		return m, func() tea.Msg {
			return RoomSelectedMsg{Room: msg.room}
		}

	case RoomErrorMsg:
		m.loading = false
		m.err = msg.Err.Error()
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

// View renders the rooms model.
func (m Model) View() string {
	frame := hud.Frame{
		Width:     m.width,
		Height:    m.height,
		ActiveTab: hud.TabRooms,
		Badges:    map[string]int64{hud.TabDMs: m.dmsTotal(), hud.TabInbox: m.inboxTotal},
		Keys:      m.keys(),
		Err:       m.err,
	}
	return frame.Render(m.body())
}

func (m Model) keys() []hud.Key {
	switch {
	case m.creating:
		return []hud.Key{{Key: "↵", Label: "create"}, {Key: "esc", Label: "cancel"}}
	case m.pickingTheme:
		return []hud.Key{{Key: "j/k", Label: "preview"}, {Key: "↵", Label: "apply"}, {Key: "esc", Label: "cancel"}}
	default:
		return []hud.Key{
			{Key: "↵", Label: "join"},
			{Key: "n", Label: "new"},
			{Key: "/", Label: "filter"},
			{Key: "tab", Label: "switch"},
			{Key: "?", Label: "help"},
		}
	}
}

func (m Model) body() string {
	height := hud.BodyHeight(m.height)

	var body string
	switch {
	case m.loading:
		body = components.Empty(m.width, height, m.spinner.View()+" loading rooms", "")
	case len(m.list.Items()) == 0:
		body = components.Empty(m.width, height, "No rooms yet", "press n to create the first one")
	default:
		body = m.list.View()
	}

	switch {
	case m.showingHelp:
		return hud.Overlay(body, hud.Help(helpSections()), m.width, height)
	case m.creating:
		return hud.Overlay(body, hud.Modal("New room", m.createInput.View()), m.width, height)
	case m.pickingTheme:
		return hud.Overlay(body, hud.Modal("Theme", m.themeList()), m.width, height)
	}
	return body
}

func helpSections() []hud.HelpSection {
	return []hud.HelpSection{
		{Title: "Navigate", Keys: []hud.Key{
			{Key: "j / ↓", Label: "move down"},
			{Key: "k / ↑", Label: "move up"},
			{Key: "enter", Label: "join room"},
			{Key: "/", Label: "filter rooms"},
		}},
		{Title: "Screens", Keys: []hud.Key{
			{Key: "tab", Label: "direct messages"},
			{Key: "shift+tab", Label: "inbox"},
			{Key: "esc", Label: "quit"},
		}},
		{Title: "Actions", Keys: []hud.Key{
			{Key: "n", Label: "new room"},
			{Key: "r", Label: "refresh"},
			{Key: "t", Label: "change theme"},
		}},
	}
}

func (m Model) updateThemePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	names := theme.Names
	switch msg.String() {
	case "j", "down":
		m.themeIndex = (m.themeIndex + 1) % len(names)
		theme.SetTheme(names[m.themeIndex])
		m.refreshStyles()
		return m, nil
	case "k", "up":
		m.themeIndex = (m.themeIndex - 1 + len(names)) % len(names)
		theme.SetTheme(names[m.themeIndex])
		m.refreshStyles()
		return m, nil
	case "enter":
		m.pickingTheme = false
		_ = theme.Save()
		return m, nil
	case "esc":
		m.pickingTheme = false
		return m, nil
	}
	return m, nil
}

func (m *Model) refreshStyles() {
	configureList(&m.list)
	m.spinner.Style = lipgloss.NewStyle().Foreground(theme.Current.Accent)
}

func (m Model) themeList() string {
	t := theme.Current
	var b strings.Builder
	for i, name := range theme.Names {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == m.themeIndex {
			b.WriteString(lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Render("▸ " + name))
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(t.Text).Render("  " + name))
	}
	return b.String()
}

func (m Model) updateCreating(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.createInput.Value())
		if name == "" {
			m.creating = false
			return m, nil
		}
		m.creating = false
		return m, m.createRoom(name)
	case "esc":
		m.creating = false
		return m, nil
	}

	var cmd tea.Cmd
	m.createInput, cmd = m.createInput.Update(msg)
	return m, cmd
}

func (m Model) dmsTotal() int64 {
	var roomsTotal int64
	for _, v := range m.badges {
		roomsTotal += v
	}
	d := m.inboxTotal - roomsTotal
	if d < 0 {
		return 0
	}
	return d
}

func (m Model) fetchRooms() tea.Cmd {
	return func() tea.Msg {
		rooms, err := m.apiClient.ListRooms()
		if err != nil {
			return RoomErrorMsg{Err: err}
		}
		return roomsLoadedMsg{rooms: rooms}
	}
}

func (m Model) createRoom(name string) tea.Cmd {
	return func() tea.Msg {
		room, err := m.apiClient.CreateRoom(name)
		if err != nil {
			return RoomErrorMsg{Err: err}
		}
		return roomCreatedMsg{room: room}
	}
}

func (m Model) joinAndSelect(room dto.Room) tea.Cmd {
	return func() tea.Msg {
		if err := m.apiClient.JoinRoom(room.ID); err != nil {
			return RoomErrorMsg{Err: err}
		}
		return roomJoinedMsg{room: room}
	}
}
