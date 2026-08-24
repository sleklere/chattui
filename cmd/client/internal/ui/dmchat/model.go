// Package dmchat provides the DM chat UI model.
package dmchat

import (
	"encoding/json"
	"log/slog"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/ui/chatview"
	"github.com/sleklere/chattui/cmd/client/internal/ui/components"
	"github.com/sleklere/chattui/cmd/client/internal/ui/hud"
	"github.com/sleklere/chattui/cmd/client/internal/ws"
	"github.com/sleklere/chattui/pkg/dto"
)

// LeaveDMMsg signals that the user wants to go back to the DM list.
type LeaveDMMsg struct{}

type historyLoadedMsg struct {
	messages []dto.Message
}

// Model is the Bubble Tea model for the DM chat screen.
type Model struct {
	apiClient *api.Client
	wsClient  *ws.Client
	logger    *slog.Logger

	conversationID int64 // 0 if conversation not yet created
	peerID         int64
	peerUsername   string
	myUserID       int64
	myUsername     string

	viewport viewport.Model
	input    textinput.Model
	messages []chatview.Message
	loading  bool
	err      string
	width    int
	height   int
}

// New creates a new DM chat Model.
func New(
	apiClient *api.Client,
	wsClient *ws.Client,
	logger *slog.Logger,
	conversationID, peerID int64,
	peerUsername string,
	myUserID int64,
	myUsername string,
	width, height int,
) Model {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(historyHeight(height)))

	input := textinput.New()
	input.Placeholder = "message @" + peerUsername
	input.Prompt = ""
	input.Focus()
	input.CharLimit = 500
	input.SetWidth(width - 8)
	input.SetStyles(components.InputStyles())

	return Model{
		apiClient:      apiClient,
		wsClient:       wsClient,
		logger:         logger,
		conversationID: conversationID,
		peerID:         peerID,
		peerUsername:   peerUsername,
		myUserID:       myUserID,
		myUsername:     myUsername,
		viewport:       vp,
		input:          input,
		loading:        conversationID != 0,
		width:          width,
		height:         height,
	}
}

// historyHeight is the room left for the message history once the frame and
// the composer box are subtracted from the terminal height.
func historyHeight(height int) int {
	h := hud.BodyHeight(height) - chatview.ComposerHeight
	if h < 1 {
		return 1
	}
	return h
}

// Init initializes the DM chat model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.conversationID != 0 {
		cmds = append(cmds, m.loadHistory())
		convID := m.conversationID
		cmds = append(cmds, func() tea.Msg {
			_ = m.apiClient.MarkAsRead(&convID, nil, false)
			return nil
		})
	}
	return tea.Batch(cmds...)
}

// Update handles messages for the DM chat model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return LeaveDMMsg{} }
		case "enter":
			return m.sendMessage()
		}
		// Only scroll keys reach the viewport; everything else is typing.
		if isScrollKey(msg.String()) {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case historyLoadedMsg:
		m.loading = false
		for i := len(msg.messages) - 1; i >= 0; i-- {
			m2 := msg.messages[i]
			senderUsername := m.peerUsername
			if m2.SenderID == m.myUserID {
				senderUsername = m.myUsername
			}
			m.messages = append(m.messages, chatview.Message{
				SenderID: m2.SenderID,
				Sender:   senderUsername,
				Body:     m2.Body,
				At:       m2.CreatedAt,
			})
		}
		m.updateViewport()
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
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(historyHeight(msg.Height))
		m.input.SetWidth(msg.Width - 8)
		m.updateViewport()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// isScrollKey reports whether a key belongs to the message history rather than the
// composer, which holds focus while chatting.
func isScrollKey(key string) bool {
	switch key {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return true
	}
	return false
}

// View renders the DM chat model.
func (m Model) View() string {
	frame := hud.Frame{
		Width:    m.width,
		Height:   m.height,
		Title:    "@ " + m.peerUsername,
		Subtitle: "direct message",
		Keys: []hud.Key{
			{Key: "↵", Label: "send"},
			{Key: "pgup/pgdn", Label: "scroll"},
			{Key: "esc", Label: "back"},
		},
		Err: m.err,
	}

	body := m.viewport.View()
	if m.loading {
		body = components.Empty(m.width, m.viewport.Height(), "loading history…", "")
	} else if len(m.messages) == 0 {
		body = components.Empty(m.width, m.viewport.Height(), "No messages yet", "this is the beginning of your conversation with "+m.peerUsername)
	}

	return frame.Render(body + "\n" + chatview.Composer(m.input, m.width))
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

		m.messages = append(m.messages, chatview.Message{
			SenderID: payload.FromUserID,
			Sender:   senderUsername,
			Body:     payload.Content,
			At:       msg.Message.Timestamp,
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
	m.viewport.SetContent(chatview.Render(m.messages, m.myUserID, m.width))
	m.viewport.GotoBottom()
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
