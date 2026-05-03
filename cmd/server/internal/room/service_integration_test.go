package room

import (
	"context"
	"testing"

	"github.com/sleklere/chattui/cmd/server/internal/bus"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
	"github.com/sleklere/chattui/cmd/server/internal/testhelper"
)

// newSvc creates a Service wired to testPool.
func newSvc(t *testing.T, b bus.Bus) (*Service, *dbstore.Queries) {
	t.Helper()
	q := dbstore.New(testPool)
	return NewService(q, testhelper.DiscardLogger(), b, testPool), q
}

// newUser inserts a user and registers cleanup.
func newUser(t *testing.T, q *dbstore.Queries, username string) int64 {
	t.Helper()
	ctx := context.Background()
	u, err := q.CreateUser(ctx, dbstore.CreateUserParams{Username: username, Password: "x"})
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, "DELETE FROM users WHERE id = $1", u.ID) }) //nolint:errcheck
	return u.ID
}

// newRoom creates a room via the service and registers cleanup.
func newRoom(t *testing.T, svc *Service, name string, creatorID int64) dbstore.Room {
	t.Helper()
	room, err := svc.Create(context.Background(), name, creatorID)
	if err != nil {
		t.Fatalf("Create room %q: %v", name, err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), "DELETE FROM rooms WHERE id = $1", room.ID) }) //nolint:errcheck
	return room
}

func TestCreate_CreatorIsAddedAsMember(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	svc, q := newSvc(t, bus.NewBus(testhelper.DiscardLogger()))

	userID := newUser(t, q, "creator")
	room := newRoom(t, svc, "General", userID)

	rooms, err := q.GetRoomsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetRoomsForUser: %v", err)
	}
	if len(rooms) != 1 || rooms[0].ID != room.ID {
		t.Errorf("expected creator to be a member of the new room")
	}
}

func TestCreate_JoinFailure_RollsBack(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	svc, q := newSvc(t, bus.NewBus(testhelper.DiscardLogger()))

	const slug = "orphan-room"
	_, _ = testPool.Exec(ctx, "DELETE FROM rooms WHERE slug = $1", slug)
	t.Cleanup(func() { testPool.Exec(ctx, "DELETE FROM rooms WHERE slug = $1", slug) }) //nolint:errcheck

	_, err := svc.Create(ctx, "Orphan Room", 999999) // non-existent user → JoinRoom fails
	if err == nil {
		t.Fatal("expected error when creator does not exist")
	}

	// Transaction must have rolled back — room must not exist.
	_, lookupErr := q.GetRoomBySlug(ctx, slug)
	if lookupErr == nil {
		t.Fatal("room exists after failed Create — transaction did not roll back")
	}
}

func TestJoin_AddsUserToRoom(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	svc, q := newSvc(t, bus.NewBus(testhelper.DiscardLogger()))

	creatorID := newUser(t, q, "owner")
	joinerID := newUser(t, q, "joiner")
	room := newRoom(t, svc, "Lobby", creatorID)

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
	svc, q := newSvc(t, bus.NewBus(testhelper.DiscardLogger()))

	userID := newUser(t, q, "leaver")
	room := newRoom(t, svc, "Temp", userID)

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
	b := &testhelper.CaptureBus{}
	svc, q := newSvc(t, b)

	userID := newUser(t, q, "creator-pub")
	newRoom(t, svc, "Pub Room", userID)

	if len(b.EventsOfKind("room_join")) != 1 {
		t.Errorf("expected 1 room_join event, got %d", len(b.EventsOfKind("room_join")))
	}
}

func TestJoin_PublishesRoomJoinedEvent(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	b := &testhelper.CaptureBus{}
	svc, q := newSvc(t, b)

	creatorID := newUser(t, q, "creator-j")
	joinerID := newUser(t, q, "joiner-j")
	room := newRoom(t, svc, "Join Pub", creatorID)

	if err := svc.Join(context.Background(), room.ID, joinerID); err != nil {
		t.Fatalf("Join: %v", err)
	}

	if len(b.EventsOfKind("room_join")) != 2 { // Create + Join
		t.Errorf("expected 2 room_join events, got %d", len(b.EventsOfKind("room_join")))
	}
}

func TestLeave_PublishesRoomLeftEvent(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	b := &testhelper.CaptureBus{}
	svc, q := newSvc(t, b)

	userID := newUser(t, q, "leaver-pub")
	room := newRoom(t, svc, "Leave Pub", userID)

	if err := svc.Leave(context.Background(), room.ID, userID); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	if len(b.EventsOfKind("room_leave")) != 1 {
		t.Errorf("expected 1 room_leave event, got %d", len(b.EventsOfKind("room_leave")))
	}
}

func TestSendRoomMessage_PublishesEvent(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	b := &testhelper.CaptureBus{}
	svc, q := newSvc(t, b)

	userID := newUser(t, q, "msg-pub")
	room := newRoom(t, svc, "Msg Pub", userID)

	if _, err := svc.SendRoomMessage(context.Background(), room.ID, userID, "hello"); err != nil {
		t.Fatalf("SendRoomMessage: %v", err)
	}

	if len(b.EventsOfKind("room_message")) != 1 {
		t.Errorf("expected 1 room_message event, got %d", len(b.EventsOfKind("room_message")))
	}
}

func TestSendRoomMessage_PersistsMessage(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	svc, q := newSvc(t, bus.NewBus(testhelper.DiscardLogger()))

	userID := newUser(t, q, "sender")
	room := newRoom(t, svc, "Chat", userID)

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
