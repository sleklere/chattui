// Package dmchat provides the DM chat UI model.
package dmchat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sleklere/realtime-chat/cmd/client/internal/api"
	"github.com/sleklere/realtime-chat/cmd/client/internal/ui/theme"
	"github.com/sleklere/realtime-chat/cmd/client/internal/ws"
)

// LeaveDMMsg signals that the user wants to go back to the DM list.
type LeaveDMMsg struct{}

type historyLoadedMsg struct {
	messages []api.MessageResponse
}

type wsConnectedMsg struct {
	client *ws.Client
}

type dmMessage struct {
	senderID       int64
	senderUsername string
	content        string
	timestamp      string
}

// Model is the Bubble Tea model for the DM chat screen.
type Model struct {
	apiClient *api.Client
	wsClient  *ws.Client
	program   *tea.Program
	logger    *slog.Logger

	conversationID int64 // 0 if conversation not yet created
	peerID         int64
	peerUsername   string
	myUserID       int64
	myUsername     string
	wsURL          string
	token          string

	viewport viewport.Model
	input    textinput.Model
	messages []dmMessage
	err      string
	width    int
	height   int
}

// New creates a new DM chat Model.
func New(
	apiClient *api.Client,
	program *tea.Program,
	logger *slog.Logger,
	conversationID, peerID int64,
	peerUsername string,
	myUserID int64,
	myUsername, wsURL, token string,
	width, height int,
) Model {
	vp := viewport.New(width, height-4)

	input := textinput.New()
	input.Placeholder = "type a message..."
	input.Focus()
	input.CharLimit = 500
	input.Width = width - 6

	return Model{
		apiClient:      apiClient,
		program:        program,
		logger:         logger,
		conversationID: conversationID,
		peerID:         peerID,
		peerUsername:   peerUsername,
		myUserID:       myUserID,
		myUsername:     myUsername,
		wsURL:          wsURL,
		token:          token,
		viewport:       vp,
		input:          input,
		width:          width,
		height:         height,
	}
}

// Init initializes the DM chat model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.connectWS(), textinput.Blink}
	if m.conversationID != 0 {
		cmds = append(cmds, m.loadHistory())
	}
	return tea.Batch(cmds...)
}

// Update handles messages for the DM chat model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.cleanup()
			return m, func() tea.Msg { return LeaveDMMsg{} }
		case "enter":
			return m.sendMessage()
		}

	case historyLoadedMsg:
		for i := len(msg.messages) - 1; i >= 0; i-- {
			m2 := msg.messages[i]
			senderUsername := m.peerUsername
			if m2.SenderID == m.myUserID {
				senderUsername = m.myUsername
			}
			m.messages = append(m.messages, dmMessage{
				senderID:       m2.SenderID,
				senderUsername: senderUsername,
				content:        m2.Body,
				timestamp:      m2.CreatedAt.Format("15:04"),
			})
		}
		m.updateViewport()
		return m, nil

	case wsConnectedMsg:
		m.wsClient = msg.client
		m.logger.Info("ws connected for dm", "peer_id", m.peerID)
		return m, nil

	case ws.IncomingMsg:
		return m.handleWSMessage(msg)

	case ws.ErrorMsg:
		m.err = msg.Err.Error()
		m.logger.Error("ws error in dm chat", "error", msg.Err)
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

// View renders the DM chat model.
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

	header := fmt.Sprintf("@ %s", m.peerUsername)
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
	statusParts = append(statusParts, statusStyle.Render("esc: back  enter: send"))
	b.WriteString(strings.Join(statusParts, "  "))

	return b.String()
}

func (m Model) sendMessage() (Model, tea.Cmd) {
	content := strings.TrimSpace(m.input.Value())
	if content == "" || m.wsClient == nil {
		return m, nil
	}

	m.input.SetValue("")

	if err := m.wsClient.SendDirectMessage(m.peerID, content); err != nil {
		m.err = err.Error()
	}

	return m, nil
}

func (m Model) handleWSMessage(msg ws.IncomingMsg) (Model, tea.Cmd) {
	switch msg.Message.Type {
	case ws.TypeDirectMessage:
		var payload ws.DirectMessagePayload
		if err := json.Unmarshal(msg.Message.Payload, &payload); err != nil {
			m.logger.Error("failed to unmarshal dm", "error", err)
			return m, nil
		}

		// only show messages for this conversation
		isFromPeer := payload.FromUserID == m.peerID
		isFromMe := payload.FromUserID == m.myUserID && payload.ToUserID == m.peerID
		if !isFromPeer && !isFromMe {
			return m, nil
		}

		// update conversationID from first message if not set
		if m.conversationID == 0 && payload.ConversationID != 0 {
			m.conversationID = payload.ConversationID
		}

		senderUsername := payload.FromUsername
		if payload.FromUserID == m.myUserID {
			senderUsername = m.myUsername
		}

		m.messages = append(m.messages, dmMessage{
			senderID:       payload.FromUserID,
			senderUsername: senderUsername,
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
		if msg.senderID == m.myUserID {
			name = ownStyle.Render("you")
		} else {
			name = otherStyle.Render(msg.senderUsername)
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", ts, name, contentStyle.Render(msg.content)))
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
	m.viewport.GotoBottom()
}

func (m *Model) cleanup() {
	if m.wsClient != nil {
		m.wsClient.Close()
		m.wsClient = nil
	}
}

func (m Model) loadHistory() tea.Cmd {
	return func() tea.Msg {
		msgs, err := m.apiClient.GetConversationMessages(m.conversationID, 50)
		if err != nil {
			return ws.ErrorMsg{Err: err}
		}
		return historyLoadedMsg{messages: msgs}
	}
}

func (m Model) connectWS() tea.Cmd {
	return func() tea.Msg {
		client, err := ws.Connect(context.Background(), m.wsURL, m.token, m.program, m.logger)
		if err != nil {
			return ws.ErrorMsg{Err: err}
		}
		return wsConnectedMsg{client: client}
	}
}
