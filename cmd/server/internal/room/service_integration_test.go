package room

import (
	"context"
	"testing"

	"github.com/sleklere/realtime-chat/cmd/server/internal/bus"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
	"github.com/sleklere/realtime-chat/cmd/server/internal/testhelper"
)

// newUser creates a user directly via the store and returns their ID.
func newUser(t *testing.T, q *dbstore.Queries, username string) int64 {
	t.Helper()
	u, err := q.CreateUser(context.Background(), dbstore.CreateUserParams{
		Username: username,
		Password: "hashed",
	})
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return u.ID
}

func TestCreate_CreatorIsAddedAsMember(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := testhelper.WithTx(t, testPool)
	svc := NewService(q, testhelper.DiscardLogger(), bus.NewBus(testhelper.DiscardLogger()))

	userID := newUser(t, q, "creator")

	room, err := svc.Create(ctx, "General", userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rooms, err := q.GetRoomsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetRoomsForUser: %v", err)
	}
	if len(rooms) != 1 || rooms[0].ID != room.ID {
		t.Errorf("expected creator to be a member of the new room")
	}
}

func TestCreate_JoinFailure_RoomOrphaned(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	// This test documents a known bug: Create is not wrapped in a transaction.
	// If JoinRoom fails, the room is already committed and becomes orphaned.
	// The test currently FAILS — it should PASS once Create uses a transaction.
	ctx := context.Background()
	q := dbstore.New(testPool) // direct pool: we need to see committed state
	svc := NewService(q, testhelper.DiscardLogger(), bus.NewBus(testhelper.DiscardLogger()))

	const slug = "orphan-room"
	const nonExistentUserID = int64(999999)

	// Ensure the slug is clean before and after (in case a previous run crashed).
	_, _ = testPool.Exec(ctx, "DELETE FROM rooms WHERE slug = $1", slug)
	t.Cleanup(func() { _, _ = testPool.Exec(ctx, "DELETE FROM rooms WHERE slug = $1", slug) })

	_, err := svc.Create(ctx, "Orphan Room", nonExistentUserID)
	if err == nil {
		t.Fatal("expected Create to return an error when creator user does not exist")
	}

	// If Create were transactional, no room would survive the JoinRoom failure.
	_, lookupErr := q.GetRoomBySlug(ctx, slug)
	if lookupErr == nil {
		t.Fatalf("BUG: room %q was committed despite Create failing — Create is not transactional", slug)
	}
}

func TestJoin_AddsUserToRoom(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := testhelper.WithTx(t, testPool)
	svc := NewService(q, testhelper.DiscardLogger(), bus.NewBus(testhelper.DiscardLogger()))

	creatorID := newUser(t, q, "owner")
	joinerID := newUser(t, q, "joiner")

	room, err := svc.Create(ctx, "Lobby", creatorID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Join(ctx, room.ID, joinerID); err != nil {
		t.Fatalf("Join: %v", err)
	}

	rooms, err := q.GetRoomsForUser(ctx, joinerID)
	if err != nil {
		t.Fatalf("GetRoomsForUser: %v", err)
	}
	if len(rooms) != 1 || rooms[0].ID != room.ID {
		t.Errorf("expected joiner to be in the room after Join")
	}
}

func TestLeave_RemovesUserFromRoom(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := testhelper.WithTx(t, testPool)
	svc := NewService(q, testhelper.DiscardLogger(), bus.NewBus(testhelper.DiscardLogger()))

	userID := newUser(t, q, "leaver")

	room, err := svc.Create(ctx, "Temp", userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Leave(ctx, room.ID, userID); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	rooms, err := q.GetRoomsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetRoomsForUser: %v", err)
	}
	if len(rooms) != 0 {
		t.Errorf("expected user to have no rooms after Leave, got %d", len(rooms))
	}
}

func TestCreate_PublishesRoomJoinedEvent(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := testhelper.WithTx(t, testPool)
	b := &testhelper.CaptureBus{}
	svc := NewService(q, testhelper.DiscardLogger(), b)

	userID := newUser(t, q, "creator-pub")
	_, err := svc.Create(ctx, "Pub Room", userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	events := b.EventsOfKind("room_join")
	if len(events) != 1 {
		t.Fatalf("expected 1 room_join event, got %d", len(events))
	}
}

func TestJoin_PublishesRoomJoinedEvent(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := testhelper.WithTx(t, testPool)
	b := &testhelper.CaptureBus{}
	svc := NewService(q, testhelper.DiscardLogger(), b)

	creatorID := newUser(t, q, "creator-j")
	joinerID := newUser(t, q, "joiner-j")
	room, _ := svc.Create(ctx, "Join Pub", creatorID)
	b.EventsOfKind("room_join") // drain Create's event

	if err := svc.Join(ctx, room.ID, joinerID); err != nil {
		t.Fatalf("Join: %v", err)
	}

	events := b.EventsOfKind("room_join")
	if len(events) != 2 { // Create + Join
		t.Fatalf("expected 2 room_join events total, got %d", len(events))
	}
}

func TestLeave_PublishesRoomLeftEvent(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := testhelper.WithTx(t, testPool)
	b := &testhelper.CaptureBus{}
	svc := NewService(q, testhelper.DiscardLogger(), b)

	userID := newUser(t, q, "leaver-pub")
	room, _ := svc.Create(ctx, "Leave Pub", userID)

	if err := svc.Leave(ctx, room.ID, userID); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	events := b.EventsOfKind("room_leave")
	if len(events) != 1 {
		t.Fatalf("expected 1 room_leave event, got %d", len(events))
	}
}

func TestSendRoomMessage_PublishesRoomMessageSentEvent(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := testhelper.WithTx(t, testPool)
	b := &testhelper.CaptureBus{}
	svc := NewService(q, testhelper.DiscardLogger(), b)

	userID := newUser(t, q, "msg-pub")
	room, _ := svc.Create(ctx, "Msg Pub", userID)

	if _, err := svc.SendRoomMessage(ctx, room.ID, userID, "hello"); err != nil {
		t.Fatalf("SendRoomMessage: %v", err)
	}

	events := b.EventsOfKind("room_message")
	if len(events) != 1 {
		t.Fatalf("expected 1 room_message event, got %d", len(events))
	}
}

func TestSendRoomMessage_PersistsMessage(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := testhelper.WithTx(t, testPool)
	svc := NewService(q, testhelper.DiscardLogger(), bus.NewBus(testhelper.DiscardLogger()))

	userID := newUser(t, q, "sender")

	room, err := svc.Create(ctx, "Chat", userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	msg, err := svc.SendRoomMessage(ctx, room.ID, userID, "hello world")
	if err != nil {
		t.Fatalf("SendRoomMessage: %v", err)
	}

	if msg.Body != "hello world" {
		t.Errorf("body: want %q, got %q", "hello world", msg.Body)
	}
	if msg.SenderID != userID {
		t.Errorf("sender: want %d, got %d", userID, msg.SenderID)
	}
	if !msg.RoomID.Valid || msg.RoomID.Int64 != room.ID {
		t.Errorf("expected message linked to room %d", room.ID)
	}
}
