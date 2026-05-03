package ws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
)

// failingRoomSvc implements roomSender and always returns an error.
type failingRoomSvc struct{}

func (failingRoomSvc) SendRoomMessage(_ context.Context, _, _ int64, _ string) (dbstore.Message, error) {
	return dbstore.Message{}, errors.New("db error")
}

// failingDMSvc implements dmSender and always returns an error.
type failingDMSvc struct{}

func (failingDMSvc) SendDirectMessage(_ context.Context, _, _ int64, _ string) (dbstore.Message, error) {
	return dbstore.Message{}, errors.New("db error")
}

// ---------------------------------------------------------------------------
// dispatchRoomMessage validation — no DB needed (returns before CreateMessage)
// ---------------------------------------------------------------------------

// Test 13 – room_id zero: no broadcast
func TestDispatchRoomMessage_InvalidRoomID(t *testing.T) {
	h := startHub(t)
	c := newTestClient(h, 1, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, c, sync)

	payload, _ := json.Marshal(RoomMessagePayload{RoomID: 0, Content: "hi"})
	c.dispatchRoomMessage(context.Background(), Message{Type: TypeRoomMessage, Payload: payload})
	syncHub(t, h, sync)

	expectNoMessage(t, c.send)
}

// Test 14 – empty content: no broadcast
func TestDispatchRoomMessage_EmptyContent(t *testing.T) {
	h := startHub(t)
	c := newTestClient(h, 1, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, c, sync)

	payload, _ := json.Marshal(RoomMessagePayload{RoomID: 10, Content: ""})
	c.dispatchRoomMessage(context.Background(), Message{Type: TypeRoomMessage, Payload: payload})
	syncHub(t, h, sync)

	expectNoMessage(t, c.send)
}

// Test 15 – client not in room: no broadcast
func TestDispatchRoomMessage_ClientNotInRoom(t *testing.T) {
	h := startHub(t)
	c := newTestClient(h, 1, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, c, sync)

	payload, _ := json.Marshal(RoomMessagePayload{RoomID: 99, Content: "hi"})
	c.dispatchRoomMessage(context.Background(), Message{Type: TypeRoomMessage, Payload: payload})
	syncHub(t, h, sync)

	expectNoMessage(t, c.send)
}

// Test 16 – malformed payload: no panic, no broadcast
func TestDispatchRoomMessage_MalformedPayload(t *testing.T) {
	h := startHub(t)
	c := newTestClient(h, 1, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, c, sync)

	c.dispatchRoomMessage(context.Background(), Message{Type: TypeRoomMessage, Payload: json.RawMessage(`not json`)})
	syncHub(t, h, sync)

	expectNoMessage(t, c.send)
}

// Test 17 – persistence fails: message must NOT be broadcast to the room.
func TestDispatchRoomMessage_PersistenceFails_NoBroadcast(t *testing.T) {
	h := startHub(t)
	sender := newTestClient(h, 1, map[int64]bool{10: true})
	other := newTestClient(h, 2, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	sender.roomSvc = failingRoomSvc{}
	registerAll(t, h, sender, other, sync)

	payload, _ := json.Marshal(RoomMessagePayload{RoomID: 10, Content: "hello"})
	sender.dispatchRoomMessage(context.Background(), Message{Type: TypeRoomMessage, Payload: payload})
	syncHub(t, h, sync)

	expectNoMessage(t, sender.send)
	expectNoMessage(t, other.send)
}

// Test 18 – DM persistence fails: message must NOT be broadcast.
func TestDispatchDirectMessage_PersistenceFails_NoBroadcast(t *testing.T) {
	h := startHub(t)
	sender := newTestClient(h, 1, map[int64]bool{})
	recipient := newTestClient(h, 2, map[int64]bool{})
	sync := newTestClient(h, 99, map[int64]bool{})
	sender.conversationSvc = failingDMSvc{}
	registerAll(t, h, sender, recipient, sync)

	payload, _ := json.Marshal(DirectMessagePayload{ToUserID: 2, Content: "hey"})
	sender.dispatchDirectMessage(context.Background(), Message{Type: TypeDirectMessage, Payload: payload})
	syncHub(t, h, sync)

	expectNoMessage(t, sender.send)
	expectNoMessage(t, recipient.send)
}
