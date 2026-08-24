// Package chat provides the chat room UI model for the chat client.
package chat

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

// LeaveRoomMsg signals that the user wants to leave the current room.
type LeaveRoomMsg struct{}

type historyLoadedMsg struct {
	messages []dto.Message
}

// Model is the Bubble Tea model for the chat room screen.
type Model struct {
	apiClient *api.Client
	wsClient  *ws.Client
	logger    *slog.Logger

	room     dto.Room
	userID   int64
	username string

	viewport viewport.Model
	input    textinput.Model
	messages []chatview.Message
	loading  bool
	err      string
	width    int
	height   int
}

// New creates a new chat Model for the given room.
func New(
	apiClient *api.Client,
	wsClient *ws.Client,
	logger *slog.Logger,
	room dto.Room,
	userID int64,
	username string,
	width, height int,
) Model {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(historyHeight(height)))

	input := textinput.New()
	input.Placeholder = "message #" + room.Slug
	input.Prompt = ""
	input.Focus()
	input.CharLimit = 500
	input.SetWidth(width - 8)
	input.SetStyles(components.InputStyles())

	return Model{
		apiClient: apiClient,
		wsClient:  wsClient,
		logger:    logger,
		room:      room,
		userID:    userID,
		username:  username,
		viewport:  vp,
		input:     input,
		loading:   true,
		width:     width,
		height:    height,
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

// Init initializes the chat model.
func (m Model) Init() tea.Cmd {
	roomID := m.room.ID
	return tea.Batch(
		m.loadHistory(),
		textinput.Blink,
		func() tea.Msg {
			_ = m.apiClient.MarkAsRead(nil, &roomID, false)
			return nil
		},
	)
}

// Update handles messages for the chat model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return LeaveRoomMsg{} }
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
		// Messages come from the API in DESC order (newest first), reverse for display.
		for i := len(msg.messages) - 1; i >= 0; i-- {
			m2 := msg.messages[i]
			m.messages = append(m.messages, chatview.Message{
				SenderID: m2.SenderID,
				Sender:   m2.SenderUsername,
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
		m.logger.Error("ws error in chat", "error", msg.Err)
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

// View renders the chat model.
func (m Model) View() string {
	frame := hud.Frame{
		Width:    m.width,
		Height:   m.height,
		Title:    "# " + m.room.Slug,
		Subtitle: m.room.Name,
		Keys: []hud.Key{
			{Key: "↵", Label: "send"},
			{Key: "pgup/pgdn", Label: "scroll"},
			{Key: "esc", Label: "leave"},
		},
		Err: m.err,
	}

	body := m.viewport.View()
	if m.loading {
		body = components.Empty(m.width, m.viewport.Height(), "loading history…", "")
	} else if len(m.messages) == 0 {
		body = components.Empty(m.width, m.viewport.Height(), "No messages yet", "say something to start the conversation")
	}

	return frame.Render(body + "\n" + chatview.Composer(m.input, m.width))
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

		m.messages = append(m.messages, chatview.Message{
			SenderID: payload.SenderID,
			Sender:   payload.SenderUsername,
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
	m.viewport.SetContent(chatview.Render(m.messages, m.userID, m.width))
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
