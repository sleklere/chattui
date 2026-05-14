package inbox

import (
	"github.com/sleklere/chattui/cmd/server/internal/event"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

func toInboxEntryUpdatedEvent(userID int64, entry FeedEntry) event.InboxEntryUpdatedEvent {
	return event.InboxEntryUpdatedEvent{
		UserID:              userID,
		EntryType:           entry.EntryType,
		EntryKind:           entry.Kind,
		SourceUserID:        entry.SourceUserID,
		SourceUsername:      entry.SourceUsername,
		RefRoomID:           entry.RefRoomID,
		RefRoomName:         entry.RefRoomName,
		RefConversationID:   entry.RefConversationID,
		UnreadCount:         entry.UnreadCount,
		LastMessageBody:     entry.LastMessageBody,
		LastMessageSenderID: entry.LastMessageSenderID,
		PeerID:              entry.PeerID,
		PeerUsername:        entry.PeerUsername,
	}
}

func feedEntryFromList(row dbstore.ListInboxFeedRow) FeedEntry {
	e := FeedEntry{
		EntryType:      row.EntryType,
		Kind:           row.Kind,
		SourceUserID:   row.SourceUserID,
		SourceUsername: row.SourceUsername,
		UnreadCount:    row.UnreadCount,
		PeerID:         row.PeerID,
		PeerUsername:   row.PeerUsername,
		CreatedAt:      row.CreatedAt.Time,
	}
	if row.RefRoomID.Valid {
		e.RefRoomID = &row.RefRoomID.Int64
	}
	if row.RoomName.Valid {
		e.RefRoomName = &row.RoomName.String
	}
	if row.RefConversationID.Valid {
		e.RefConversationID = &row.RefConversationID.Int64
	}
	if row.LastMessageBody.Valid {
		e.LastMessageBody = &row.LastMessageBody.String
	}
	if row.LastMessageSenderID.Valid {
		e.LastMessageSenderID = &row.LastMessageSenderID.Int64
	}
	return e
}

func feedEntryFromDMUpdate(row dbstore.UpdateInboxCursorOnDMMessageRow) FeedEntry {
	e := FeedEntry{
		EntryType:    row.EntryType,
		Kind:         row.Kind,
		SourceUserID: row.SourceUserID,
		UnreadCount:  row.UnreadCount,
		PeerID:       row.PeerID,
		PeerUsername: row.PeerUsername,
		CreatedAt:    row.CreatedAt.Time,
	}
	if row.RefRoomID.Valid {
		e.RefRoomID = &row.RefRoomID.Int64
	}
	if row.RoomName.Valid {
		e.RefRoomName = &row.RoomName.String
	}
	if row.RefConversationID.Valid {
		e.RefConversationID = &row.RefConversationID.Int64
	}
	if row.LastMessageBody.Valid {
		e.LastMessageBody = &row.LastMessageBody.String
	}
	if row.LastMessageSenderID.Valid {
		e.LastMessageSenderID = &row.LastMessageSenderID.Int64
	}
	return e
}

func feedEntryFromRoomUpdate(row dbstore.UpdateInboxCursorOnRoomMessageRow) FeedEntry {
	e := FeedEntry{
		EntryType:    row.EntryType,
		Kind:         row.Kind,
		SourceUserID: row.SourceUserID,
		UnreadCount:  row.UnreadCount,
		PeerID:       row.PeerID,
		PeerUsername: row.PeerUsername,
		CreatedAt:    row.CreatedAt.Time,
	}
	if row.RefRoomID.Valid {
		e.RefRoomID = &row.RefRoomID.Int64
	}
	if row.RoomName.Valid {
		e.RefRoomName = &row.RoomName.String
	}
	if row.RefConversationID.Valid {
		e.RefConversationID = &row.RefConversationID.Int64
	}
	if row.LastMessageBody.Valid {
		e.LastMessageBody = &row.LastMessageBody.String
	}
	if row.LastMessageSenderID.Valid {
		e.LastMessageSenderID = &row.LastMessageSenderID.Int64
	}
	return e
}
