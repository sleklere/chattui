// Package ui provides the terminal user interface for the chat client.
package ui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/config"
	"github.com/sleklere/chattui/cmd/client/internal/ui/auth"
	"github.com/sleklere/chattui/cmd/client/internal/ui/chat"
	"github.com/sleklere/chattui/cmd/client/internal/ui/dm"
	"github.com/sleklere/chattui/cmd/client/internal/ui/dmchat"
	"github.com/sleklere/chattui/cmd/client/internal/ui/inbox"
	"github.com/sleklere/chattui/cmd/client/internal/ui/rooms"
	"github.com/sleklere/chattui/cmd/client/internal/ws"
	"github.com/sleklere/chattui/pkg/dto"
)

type wsConnectedMsg struct {
	client *ws.Client
}

type screen int

const (
	screenAuth screen = iota
	screenRooms
	screenChat
	screenDM
	screenDMChat
	screenInbox
)

// AppState holds shared state across UI screens.
type AppState struct {
	Config      config.Config
	Logger      *slog.Logger
	APIClient   *api.Client
	Token       string
	UserID      int64
	Username    string
	CurrentRoom *dto.Room
}

// App is the top-level Bubble Tea model that manages screen transitions.
type App struct {
	state    *AppState
	program  *tea.Program
	wsClient *ws.Client
	active   screen
	width    int
	height   int

	auth   auth.Model
	rooms  rooms.Model
	chat   chat.Model
	dm     dm.Model
	dmChat dmchat.Model
	inbox  inbox.Model
}

// NewApp creates a new App with the given configuration and logger.
func NewApp(cfg config.Config, logger *slog.Logger) App {
	apiClient := api.New(cfg.ServerURL, logger)

	return App{
		state:  &AppState{Config: cfg, Logger: logger, APIClient: apiClient},
		active: screenAuth,
		auth:   auth.New(apiClient),
	}
}

// SetProgram sets the Bubble Tea program reference used for async messaging.
func (a *App) SetProgram(p *tea.Program) {
	a.program = p
}

// Init initializes the app model.
func (a *App) Init() tea.Cmd {
	return a.auth.Init()
}

// Update handles messages for the app model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		if msg.String() == "esc" && a.active == screenRooms {
			return a, tea.Quit
		}

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case auth.SuccessMsg:
		a.state.Token = msg.Token
		a.state.UserID = msg.UserID
		a.state.Username = msg.Username
		a.state.APIClient.SetToken(msg.Token)
		a.state.Logger.Info("authenticated", "user_id", msg.UserID, "username", msg.Username)
		a.active = screenRooms
		a.rooms = rooms.New(a.state.APIClient, a.width, a.height)
		return a, tea.Batch(a.rooms.Init(), a.connectWS())

	case wsConnectedMsg:
		a.wsClient = msg.client
		a.state.Logger.Info("websocket connected at app level")
		return a, nil

	case rooms.RoomSelectedMsg:
		a.state.CurrentRoom = &msg.Room
		a.state.Logger.Info("room selected", "room_id", msg.Room.ID, "room_name", msg.Room.Name)
		a.active = screenChat
		a.chat = chat.New(
			a.state.APIClient,
			a.wsClient,
			a.state.Logger,
			msg.Room,
			a.state.UserID,
			a.state.Username,
			a.width,
			a.height,
		)
		return a, a.chat.Init()

	case chat.LeaveRoomMsg:
		a.state.Logger.Info("left room", "room_id", a.state.CurrentRoom.ID)
		a.state.CurrentRoom = nil
		a.active = screenRooms
		a.rooms = rooms.New(a.state.APIClient, a.width, a.height)
		return a, a.rooms.Init()

	case rooms.ShowDMsMsg:
		a.active = screenDM
		a.dm = dm.New(a.state.APIClient, a.width, a.height)
		return a, a.dm.Init()

	case dm.ConvSelectedMsg:
		a.active = screenDMChat
		a.dmChat = dmchat.New(
			a.state.APIClient,
			a.wsClient,
			a.state.Logger,
			msg.Conv.ID,
			msg.Conv.PeerID,
			msg.Conv.PeerUsername,
			a.state.UserID,
			a.state.Username,
			a.width,
			a.height,
		)
		return a, a.dmChat.Init()

	case dm.NewDMMsg:
		a.active = screenDMChat
		a.dmChat = dmchat.New(
			a.state.APIClient,
			a.wsClient,
			a.state.Logger,
			0, // no conversation yet
			msg.PeerID,
			msg.PeerUsername,
			a.state.UserID,
			a.state.Username,
			a.width,
			a.height,
		)
		return a, a.dmChat.Init()

	case dm.LeaveDMListMsg:
		a.active = screenRooms
		a.rooms = rooms.New(a.state.APIClient, a.width, a.height)
		return a, a.rooms.Init()

	case dmchat.LeaveDMMsg:
		a.active = screenDM
		a.dm = dm.New(a.state.APIClient, a.width, a.height)
		return a, a.dm.Init()

	case rooms.ShowInboxMsg, dm.ShowInboxMsg:
		a.active = screenInbox
		a.inbox = inbox.New(a.state.APIClient, a.width, a.height)
		return a, a.inbox.Init()

	case inbox.LeaveInboxMsg:
		a.active = screenRooms
		a.rooms = rooms.New(a.state.APIClient, a.width, a.height)
		return a, a.rooms.Init()

	case inbox.ShowDMsMsg:
		a.active = screenDM
		a.dm = dm.New(a.state.APIClient, a.width, a.height)
		return a, a.dm.Init()

	case inbox.OpenRoomMsg:
		room := dto.Room{ID: msg.RoomID, Name: msg.RoomName}
		a.state.CurrentRoom = &room
		a.active = screenChat
		a.chat = chat.New(
			a.state.APIClient,
			a.wsClient,
			a.state.Logger,
			room,
			a.state.UserID,
			a.state.Username,
			a.width,
			a.height,
		)
		return a, a.chat.Init()

	case inbox.OpenDMMsg:
		a.active = screenDMChat
		a.dmChat = dmchat.New(
			a.state.APIClient,
			a.wsClient,
			a.state.Logger,
			msg.ConversationID,
			msg.PeerID,
			msg.PeerUsername,
			a.state.UserID,
			a.state.Username,
			a.width,
			a.height,
		)
		return a, a.dmChat.Init()
	}

	switch a.active {
	case screenAuth:
		var cmd tea.Cmd
		a.auth, cmd = a.auth.Update(msg)
		return a, cmd
	case screenRooms:
		var cmd tea.Cmd
		a.rooms, cmd = a.rooms.Update(msg)
		return a, cmd
	case screenChat:
		var cmd tea.Cmd
		a.chat, cmd = a.chat.Update(msg)
		return a, cmd
	case screenDM:
		var cmd tea.Cmd
		a.dm, cmd = a.dm.Update(msg)
		return a, cmd
	case screenDMChat:
		var cmd tea.Cmd
		a.dmChat, cmd = a.dmChat.Update(msg)
		return a, cmd
	case screenInbox:
		var cmd tea.Cmd
		a.inbox, cmd = a.inbox.Update(msg)
		return a, cmd
	}

	return a, nil
}

// View renders the active screen.
func (a *App) View() string {
	switch a.active {
	case screenAuth:
		return a.auth.View()
	case screenRooms:
		return a.rooms.View()
	case screenChat:
		return a.chat.View()
	case screenDM:
		return a.dm.View()
	case screenDMChat:
		return a.dmChat.View()
	case screenInbox:
		return a.inbox.View()
	}
	return ""
}

func (a *App) connectWS() tea.Cmd {
	return func() tea.Msg {
		client, err := ws.Connect(context.Background(), a.state.Config.WSURL, a.state.Token, a.program, a.state.Logger)
		if err != nil {
			return ws.ErrorMsg{Err: err}
		}
		return wsConnectedMsg{client: client}
	}
}
