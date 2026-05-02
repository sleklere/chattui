package ws

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sleklere/realtime-chat/cmd/server/internal/bus"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var nopLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// newTestClient creates a Client with no WebSocket conn and no DB queries,
// suitable for unit-testing Hub routing logic.
func newTestClient(h *Hub, userID int64, roomIDs map[int64]bool) *Client {
	return &Client{
		hub:      h,
		userID:   userID,
		username: fmt.Sprintf("user_%d", userID),
		roomIDs:  roomIDs,
		send:     make(chan Message, 256),
		logger:   nopLogger,
	}
}

// expectMessage reads one message from ch or fails after 1 s.
func expectMessage(t *testing.T, ch <-chan Message) Message {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(time.Second):
		t.Fatal("expected a message but timed out")
		return Message{}
	}
}

// expectNoMessage asserts that no message arrives within 50 ms.
// Call syncHub first so the Hub has had time to process prior operations.
func expectNoMessage(t *testing.T, ch <-chan Message) {
	t.Helper()
	select {
	case msg := <-ch:
		t.Fatalf("expected no message but got type=%s", msg.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

// expectClosed asserts that the channel is closed (read returns ok=false).
func expectClosed(t *testing.T, ch <-chan Message) {
	t.Helper()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed but received a message")
		}
	case <-time.After(time.Second):
		t.Fatal("expected channel to be closed but timed out")
	}
}

// syncHub ensures all prior Hub operations have been processed.
// It sends a DM-style broadcast to `via` and waits for delivery; since the
// Hub is single-goroutine, everything queued before the sync message has
// already been handled by the time it arrives.
func syncHub(t *testing.T, h *Hub, via *Client) {
	t.Helper()
	h.broadcast <- BroadcastMsg{
		msg:           Message{Type: "__sync__"},
		targetUserIDs: []int64{via.userID},
	}
	select {
	case <-via.send:
	case <-time.After(time.Second):
		t.Fatal("syncHub: timed out waiting for hub")
	}
}

// startHub creates a Hub, starts its Run loop, and returns it.
func startHub(t *testing.T) *Hub {
	t.Helper()
	h := NewHub(bus.NewBus(nopLogger))
	go h.Run()
	return h
}

// registerAll registers every client and syncs via the last one.
func registerAll(t *testing.T, h *Hub, clients ...*Client) {
	t.Helper()
	for _, c := range clients {
		h.register <- c
	}
	syncHub(t, h, clients[len(clients)-1])
}

// ---------------------------------------------------------------------------
// Test 1 – Register: client registered receives broadcasts of its room
// ---------------------------------------------------------------------------

func TestHub_Register(t *testing.T) {
	h := startHub(t)

	c := newTestClient(h, 1, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, c, sync)

	h.broadcast <- BroadcastMsg{msg: Message{Type: TypeRoomMessage}, targetRoomID: 10}
	syncHub(t, h, sync)

	got := expectMessage(t, c.send)
	if got.Type != TypeRoomMessage {
		t.Fatalf("expected type %s, got %s", TypeRoomMessage, got.Type)
	}
}

// ---------------------------------------------------------------------------
// Test 2 – Unregister: send channel closed, no more messages
// ---------------------------------------------------------------------------

func TestHub_Unregister(t *testing.T) {
	h := startHub(t)

	c := newTestClient(h, 1, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, c, sync)

	h.unregister <- c
	syncHub(t, h, sync)

	expectClosed(t, c.send)
}

// ---------------------------------------------------------------------------
// Test 3 – Broadcast room routing: A (room 10) receives, B (room 20) does not
// ---------------------------------------------------------------------------

func TestHub_BroadcastRoomRouting(t *testing.T) {
	h := startHub(t)

	a := newTestClient(h, 1, map[int64]bool{10: true})
	b := newTestClient(h, 2, map[int64]bool{20: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, a, b, sync)

	h.broadcast <- BroadcastMsg{msg: Message{Type: TypeRoomMessage}, targetRoomID: 10}
	syncHub(t, h, sync)

	got := expectMessage(t, a.send)
	if got.Type != TypeRoomMessage {
		t.Fatalf("a: expected %s, got %s", TypeRoomMessage, got.Type)
	}
	expectNoMessage(t, b.send)
}

// ---------------------------------------------------------------------------
// Test 4 – Broadcast room multi-client: both A and B in room 10 receive
// ---------------------------------------------------------------------------

func TestHub_BroadcastRoomMultiClient(t *testing.T) {
	h := startHub(t)

	a := newTestClient(h, 1, map[int64]bool{10: true})
	b := newTestClient(h, 2, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, a, b, sync)

	h.broadcast <- BroadcastMsg{msg: Message{Type: TypeRoomMessage}, targetRoomID: 10}
	syncHub(t, h, sync)

	gotA := expectMessage(t, a.send)
	gotB := expectMessage(t, b.send)
	if gotA.Type != TypeRoomMessage || gotB.Type != TypeRoomMessage {
		t.Fatalf("expected both to get %s, got a=%s b=%s", TypeRoomMessage, gotA.Type, gotB.Type)
	}
}

// ---------------------------------------------------------------------------
// Test 5 – Broadcast DM: only targeted users receive
// ---------------------------------------------------------------------------

func TestHub_BroadcastDM(t *testing.T) {
	h := startHub(t)

	a := newTestClient(h, 1, map[int64]bool{})
	b := newTestClient(h, 2, map[int64]bool{})
	c := newTestClient(h, 3, map[int64]bool{})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, a, b, c, sync)

	h.broadcast <- BroadcastMsg{
		msg:           Message{Type: TypeDirectMessage},
		targetUserIDs: []int64{1, 2},
	}
	syncHub(t, h, sync)

	expectMessage(t, a.send)
	expectMessage(t, b.send)
	expectNoMessage(t, c.send)
}

// ---------------------------------------------------------------------------
// Test 6 – Backpressure: client with full buffer is kicked
// ---------------------------------------------------------------------------

func TestHub_Backpressure(t *testing.T) {
	h := startHub(t)

	a := newTestClient(h, 1, map[int64]bool{10: true})
	a.send = make(chan Message) // unbuffered → always full for non-blocking send
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, a, sync)

	h.broadcast <- BroadcastMsg{msg: Message{Type: TypeRoomMessage}, targetRoomID: 10}
	syncHub(t, h, sync)

	expectClosed(t, a.send)
}

// ---------------------------------------------------------------------------
// Test 7 – JoinRoom via userRoomUpdate: client receives in new room
// ---------------------------------------------------------------------------

func TestHub_JoinRoom(t *testing.T) {
	h := startHub(t)

	c := newTestClient(h, 1, map[int64]bool{})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, c, sync)

	// join room 10
	h.userRoomUpdate <- UserRoomPresent{userID: 1, roomID: 10, present: true}
	syncHub(t, h, sync)

	h.broadcast <- BroadcastMsg{msg: Message{Type: TypeRoomMessage}, targetRoomID: 10}
	syncHub(t, h, sync)

	got := expectMessage(t, c.send)
	if got.Type != TypeRoomMessage {
		t.Fatalf("expected %s, got %s", TypeRoomMessage, got.Type)
	}
}

// ---------------------------------------------------------------------------
// Test 8 – LeaveRoom via userRoomUpdate: client stops receiving
//          (exposes bug: line 100 used to delete the whole room entry)
// ---------------------------------------------------------------------------

func TestHub_LeaveRoom(t *testing.T) {
	h := startHub(t)

	a := newTestClient(h, 1, map[int64]bool{10: true})
	b := newTestClient(h, 2, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, a, b, sync)

	// a leaves room 10
	h.userRoomUpdate <- UserRoomPresent{userID: 1, roomID: 10, present: false}
	syncHub(t, h, sync)

	h.broadcast <- BroadcastMsg{msg: Message{Type: TypeRoomMessage}, targetRoomID: 10}
	syncHub(t, h, sync)

	// b should still receive (room not deleted)
	got := expectMessage(t, b.send)
	if got.Type != TypeRoomMessage {
		t.Fatalf("b: expected %s, got %s", TypeRoomMessage, got.Type)
	}
	// a should NOT receive
	expectNoMessage(t, a.send)
}

// ---------------------------------------------------------------------------
// Test 9 – Register duplicate: same userID overwrites, old channel orphaned
// ---------------------------------------------------------------------------

func TestHub_RegisterDuplicate(t *testing.T) {
	h := startHub(t)

	c1 := newTestClient(h, 1, map[int64]bool{10: true})
	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, c1, sync)

	c2 := newTestClient(h, 1, map[int64]bool{10: true}) // same userID
	h.register <- c2
	syncHub(t, h, sync)

	h.broadcast <- BroadcastMsg{msg: Message{Type: TypeRoomMessage}, targetRoomID: 10}
	syncHub(t, h, sync)

	// c2 (new) should receive
	got := expectMessage(t, c2.send)
	if got.Type != TypeRoomMessage {
		t.Fatalf("c2: expected %s, got %s", TypeRoomMessage, got.Type)
	}
	// c1 (old) should NOT receive — silently orphaned, channel still open
	expectNoMessage(t, c1.send)
}

// ---------------------------------------------------------------------------
// Test 10 – Unregister without register: no panic
// ---------------------------------------------------------------------------

func TestHub_UnregisterWithoutRegister(t *testing.T) {
	h := startHub(t)

	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, sync)

	ghost := newTestClient(h, 42, map[int64]bool{10: true})
	h.unregister <- ghost
	syncHub(t, h, sync) // would hang if Hub panicked
}

// ---------------------------------------------------------------------------
// Test 11 – JoinRoom / LeaveRoom for non-connected user: no panic
// ---------------------------------------------------------------------------

func TestHub_UserRoomUpdate_NonConnected(t *testing.T) {
	h := startHub(t)

	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, sync)

	// join for user that is not registered
	h.userRoomUpdate <- UserRoomPresent{userID: 42, roomID: 10, present: true}
	syncHub(t, h, sync)

	// leave for user that is not registered
	h.userRoomUpdate <- UserRoomPresent{userID: 42, roomID: 10, present: false}
	syncHub(t, h, sync)
}

// ---------------------------------------------------------------------------
// Test 12 – Broadcast to non-existent room: no panic
// ---------------------------------------------------------------------------

func TestHub_BroadcastNonExistentRoom(t *testing.T) {
	h := startHub(t)

	sync := newTestClient(h, 99, map[int64]bool{})
	registerAll(t, h, sync)

	h.broadcast <- BroadcastMsg{msg: Message{Type: TypeRoomMessage}, targetRoomID: 9999}
	syncHub(t, h, sync) // would hang if Hub panicked

	// also check DM to non-existent user
	h.broadcast <- BroadcastMsg{
		msg:           Message{Type: TypeDirectMessage},
		targetUserIDs: []int64{777},
	}
	syncHub(t, h, sync)
}

// ---------------------------------------------------------------------------
// Sanity: verify helpers compile with json import
// ---------------------------------------------------------------------------

var _ = json.RawMessage{}
