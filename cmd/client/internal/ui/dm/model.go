// Package dm provides the DM conversation list UI model.
package dm

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

type convItemDelegate struct{}

func (d convItemDelegate) Height() int                             { return 2 }
func (d convItemDelegate) Spacing() int                            { return 0 }
func (d convItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d convItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(convItem)
	if !ok {
		return
	}

	t := theme.Current
	if index == m.Index() {
		nameStyle := lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
		indicator := lipgloss.NewStyle().Foreground(t.Accent).Render(">")
		_, _ = fmt.Fprintf(w, "%s %s", indicator, nameStyle.Render(i.conv.PeerUsername))
	} else {
		nameStyle := lipgloss.NewStyle().Foreground(t.Text)
		_, _ = fmt.Fprintf(w, "  %s", nameStyle.Render(i.conv.PeerUsername))
	}
}

// Model is the Bubble Tea model for the DM conversation list screen.
type Model struct {
	apiClient   *api.Client
	list        list.Model
	creating    bool
	createInput textinput.Model
	err         string
	width       int
	height      int
}

// New creates a new DM list Model.
func New(apiClient *api.Client, width, height int) Model {
	t := theme.Current

	l := list.New([]list.Item{}, convItemDelegate{}, width, height-6)
	l.Title = "Direct Messages"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder(), false, false, true, false).
		BorderForeground(t.Surface)

	input := textinput.New()
	input.Placeholder = "username"
	input.CharLimit = 50

	return Model{
		apiClient:   apiClient,
		list:        l,
		createInput: input,
		width:       width,
		height:      height,
	}
}

// Init initializes the DM list model.
func (m Model) Init() tea.Cmd {
	return m.fetchConvs()
}

// Update handles messages for the DM list model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.creating {
			return m.updateCreating(msg)
		}

		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return LeaveDMListMsg{} }
		case "tab":
			return m, func() tea.Msg { return ShowInboxMsg{} }
		case "shift+tab":
			return m, func() tea.Msg { return LeaveDMListMsg{} }
		case "i":
			return m, func() tea.Msg { return ShowInboxMsg{} }
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

	case convsLoadedMsg:
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
		m.err = msg.err.Error()
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

// View renders the DM list model.
func (m Model) View() string {
	t := theme.Current
	helpStyle := lipgloss.NewStyle().Foreground(t.Subtle).Italic(true)
	errorStyle := lipgloss.NewStyle().Foreground(t.Error)
	promptStyle := lipgloss.NewStyle().Foreground(t.Gold).Bold(true)

	var b strings.Builder

	b.WriteString(tabbar.Render("DMs"))
	b.WriteString("\n")
	b.WriteString(m.list.View())
	b.WriteString("\n")

	if m.creating {
		b.WriteString(promptStyle.Render("New DM with: "))
		b.WriteString(m.createInput.View())
		b.WriteString("\n")
	}

	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err))
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("enter: open  n: new DM  tab: switch tabs  esc: back to rooms"))

	return b.String()
}

func (m Model) updateCreating(msg tea.KeyMsg) (Model, tea.Cmd) {
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
