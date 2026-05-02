package ws

import (
	"log/slog"

	"github.com/coder/websocket"
	"github.com/sleklere/realtime-chat/cmd/server/internal/bus"
	"github.com/sleklere/realtime-chat/cmd/server/internal/conversation"
	"github.com/sleklere/realtime-chat/cmd/server/internal/room"
)

// Client represents a single WebSocket connection.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	userID   int64
	username string
	roomIDs  map[int64]bool // rooms this client is a member of
	send     chan Message   // Hub writes here, WritePump drains

	logger          *slog.Logger
	roomSvc         *room.Service
	conversationSvc *conversation.Service
}

// Hub is the central message broker.
// A single goroutine runs Hub.Run() and is the sole owner of the maps.
type Hub struct {
	clients map[int64]*Client        // userID → client
	rooms   map[int64]map[int64]bool // roomID → set of userIDs online

	register       chan *Client
	unregister     chan *Client
	broadcast      chan BroadcastMsg
	userRoomUpdate chan UserRoomPresent

	bus bus.Bus
}

// BroadcastMsg wraps a message with routing info.
type BroadcastMsg struct {
	msg           Message
	targetRoomID  int64   // if > 0, route to room members
	targetUserIDs []int64 // if targetRoomID == 0, route to these users (DM)
}

// UserRoomPresent represents a user joining or leaving a room.
type UserRoomPresent struct {
	userID  int64
	roomID  int64
	present bool
}

// NewHub creates a Hub with initialized maps and channels.
func NewHub(b bus.Bus) *Hub {
	return &Hub{
		clients:        make(map[int64]*Client),
		rooms:          make(map[int64]map[int64]bool),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		broadcast:      make(chan BroadcastMsg, 256),
		userRoomUpdate: make(chan UserRoomPresent),
		bus:            b,
	}
}

// Run starts the Hub event loop. Must be called in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)
		case client := <-h.unregister:
			h.unregisterClient(client)
		case userRoomUpdate := <-h.userRoomUpdate:
			h.updateUserPresenceInRoom(userRoomUpdate)
		case broadcastMsg := <-h.broadcast:
			h.broadcastMessage(broadcastMsg)
		}
	}
}

func (h *Hub) registerClient(c *Client) {
	for roomID, isInRoom := range c.roomIDs {
		if _, ok := h.rooms[roomID]; !ok {
			h.rooms[roomID] = make(map[int64]bool)
		}
		h.rooms[roomID][c.userID] = isInRoom
	}
	h.clients[c.userID] = c
}

func (h *Hub) unregisterClient(c *Client) {
	for roomID := range c.roomIDs {
		_, ok := h.rooms[roomID]
		if !ok {
			continue
		}
		delete(h.rooms[roomID], c.userID)
	}
	delete(h.clients, c.userID)
	close(c.send)
}

func (h *Hub) updateUserPresenceInRoom(userRoomUpdate UserRoomPresent) {
	client, ok := h.clients[userRoomUpdate.userID]
	if !ok {
		return
	}
	if userRoomUpdate.present {
		if _, ok := h.rooms[userRoomUpdate.roomID]; !ok {
			h.rooms[userRoomUpdate.roomID] = make(map[int64]bool)
		}
		h.rooms[userRoomUpdate.roomID][userRoomUpdate.userID] = true
		client.roomIDs[userRoomUpdate.roomID] = true
	} else {
		if _, ok := h.rooms[userRoomUpdate.roomID]; !ok {
			return
		}
		delete(h.rooms[userRoomUpdate.roomID], userRoomUpdate.userID)
		delete(client.roomIDs, userRoomUpdate.roomID)
	}
}

func (h *Hub) broadcastMessage(broadcastMsg BroadcastMsg) {
	// if it's a room msg
	if broadcastMsg.targetRoomID > 0 {
		for userID, isConnected := range h.rooms[broadcastMsg.targetRoomID] {
			if !isConnected {
				continue
			}
			c := h.clients[userID]
			select {
			case c.send <- broadcastMsg.msg:
			default:
				// client laggeado, lo sacamos
				h.kickClient(c)
			}
		}
	} else {
		for _, userID := range broadcastMsg.targetUserIDs {
			// if the client exists
			if c, ok := h.clients[userID]; ok {
				select {
				case c.send <- broadcastMsg.msg:
				default:
					// client laggeado, lo sacamos
					h.kickClient(c)
				}

			}
		}
	}
}

func (h *Hub) kickClient(c *Client) {
	close(c.send)
	delete(h.clients, c.userID)
	for roomID := range c.roomIDs {
		_, ok := h.rooms[roomID]
		if !ok {
			continue
		}
		delete(h.rooms[roomID], c.userID)
	}
}

// Register sends a client to the Hub's register channel.
func (h *Hub) Register(c *Client) {
	h.register <- c
}

// UpdateUserRoomState notifies the Hub that a user joined or left a room.
func (h *Hub) UpdateUserRoomState(roomID int64, userID int64, present bool) {
	h.userRoomUpdate <- UserRoomPresent{
		roomID:  roomID,
		userID:  userID,
		present: present,
	}
}
