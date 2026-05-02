package ws

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/coder/websocket"
	"github.com/sleklere/realtime-chat/cmd/server/internal/conversation"
	"github.com/sleklere/realtime-chat/cmd/server/internal/room"
)

// NewClient creates a new Client ready to be registered with the Hub.
func NewClient(hub *Hub, conn *websocket.Conn, roomSvc *room.Service, conversationSvc *conversation.Service, userID int64, username string, roomIDs map[int64]bool, logger *slog.Logger) *Client {
	return &Client{
		hub:             hub,
		conn:            conn,
		userID:          userID,
		username:        username,
		roomIDs:         roomIDs,
		logger:          logger,
		send:            make(chan Message, 256),
		roomSvc:         roomSvc,
		conversationSvc: conversationSvc,
	}
}

// ReadPump reads messages from the WebSocket and routes them to the Hub.
func (c *Client) ReadPump(ctx context.Context) {
	defer func() {
		c.hub.unregister <- c
	}()

	for {
		// 1. leer del websocket con c.conn.Read(ctx)
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			c.logger.Warn("error while reading from conn")
			return
		}
		// 2. parsear el envelope (Message)
		var msg Message
		err = json.Unmarshal(data, &msg)
		if err != nil {
			c.logger.Warn("error while unmarshalling ws msg data")
			continue
		}
		// 3. switch msg.Type
		switch msg.Type {
		case TypeRoomMessage:
			c.dispatchRoomMessage(ctx, msg)
		case TypeDirectMessage:
			c.dispatchDirectMessage(ctx, msg)
		case TypeJoinRoom, TypeLeaveRoom:
			c.dispatchUserRoomUpdate(ctx, msg)
		}
	}
}

// WritePump drains the send channel and writes messages to the WebSocket.
func (c *Client) WritePump(ctx context.Context) {
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				c.logger.Debug("(WritePump): channel closed, closing client with userID", "userID", c.userID)
				return
			}
			data, err := json.Marshal(msg)
			if err != nil {
				c.logger.Warn("(WritePump): error while marshalling ws msg data")
				continue
			}
			err = c.conn.Write(ctx, websocket.MessageText, data)
			if err != nil {
				c.logger.Error("(WritePump): error while writing to conn")
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) dispatchRoomMessage(ctx context.Context, msg Message) {
	//    - parsear el RoomMessagePayload del msg.Payload
	var roomMsgPayload RoomMessagePayload
	err := json.Unmarshal(msg.Payload, &roomMsgPayload)
	if err != nil {
		c.logger.Warn("error while unmarshalling room msg payload")
		return
	}
	//    - validar (roomID > 0, content no vacío, que el client sea miembro del room)
	_, clientInRoom := c.roomIDs[roomMsgPayload.RoomID]
	if roomMsgPayload.RoomID == 0 || roomMsgPayload.Content == "" ||
		!clientInRoom {
		c.logger.Warn("failed TypeRoomMessage validation", "room_id", roomMsgPayload.RoomID)
		return
	}
	//    - persist to DB and broadcast
	roomMsgPayload.SenderID = c.userID
	roomMsgPayload.SenderUsername = c.username

	dbMsg, err := c.roomSvc.SendRoomMessage(
		ctx,
		roomMsgPayload.RoomID,
		roomMsgPayload.SenderID,
		roomMsgPayload.Content,
	)
	if err != nil {
		c.logger.Warn("failed to persist room message", "error", err)
	} else {
		roomMsgPayload.MessageID = dbMsg.ID
	}

	completePayload, err := json.Marshal(roomMsgPayload)
	if err != nil {
		c.logger.Warn("error while marshalling complete msg payload")
		return
	}
	msgWithCompletePayload := Message{Type: msg.Type, Payload: completePayload, Timestamp: msg.Timestamp}

	broadcastMsg := BroadcastMsg{msg: msgWithCompletePayload, targetRoomID: roomMsgPayload.RoomID}
	c.hub.broadcast <- broadcastMsg
}

func (c *Client) dispatchDirectMessage(ctx context.Context, msg Message) {
	var directMsgPayload DirectMessagePayload
	err := json.Unmarshal(msg.Payload, &directMsgPayload)
	if err != nil {
		c.logger.Warn("error while unmarshalling direct msg payload")
		return
	}

	if directMsgPayload.ToUserID == 0 || directMsgPayload.Content == "" {
		c.logger.Warn("failed TypeRoomMessage validation", "room_id", directMsgPayload.ConversationID)
		return
	}

	directMsgPayload.SenderID = c.userID
	directMsgPayload.SenderUsername = c.username

	dbMsg, err := c.conversationSvc.SendDirectMessage(
		ctx,
		directMsgPayload.SenderID,
		directMsgPayload.ToUserID,
		directMsgPayload.Content,
	)
	if err != nil {
		c.logger.Warn("failed to persist dm message", "error", err)
	}

	directMsgPayload.MessageID = dbMsg.ID

	completePayload, err := json.Marshal(directMsgPayload)
	if err != nil {
		c.logger.Warn("error while marshalling complete msg payload")
		return
	}
	msgWithCompletePayload := Message{Type: msg.Type, Payload: completePayload, Timestamp: msg.Timestamp}

	broadcastMsg := BroadcastMsg{msg: msgWithCompletePayload, targetUserIDs: []int64{c.userID, directMsgPayload.ToUserID}}
	c.hub.broadcast <- broadcastMsg
}

func (c *Client) dispatchUserRoomUpdate(_ context.Context, msg Message) {

	var roomPresencePayload RoomPresencePayload
	err := json.Unmarshal(msg.Payload, &roomPresencePayload)
	if err != nil {
		c.logger.Warn("error while unmarshalling JoinRoomPayload")
		return
	}

	if roomPresencePayload.RoomID <= 0 {
		c.logger.Warn("invalid room ID")
		return
	}

	c.hub.updateUserPresenceInRoom(UserRoomPresent{userID: c.userID, roomID: roomPresencePayload.RoomID, present: msg.Type == TypeJoinRoom})
}
