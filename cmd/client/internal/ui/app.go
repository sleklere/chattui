// Package ui provides the terminal user interface for the chat client.
package ui

import (
	"context"
	"encoding/json"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sleklere/chattui/cmd/client/internal/api"
	"github.com/sleklere/chattui/cmd/client/internal/config"
	"github.com/sleklere/chattui/cmd/client/internal/ui/auth"
	"github.com/sleklere/chattui/cmd/client/internal/ui/chat"
	"github.com/sleklere/chattui/cmd/client/internal/ui/dm"
	"github.com/sleklere/chattui/cmd/client/internal/ui/dmchat"
	"github.com/sleklere/chattui/cmd/client/internal/ui/hud"
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
	RoomBadges  map[int64]int64 // roomID → unread
	ConvBadges  map[int64]int64 // convID → unread
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
	// The connection indicator lives in the shared frame, so keep it in sync
	// here without consuming the message: screens still handle their own errors.
	if _, ok := msg.(ws.ErrorMsg); ok {
		hud.SetStatus(hud.StatusOffline)
	}

	// Intercept inbox_updated WS events to keep badges in sync regardless of active screen.
	if wsMsg, ok := msg.(ws.IncomingMsg); ok && wsMsg.Message.Type == ws.TypeInboxUpdated {
		var payload ws.InboxUpdatedPayload
		if err := json.Unmarshal(wsMsg.Message.Payload, &payload); err == nil {
			if a.state.RoomBadges == nil {
				a.state.RoomBadges = make(map[int64]int64)
			}
			if a.state.ConvBadges == nil {
				a.state.ConvBadges = make(map[int64]int64)
			}
			if payload.RefRoomID != nil {
				a.state.RoomBadges[*payload.RefRoomID] = payload.UnreadCount
				if a.active == screenRooms {
					rid, cnt := *payload.RefRoomID, payload.UnreadCount
					total := a.badgeTotal()
					var activeCmd tea.Cmd
					a.rooms, activeCmd = a.rooms.Update(rooms.RoomBadgeUpdateMsg{RoomID: rid, Count: cnt, Total: total})
					return a, activeCmd
				}
			}
			if payload.RefConversationID != nil {
				a.state.ConvBadges[*payload.RefConversationID] = payload.UnreadCount
				if a.active == screenDM {
					cid, cnt := *payload.RefConversationID, payload.UnreadCount
					total := a.badgeTotal()
					var activeCmd tea.Cmd
					a.dm, activeCmd = a.dm.Update(dm.ConvBadgeUpdateMsg{ConvID: cid, Count: cnt, Total: total})
					return a, activeCmd
				}
			}
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		if msg.String() == "esc" && a.active == screenRooms && !a.rooms.HasOverlay() {
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
		hud.SetUsername(msg.Username)
		hud.SetStatus(hud.StatusConnecting)
		a.state.RoomBadges = make(map[int64]int64)
		a.state.ConvBadges = make(map[int64]int64)
		a.active = screenRooms
		a.rooms = rooms.New(a.state.APIClient, a.state.RoomBadges, a.badgeTotal(), a.width, a.height)
		return a, tea.Batch(a.rooms.Init(), a.connectWS(), inbox.FetchBadgesCmd(a.state.APIClient))

	case wsConnectedMsg:
		a.wsClient = msg.client
		a.state.Logger.Info("websocket connected at app level")
		hud.SetStatus(hud.StatusLive)
		return a, nil

	case ws.ReconnectingMsg:
		hud.SetStatus(hud.StatusConnecting)
		return a, nil

	case ws.ConnectedMsg:
		hud.SetStatus(hud.StatusLive)
		return a, nil

	case inbox.BadgesMsg:
		a.state.RoomBadges = msg.RoomBadges
		a.state.ConvBadges = msg.ConvBadges
		total := a.badgeTotal()
		switch a.active {
		case screenRooms:
			var cmd tea.Cmd
			a.rooms, cmd = a.rooms.Update(rooms.RefreshBadgesMsg{Badges: msg.RoomBadges, InboxTotal: total})
			return a, cmd
		case screenDM:
			var cmd tea.Cmd
			a.dm, cmd = a.dm.Update(dm.RefreshBadgesMsg{Badges: msg.ConvBadges, InboxTotal: total})
			return a, cmd
		}
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
		a.rooms = rooms.New(a.state.APIClient, a.state.RoomBadges, a.badgeTotal(), a.width, a.height)
		return a, tea.Batch(a.rooms.Init(), inbox.FetchBadgesCmd(a.state.APIClient))

	case rooms.ShowDMsMsg:
		a.active = screenDM
		a.dm = dm.New(a.state.APIClient, a.state.ConvBadges, a.badgeTotal(), a.width, a.height)
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
		a.rooms = rooms.New(a.state.APIClient, a.state.RoomBadges, a.badgeTotal(), a.width, a.height)
		return a, a.rooms.Init()

	case dmchat.LeaveDMMsg:
		a.active = screenDM
		a.dm = dm.New(a.state.APIClient, a.state.ConvBadges, a.badgeTotal(), a.width, a.height)
		return a, tea.Batch(a.dm.Init(), inbox.FetchBadgesCmd(a.state.APIClient))

	case rooms.ShowInboxMsg, dm.ShowInboxMsg:
		a.active = screenInbox
		a.inbox = inbox.New(a.state.APIClient, a.badgeTotal(), a.width, a.height)
		return a, a.inbox.Init()

	case inbox.LeaveInboxMsg:
		a.active = screenRooms
		a.rooms = rooms.New(a.state.APIClient, a.state.RoomBadges, a.badgeTotal(), a.width, a.height)
		return a, a.rooms.Init()

	case inbox.ShowDMsMsg:
		a.active = screenDM
		a.dm = dm.New(a.state.APIClient, a.state.ConvBadges, a.badgeTotal(), a.width, a.height)
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

func (a *App) badgeTotal() int64 {
	var total int64
	for _, v := range a.state.RoomBadges {
		total += v
	}
	for _, v := range a.state.ConvBadges {
		total += v
	}
	return total
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
