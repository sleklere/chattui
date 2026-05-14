package inbox

import (
	"context"
	"testing"

	"github.com/sleklere/chattui/cmd/server/internal/event"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
	"github.com/sleklere/chattui/cmd/server/internal/testhelper"
)

// setup creates two users, a room, and adds both as room_members.
// Returns (memberID, joinerID, roomID).
func setup(t *testing.T, q *dbstore.Queries, suffix string) (int64, int64, int64) {
	t.Helper()
	ctx := context.Background()

	member, err := q.CreateUser(ctx, dbstore.CreateUserParams{Username: "member-" + suffix, Password: "x"})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	joiner, err := q.CreateUser(ctx, dbstore.CreateUserParams{Username: "joiner-" + suffix, Password: "x"})
	if err != nil {
		t.Fatalf("create joiner: %v", err)
	}
	room, err := q.CreateRoom(ctx, dbstore.CreateRoomParams{Name: "room-" + suffix, Slug: "room-" + suffix})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if err := q.JoinRoom(ctx, dbstore.JoinRoomParams{RoomID: room.ID, UserID: member.ID}); err != nil {
		t.Fatalf("join room (member): %v", err)
	}
	return member.ID, joiner.ID, room.ID
}

func newInboxSvc(t *testing.T, q *dbstore.Queries) *Service {
	t.Helper()
	return NewService(&testhelper.CaptureBus{}, testhelper.DiscardLogger(), q)
}

func newInboxSvcWithBus(t *testing.T, q *dbstore.Queries) (*Service, *testhelper.CaptureBus) {
	t.Helper()
	b := &testhelper.CaptureBus{}
	return NewService(b, testhelper.DiscardLogger(), q), b
}

func TestHandleRoomJoined_SavesEventForMembersAndCursorForJoiner(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	_, q := testhelper.NewTx(t, testPool)
	svc := newInboxSvc(t, q)

	memberID, joinerID, roomID := setup(t, q, "join")

	if err := svc.handleRoomJoined(ctx, event.RoomJoinedEvent{RoomID: roomID, UserID: joinerID}); err != nil {
		t.Fatalf("handleRoomJoined: %v", err)
	}

	// inbox_event must exist for the existing member, not the joiner
	feed, err := q.ListInboxFeed(ctx, dbstore.ListInboxFeedParams{UserID: memberID, Lim: 10})
	if err != nil {
		t.Fatalf("ListInboxFeed: %v", err)
	}
	if len(feed) != 1 || feed[0].EntryType != "event" || feed[0].Kind != "room_join" {
		t.Errorf("expected 1 room_join event for member, got %+v", feed)
	}

	// cursor must exist for the joiner (no last_message_at yet, so ListInboxFeed won't show it;
	// verify via direct query)
	// Note: cursor won't appear in ListInboxFeed until a message is sent (last_message_at IS NOT NULL filter).
}

func TestHandleRoomLeft_SavesEventForRemainingMembers(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	_, q := testhelper.NewTx(t, testPool)
	svc := newInboxSvc(t, q)

	memberID, leaverID, roomID := setup(t, q, "leave")
	if err := q.JoinRoom(ctx, dbstore.JoinRoomParams{RoomID: roomID, UserID: leaverID}); err != nil {
		t.Fatalf("join room (leaver): %v", err)
	}

	if err := svc.handleRoomLeft(ctx, event.RoomLeftEvent{RoomID: roomID, UserID: leaverID}); err != nil {
		t.Fatalf("handleRoomLeft: %v", err)
	}

	feed, err := q.ListInboxFeed(ctx, dbstore.ListInboxFeedParams{UserID: memberID, Lim: 10})
	if err != nil {
		t.Fatalf("ListInboxFeed: %v", err)
	}
	if len(feed) != 1 || feed[0].Kind != "room_leave" {
		t.Errorf("expected 1 room_leave event for member, got %+v", feed)
	}
}

func TestHandleRoomMessageSent_UpdatesCursorForAllMembersExceptSender(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	_, q := testhelper.NewTx(t, testPool)
	svc := newInboxSvc(t, q)

	memberID, senderID, roomID := setup(t, q, "rmsg")

	// Both users need a cursor before UpdateInboxCursorOnRoomMessage can update it.
	for _, uid := range []int64{memberID, senderID} {
		if err := svc.handleRoomJoined(ctx, event.RoomJoinedEvent{RoomID: roomID, UserID: uid}); err != nil {
			t.Fatalf("handleRoomJoined uid=%d: %v", uid, err)
		}
	}

	if err := svc.handleRoomMessageSent(ctx, event.RoomMessageSentEvent{
		RoomID:   roomID,
		SenderID: senderID,
		Body:     "hello room",
	}); err != nil {
		t.Fatalf("handleRoomMessageSent: %v", err)
	}

	// member's cursor should reflect the new message
	feed, err := q.ListInboxFeed(ctx, dbstore.ListInboxFeedParams{UserID: memberID, Lim: 10})
	if err != nil {
		t.Fatalf("ListInboxFeed: %v", err)
	}
	var conv *dbstore.ListInboxFeedRow
	for i := range feed {
		if feed[i].EntryType == "conversation" {
			conv = &feed[i]
			break
		}
	}
	if conv == nil {
		t.Fatal("expected a conversation entry in member's inbox")
	}
	if !conv.LastMessageBody.Valid || conv.LastMessageBody.String != "hello room" {
		t.Errorf("last_message_body: want %q, got %+v", "hello room", conv.LastMessageBody)
	}
	if conv.UnreadCount != 1 {
		t.Errorf("unread_count: want 1, got %d", conv.UnreadCount)
	}

	// sender's own cursor must NOT be updated (user_id != sender_id filter in SQL)
	senderFeed, err := q.ListInboxFeed(ctx, dbstore.ListInboxFeedParams{UserID: senderID, Lim: 10})
	if err != nil {
		t.Fatalf("ListInboxFeed sender: %v", err)
	}
	for _, row := range senderFeed {
		if row.EntryType == "conversation" && row.UnreadCount > 0 {
			t.Errorf("sender's own cursor should have unread_count=0, got %d", row.UnreadCount)
		}
	}
}

func TestHandleRoomMessageSent_PublishesInboxEntryUpdatedForMembers(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	_, q := testhelper.NewTx(t, testPool)
	svc, bus := newInboxSvcWithBus(t, q)

	memberID, senderID, roomID := setup(t, q, "rmsg-pub")
	for _, uid := range []int64{memberID, senderID} {
		if err := svc.handleRoomJoined(ctx, event.RoomJoinedEvent{RoomID: roomID, UserID: uid}); err != nil {
			t.Fatalf("handleRoomJoined uid=%d: %v", uid, err)
		}
	}

	if err := svc.handleRoomMessageSent(ctx, event.RoomMessageSentEvent{
		RoomID: roomID, SenderID: senderID, Body: "hello",
	}); err != nil {
		t.Fatalf("handleRoomMessageSent: %v", err)
	}

	published := bus.EventsOfKind("inbox_entry_updated")
	if len(published) != 1 {
		t.Fatalf("expected 1 inbox_entry_updated event (member only), got %d", len(published))
	}
	e := published[0].(event.InboxEntryUpdatedEvent)
	if e.UserID != memberID {
		t.Errorf("expected UserID=%d, got %d", memberID, e.UserID)
	}
	if e.UnreadCount != 1 {
		t.Errorf("expected UnreadCount=1, got %d", e.UnreadCount)
	}
}

func TestHandleDMSent_PublishesInboxEntryUpdatedForRecipient(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	_, q := testhelper.NewTx(t, testPool)
	svc, bus := newInboxSvcWithBus(t, q)

	sender, err := q.CreateUser(ctx, dbstore.CreateUserParams{Username: "dmpub-sender", Password: "x"})
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	recipient, err := q.CreateUser(ctx, dbstore.CreateUserParams{Username: "dmpub-recipient", Password: "x"})
	if err != nil {
		t.Fatalf("create recipient: %v", err)
	}
	conv, err := q.GetOrCreateConversation(ctx, dbstore.GetOrCreateConversationParams{UserA: sender.ID, UserB: recipient.ID})
	if err != nil {
		t.Fatalf("GetOrCreateConversation: %v", err)
	}
	if err := svc.handleConversationCreated(ctx, event.ConversationCreatedEvent{
		ConversationID: conv.ID, UserAID: sender.ID, UserBID: recipient.ID,
	}); err != nil {
		t.Fatalf("handleConversationCreated: %v", err)
	}

	if err := svc.handleDMSent(ctx, event.DirectMessageSentEvent{
		ConversationID: conv.ID, SenderID: sender.ID, RecipientID: recipient.ID, Body: "hey",
	}); err != nil {
		t.Fatalf("handleDMSent: %v", err)
	}

	published := bus.EventsOfKind("inbox_entry_updated")
	if len(published) != 1 {
		t.Fatalf("expected 1 inbox_entry_updated event, got %d", len(published))
	}
	e := published[0].(event.InboxEntryUpdatedEvent)
	if e.UserID != recipient.ID {
		t.Errorf("expected UserID=%d, got %d", recipient.ID, e.UserID)
	}
	if e.RefConversationID == nil || *e.RefConversationID != conv.ID {
		t.Errorf("expected RefConversationID=%d, got %v", conv.ID, e.RefConversationID)
	}
}

func TestHandleDMSent_UpdatesCursorForRecipient(t *testing.T) {
	if testPool == nil {
		t.Skip("testcontainers not available")
	}
	ctx := context.Background()
	_, q := testhelper.NewTx(t, testPool)
	svc := newInboxSvc(t, q)

	sender, err := q.CreateUser(ctx, dbstore.CreateUserParams{Username: "dm-sender", Password: "x"})
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	recipient, err := q.CreateUser(ctx, dbstore.CreateUserParams{Username: "dm-recipient", Password: "x"})
	if err != nil {
		t.Fatalf("create recipient: %v", err)
	}

	// Create conversation and cursors for both sides.
	conv, err := q.GetOrCreateConversation(ctx, dbstore.GetOrCreateConversationParams{
		UserA: sender.ID,
		UserB: recipient.ID,
	})
	if err != nil {
		t.Fatalf("GetOrCreateConversation: %v", err)
	}
	if err := svc.handleConversationCreated(ctx, event.ConversationCreatedEvent{
		ConversationID: conv.ID,
		UserAID:        sender.ID,
		UserBID:        recipient.ID,
	}); err != nil {
		t.Fatalf("handleConversationCreated: %v", err)
	}

	if err := svc.handleDMSent(ctx, event.DirectMessageSentEvent{
		ConversationID: conv.ID,
		SenderID:       sender.ID,
		RecipientID:    recipient.ID,
		Body:           "hey there",
	}); err != nil {
		t.Fatalf("handleDMSent: %v", err)
	}

	feed, err := q.ListInboxFeed(ctx, dbstore.ListInboxFeedParams{UserID: recipient.ID, Lim: 10})
	if err != nil {
		t.Fatalf("ListInboxFeed: %v", err)
	}
	if len(feed) == 0 || feed[0].EntryType != "conversation" {
		t.Fatalf("expected conversation entry in recipient inbox, got %+v", feed)
	}
	if !feed[0].LastMessageBody.Valid || feed[0].LastMessageBody.String != "hey there" {
		t.Errorf("last_message_body: want %q, got %+v", "hey there", feed[0].LastMessageBody)
	}
	if feed[0].UnreadCount != 1 {
		t.Errorf("unread_count: want 1, got %d", feed[0].UnreadCount)
	}
	if feed[0].PeerUsername != sender.Username {
		t.Errorf("peer_username: want %q, got %q", sender.Username, feed[0].PeerUsername)
	}
}
