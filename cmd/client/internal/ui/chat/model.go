// Package chat provides the chat room UI model for the chat client.
package chat

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/ui/theme"
	"github.com/sleklere/chattui/cmd/client/internal/ws"
)

// LeaveRoomMsg signals that the user wants to leave the current room.
type LeaveRoomMsg struct{}

type historyLoadedMsg struct {
	messages []api.MessageResponse
}

// Model is the Bubble Tea model for the chat room screen.
type Model struct {
	apiClient *api.Client
	wsClient  *ws.Client
	logger    *slog.Logger

	room     api.RoomResponse
	userID   int64
	username string

	viewport viewport.Model
	input    textinput.Model
	messages []chatMessage
	err      string
	width    int
	height   int
}

type chatMessage struct {
	senderID       int64
	senderUsername string
	content        string
	timestamp      string
}

// New creates a new chat Model for the given room.
func New(
	apiClient *api.Client,
	wsClient *ws.Client,
	logger *slog.Logger,
	room api.RoomResponse,
	userID int64,
	username string,
	width, height int,
) Model {
	vp := viewport.New(width, height-4)

	input := textinput.New()
	input.Placeholder = "type a message..."
	input.Focus()
	input.CharLimit = 500
	input.Width = width - 6

	return Model{
		apiClient: apiClient,
		wsClient:  wsClient,
		logger:    logger,
		room:      room,
		userID:    userID,
		username:  username,
		viewport:  vp,
		input:     input,
		width:     width,
		height:    height,
	}
}

// Init initializes the chat model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadHistory(),
		textinput.Blink,
	)
}

// Update handles messages for the chat model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return LeaveRoomMsg{} }
		case "enter":
			return m.sendMessage()
		}

	case historyLoadedMsg:
		// Messages come from the API in DESC order (newest first), reverse for display.
		for i := len(msg.messages) - 1; i >= 0; i-- {
			m2 := msg.messages[i]
			m.messages = append(m.messages, chatMessage{
				senderID:       m2.SenderID,
				senderUsername: m2.SenderUsername,
				content:        m2.Body,
				timestamp:      m2.CreatedAt.Format("15:04"),
			})
		}
		m.updateViewport()
		return m, nil

	case ws.IncomingMsg:
		return m.handleWSMessage(msg)

	case ws.ErrorMsg:
		m.err = msg.Err.Error()
		m.logger.Error("ws error in chat", "error", msg.Err)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4
		m.input.Width = msg.Width - 6
		m.updateViewport()
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the chat model.
func (m Model) View() string {
	t := theme.Current

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent).
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(t.Surface).
		Width(m.width)

	inputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Surface).
		Padding(0, 1).
		Width(m.width - 2)

	statusStyle := lipgloss.NewStyle().
		Foreground(t.Subtle).
		Italic(true)

	var b strings.Builder

	header := fmt.Sprintf("#%s  %s", m.room.Slug, lipgloss.NewStyle().Foreground(t.Subtle).Render(m.room.Name))
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	b.WriteString(inputBoxStyle.Render(m.input.View()))
	b.WriteString("\n")

	var statusParts []string
	if m.err != "" {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(t.Error).Render(m.err))
	}
	statusParts = append(statusParts, statusStyle.Render("esc: leave  enter: send"))
	b.WriteString(strings.Join(statusParts, "  "))

	return b.String()
}

func (m Model) sendMessage() (Model, tea.Cmd) {
	content := strings.TrimSpace(m.input.Value())
	if content == "" || m.wsClient == nil {
		return m, nil
	}

	m.input.SetValue("")

	if err := m.wsClient.SendRoomMessage(m.room.ID, content); err != nil {
		m.err = err.Error()
	}

	return m, nil
}

func (m Model) handleWSMessage(msg ws.IncomingMsg) (Model, tea.Cmd) {
	switch msg.Message.Type {
	case ws.TypeRoomMessage:
		var payload ws.RoomMessagePayload
		if err := json.Unmarshal(msg.Message.Payload, &payload); err != nil {
			m.logger.Error("failed to unmarshal room message", "error", err)
			return m, nil
		}

		m.messages = append(m.messages, chatMessage{
			senderID:       payload.SenderID,
			senderUsername: payload.SenderUsername,
			content:        payload.Content,
			timestamp:      msg.Message.Timestamp.Format("15:04"),
		})
		m.updateViewport()

	case ws.TypeError:
		var payload ws.ErrorPayload
		if err := json.Unmarshal(msg.Message.Payload, &payload); err == nil {
			m.err = payload.Message
		}
	}

	return m, nil
}

func (m *Model) updateViewport() {
	t := theme.Current
	ownStyle := lipgloss.NewStyle().Foreground(t.OwnMsg).Bold(true)
	otherStyle := lipgloss.NewStyle().Foreground(t.OtherMsg).Bold(true)
	timeStyle := lipgloss.NewStyle().Foreground(t.Subtle)
	contentStyle := lipgloss.NewStyle().Foreground(t.Text)

	var lines []string
	for _, msg := range m.messages {
		ts := timeStyle.Render(fmt.Sprintf("[%s]", msg.timestamp))
		var name string
		if msg.senderID == m.userID {
			name = ownStyle.Render("you")
		} else {
			name = otherStyle.Render(msg.senderUsername)
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", ts, name, contentStyle.Render(msg.content)))
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
	m.viewport.GotoBottom()
}

func (m Model) loadHistory() tea.Cmd {
	return func() tea.Msg {
		messages, err := m.apiClient.GetMessages(m.room.ID, 50)
		if err != nil {
			return ws.ErrorMsg{Err: err}
		}
		return historyLoadedMsg{messages: messages}
	}
}
