package room

import (
	"context"
	"errors"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sleklere/chattui/cmd/server/internal/conversation"
	"github.com/sleklere/chattui/cmd/server/internal/messagestore"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
	"github.com/sleklere/chattui/cmd/server/internal/testhelper"
)

func TestBadgerRoomAndDirectMessages(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := dbstore.New(testPool)
	logger := testhelper.DiscardLogger()
	b := &testhelper.CaptureBus{}
	alice := newUser(t, q, "badger-alice")
	bob := newUser(t, q, "badger-bob")
	path := t.TempDir()
	messages, err := messagestore.Open(path, messagestore.NewPostgresMetadata(testPool))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = messages.Close() })
	svc := NewService(q, logger, b, testPool, WithMessageStore(messages))
	room := newRoom(t, svc, "Badger Room", alice)
	roomMsg, err := svc.SendRoomMessage(ctx, room.ID, alice, "hello room")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := svc.GetMessagesByRoomID(ctx, room.ID, 10)
	if err != nil || len(rows) != 1 || rows[0].ID != roomMsg.ID || rows[0].SenderUsername != "badger-alice" {
		t.Fatalf("room history: %+v, %v", rows, err)
	}
	if len(b.EventsOfKind("room_message")) != 1 {
		t.Fatal("room delivery event missing")
	}
	convSvc := conversation.NewService(q, logger, b, testPool, conversation.WithMessageStore(messages))
	dm, err := convSvc.SendDirectMessage(ctx, alice, bob, "hello DM")
	if err != nil {
		t.Fatal(err)
	}
	second, err := convSvc.SendDirectMessage(ctx, bob, alice, "reply")
	if err != nil {
		t.Fatal(err)
	}
	if second.ConversationID != dm.ConversationID || second.ID <= dm.ID {
		t.Fatal("conversation identity or message ordering changed")
	}
	history, err := convSvc.ListMessages(ctx, dm.ConversationID.Int64, 10)
	if err != nil || len(history) != 2 || history[0].Body != "reply" {
		t.Fatalf("DM history: %+v, %v", history, err)
	}
	if len(b.EventsOfKind("conversation_created")) != 1 || len(b.EventsOfKind("direct_message")) != 2 {
		t.Fatal("unexpected DM events")
	}
	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM messages WHERE room_id=$1 OR conversation_id=$2`, room.ID, dm.ConversationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("Badger mode wrote %d messages to PostgreSQL", count)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM inbox_conversations WHERE ref_conversation_id=$1`, dm.ConversationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected two durable inbox cursors, got %d", count)
	}

	if err := messages.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := messagestore.Open(path, messagestore.NewPostgresMetadata(testPool))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close() //nolint:errcheck
	history, err = reopened.ListMessagesByConversation(ctx, dbstore.ListMessagesByConversationParams{ConversationID: dm.ConversationID, Limit: 10})
	if err != nil || len(history) != 2 || history[1].Body != "hello DM" {
		t.Fatalf("reopened DM history: %+v, %v", history, err)
	}
	// Resolve the current username from PostgreSQL, not a message-time snapshot.
	if _, err := testPool.Exec(ctx, "UPDATE users SET username='badger-alice-renamed' WHERE id=$1", alice); err != nil {
		t.Fatal(err)
	}
	rows, err = reopened.ListMessagesByRoom(ctx, dbstore.ListMessagesByRoomParams{RoomID: pgtype.Int8{Int64: room.ID, Valid: true}, Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].SenderUsername != "badger-alice-renamed" {
		t.Fatalf("reopened room history: %+v, %v", rows, err)
	}
}

func TestBadgerFailureLeavesConversationButNoMessageEvent(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := dbstore.New(testPool)
	b := &testhelper.CaptureBus{}
	alice := newUser(t, q, "badger-failure-alice")
	bob := newUser(t, q, "badger-failure-bob")
	path := t.TempDir()
	messages, err := messagestore.Open(path, messagestore.NewPostgresMetadata(testPool))
	if err != nil {
		t.Fatal(err)
	}
	if err := messages.Close(); err != nil {
		t.Fatal(err)
	}
	svc := conversation.NewService(q, testhelper.DiscardLogger(), b, testPool, conversation.WithMessageStore(messages))
	if _, err := svc.SendDirectMessage(ctx, alice, bob, "failed"); !errors.Is(err, badger.ErrDBClosed) {
		t.Fatalf("write failure: %v", err)
	}
	if len(b.EventsOfKind("direct_message")) != 0 {
		t.Fatal("failed message was published")
	}
	convs, err := q.ListConversationsByUser(ctx, dbstore.ListConversationsByUserParams{UserID: alice, Lim: 10})
	if err != nil || len(convs) != 1 {
		t.Fatalf("expected durable empty conversation: %+v, %v", convs, err)
	}
	reopened, err := messagestore.Open(path, messagestore.NewPostgresMetadata(testPool))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close() //nolint:errcheck
	svc = conversation.NewService(q, testhelper.DiscardLogger(), b, testPool, conversation.WithMessageStore(reopened))
	dm, err := svc.SendDirectMessage(ctx, alice, bob, "retry")
	if err != nil || dm.ConversationID.Int64 != convs[0].ID {
		t.Fatalf("retry: %+v, %v", dm, err)
	}
	history, err := svc.ListMessages(ctx, dm.ConversationID.Int64, 10)
	if err != nil || len(history) != 1 || history[0].Body != "retry" {
		t.Fatalf("retry history: %+v, %v", history, err)
	}
	if len(b.EventsOfKind("direct_message")) != 1 {
		t.Fatal("retry delivery event missing")
	}
	var count int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM inbox_conversations WHERE ref_conversation_id=$1`, dm.ConversationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("retry lost inbox cursors")
	}
}

func TestBadgerRejectsMissingMetadata(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := dbstore.New(testPool)
	alice := newUser(t, q, "badger-validation-alice")
	messages, err := messagestore.Open(t.TempDir(), messagestore.NewPostgresMetadata(testPool))
	if err != nil {
		t.Fatal(err)
	}
	defer messages.Close() //nolint:errcheck
	for _, p := range []dbstore.CreateMessageParams{
		{RoomID: pgtype.Int8{Int64: 999999, Valid: true}, SenderID: alice},
		{ConversationID: pgtype.Int8{Int64: 999999, Valid: true}, SenderID: alice},
	} {
		if _, err := messages.CreateMessage(ctx, p); err == nil {
			t.Fatal("missing chat accepted")
		}
	}
	svc := NewService(q, testhelper.DiscardLogger(), &testhelper.CaptureBus{}, testPool, WithMessageStore(messages))
	room := newRoom(t, svc, "Badger Validation Room", alice)
	if _, err := svc.SendRoomMessage(ctx, room.ID, 999999, "invalid sender"); err == nil {
		t.Fatal("missing sender accepted")
	}
}

func TestPostgresDirectMessagesRemainTransactional(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	q := dbstore.New(testPool)
	alice := newUser(t, q, "postgres-dm-alice")
	bob := newUser(t, q, "postgres-dm-bob")
	svc := conversation.NewService(q, testhelper.DiscardLogger(), &testhelper.CaptureBus{}, testPool)
	dm, err := svc.SendDirectMessage(ctx, alice, bob, "postgres message")
	if err != nil {
		t.Fatal(err)
	}
	history, err := q.ListMessagesByConversation(ctx, dbstore.ListMessagesByConversationParams{ConversationID: dm.ConversationID, Limit: 10})
	if err != nil || len(history) != 1 || history[0].ID != dm.ID {
		t.Fatalf("PostgreSQL history: %+v, %v", history, err)
	}
	if _, err := svc.SendDirectMessage(ctx, alice, 999999, "invalid recipient"); err == nil {
		t.Fatal("invalid recipient accepted")
	}
	convs, err := q.ListConversationsByUser(ctx, dbstore.ListConversationsByUserParams{UserID: alice, Lim: 10})
	if err != nil || len(convs) != 1 {
		t.Fatalf("unexpected conversations after failure: %+v, %v", convs, err)
	}
}
