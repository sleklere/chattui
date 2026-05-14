// Package rooms provides the room list UI model for the chat client.
package rooms

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/ui/tabbar"
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

func (d roomItemDelegate) Height() int                             { return 2 }
func (d roomItemDelegate) Spacing() int                            { return 0 }
func (d roomItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d roomItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(roomItem)
	if !ok {
		return
	}

	t := theme.Current
	name := i.room.Name
	slug := i.room.Slug
	badge := ""
	if unread := d.badges[i.room.ID]; unread > 0 {
		badge = fmt.Sprintf(" (%d)", unread)
	}

	if index == m.Index() {
		nameStyle := lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
		slugStyle := lipgloss.NewStyle().Foreground(t.Subtle)
		badgeStyle := lipgloss.NewStyle().Foreground(t.Gold).Bold(true)
		indicator := lipgloss.NewStyle().Foreground(t.Accent).Render(">")
		str := fmt.Sprintf("%s %s%s %s", indicator, nameStyle.Render(name), badgeStyle.Render(badge), slugStyle.Render("#"+slug))
		_, _ = fmt.Fprint(w, str)
	} else {
		nameStyle := lipgloss.NewStyle().Foreground(t.Text)
		slugStyle := lipgloss.NewStyle().Foreground(t.Subtle)
		badgeStyle := lipgloss.NewStyle().Foreground(t.Gold)
		str := fmt.Sprintf("  %s%s %s", nameStyle.Render(name), badgeStyle.Render(badge), slugStyle.Render("#"+slug))
		_, _ = fmt.Fprint(w, str)
	}
}

// Model is the Bubble Tea model for the room list screen.
type Model struct {
	apiClient    *api.Client
	list         list.Model
	badges       map[int64]int64
	inboxTotal   int64
	creating     bool
	createInput  textinput.Model
	pickingTheme bool
	themeIndex   int
	err          string
	width        int
	height       int
}

// New creates a new rooms Model with the given API client and dimensions.
func New(apiClient *api.Client, badges map[int64]int64, inboxTotal int64, width, height int) Model {
	t := theme.Current

	if badges == nil {
		badges = make(map[int64]int64)
	}
	l := list.New([]list.Item{}, roomItemDelegate{badges: badges}, width, height-6)
	l.Title = "Rooms"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder(), false, false, true, false).
		BorderForeground(t.Surface)

	input := textinput.New()
	input.Placeholder = "room name"
	input.CharLimit = 50

	return Model{
		apiClient:   apiClient,
		list:        l,
		badges:      badges,
		inboxTotal:  inboxTotal,
		createInput: input,
		width:       width,
		height:      height,
	}
}

// Init initializes the rooms model.
func (m Model) Init() tea.Cmd {
	return m.fetchRooms()
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
		case "tab":
			return m, func() tea.Msg { return ShowDMsMsg{} }
		case "shift+tab":
			return m, func() tea.Msg { return ShowInboxMsg{} }
		case "d":
			return m, func() tea.Msg { return ShowDMsMsg{} }
		case "i":
			return m, func() tea.Msg { return ShowInboxMsg{} }
		case "r":
			return m, m.fetchRooms()
		case "enter":
			if item, ok := m.list.SelectedItem().(roomItem); ok {
				return m, m.joinAndSelect(item.room)
			}
		}

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
		items := make([]list.Item, len(msg.rooms))
		for i, r := range msg.rooms {
			items[i] = roomItem{room: r}
		}
		m.list.SetItems(items)
		return m, nil

	case roomCreatedMsg:
		m.creating = false
		return m, m.fetchRooms()

	case roomJoinedMsg:
		return m, func() tea.Msg {
			return RoomSelectedMsg{Room: msg.room}
		}

	case RoomErrorMsg:
		m.err = msg.Err.Error()
		m.creating = false
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

// View renders the rooms model.
func (m Model) View() string {
	t := theme.Current
	helpStyle := lipgloss.NewStyle().Foreground(t.Subtle).Italic(true)
	errorStyle := lipgloss.NewStyle().Foreground(t.Error)
	promptStyle := lipgloss.NewStyle().Foreground(t.Gold).Bold(true)

	var b strings.Builder

	b.WriteString(tabbar.Render("Rooms", 0, m.dmsTotal(), m.inboxTotal))
	b.WriteString("\n")
	b.WriteString(m.list.View())
	b.WriteString("\n")

	if m.creating {
		b.WriteString(promptStyle.Render("New room: "))
		b.WriteString(m.createInput.View())
		b.WriteString("\n")
	}

	if m.pickingTheme {
		b.WriteString(m.themePickerView())
		b.WriteString("\n")
	}

	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err))
		b.WriteString("\n")
	}

	if m.pickingTheme {
		b.WriteString(helpStyle.Render("j/k: navigate  enter: apply  esc: cancel"))
	} else {
		b.WriteString(helpStyle.Render("enter: join  n: new room  tab: switch tabs  t: theme  r: refresh  esc: quit"))
	}

	return b.String()
}

func (m Model) updateThemePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	names := theme.Names
	switch msg.String() {
	case "j", "down":
		m.themeIndex = (m.themeIndex + 1) % len(names)
		theme.SetTheme(names[m.themeIndex])
		m.refreshListStyles()
		return m, nil
	case "k", "up":
		m.themeIndex = (m.themeIndex - 1 + len(names)) % len(names)
		theme.SetTheme(names[m.themeIndex])
		m.refreshListStyles()
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

func (m *Model) refreshListStyles() {
	t := theme.Current
	m.list.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder(), false, false, true, false).
		BorderForeground(t.Surface)
}

func (m Model) themePickerView() string {
	t := theme.Current
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Padding(0, 2)

	titleStyle := lipgloss.NewStyle().Foreground(t.Gold).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(t.Text)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Theme"))
	b.WriteString("\n")
	for i, name := range theme.Names {
		if i == m.themeIndex {
			b.WriteString(selectedStyle.Render(fmt.Sprintf("> %s", name)))
		} else {
			b.WriteString(normalStyle.Render(fmt.Sprintf("  %s", name)))
		}
		if i < len(theme.Names)-1 {
			b.WriteString("\n")
		}
	}

	return boxStyle.Render(b.String())
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
