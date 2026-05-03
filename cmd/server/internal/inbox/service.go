package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sleklere/chattui/cmd/server/internal/bus"
	"github.com/sleklere/chattui/cmd/server/internal/event"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

// FeedEntry is the domain representation of a single inbox feed row.
type FeedEntry struct {
	EntryType           string
	Kind                string
	SourceUserID        int64
	SourceUsername      string
	RefRoomID           *int64
	RefRoomName         *string
	RefConversationID   *int64
	UnreadCount         int64
	LastMessageBody     *string
	LastMessageSenderID *int64
	PeerID              int64
	PeerUsername        string
	CreatedAt           time.Time
}

// Store defines the persistence methods required by the inbox Service.
type Store interface {
	SaveInboxRoomEvent(ctx context.Context, params dbstore.SaveInboxRoomEventParams) error
	UpsertInboxConversationCursor(ctx context.Context, params dbstore.UpsertInboxConversationCursorParams) error
	UpdateInboxCursorOnRoomMessage(ctx context.Context, params dbstore.UpdateInboxCursorOnRoomMessageParams) error
	UpdateInboxCursorOnDMMessage(ctx context.Context, params dbstore.UpdateInboxCursorOnDMMessageParams) error
	ListInboxFeed(ctx context.Context, params dbstore.ListInboxFeedParams) ([]dbstore.ListInboxFeedRow, error)
}

// Service handles inbox events and persists them to the inbox tables.
type Service struct {
	bus    bus.Bus
	logger *slog.Logger
	store  Store
}

// NewService creates an inbox Service and registers its event handlers on the bus.
func NewService(b bus.Bus, l *slog.Logger, s Store) *Service {
	svc := &Service{bus: b, logger: l, store: s}
	b.Subscribe("room_join", svc.handleRoomJoined)
	b.Subscribe("room_leave", svc.handleRoomLeft)
	b.Subscribe("conversation_created", svc.handleConversationCreated)
	b.Subscribe("room_message", svc.handleRoomMessageSent)
	b.Subscribe("direct_message", svc.handleDMSent)
	return svc
}

func (s *Service) handleRoomJoined(ctx context.Context, e event.Event) error {
	roomJoinEvent, ok := e.(event.RoomJoinedEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}

	if err := s.store.SaveInboxRoomEvent(ctx, dbstore.SaveInboxRoomEventParams{
		Kind:         e.Kind(),
		RoomID:       pgtype.Int8{Int64: roomJoinEvent.RoomID, Valid: true},
		SourceUserID: roomJoinEvent.UserID,
	}); err != nil {
		return fmt.Errorf("inbox: saving %s to DB: %w", e.Kind(), err)
	}

	if err := s.store.UpsertInboxConversationCursor(ctx, dbstore.UpsertInboxConversationCursorParams{
		UserID:            roomJoinEvent.UserID,
		RefRoomID:         pgtype.Int8{Int64: roomJoinEvent.RoomID, Valid: true},
		RefConversationID: pgtype.Int8{Valid: false},
	}); err != nil {
		s.logger.Warn("inbox: failed to upsert room cursor", "user_id", roomJoinEvent.UserID, "room_id", roomJoinEvent.RoomID, "error", err)
	}

	s.logger.Debug("room_join", "user_id", roomJoinEvent.UserID, "room_id", roomJoinEvent.RoomID)
	return nil
}

func (s *Service) handleRoomLeft(ctx context.Context, e event.Event) error {
	roomLeaveEvent, ok := e.(event.RoomLeftEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}
	if err := s.store.SaveInboxRoomEvent(ctx, dbstore.SaveInboxRoomEventParams{
		Kind:         e.Kind(),
		RoomID:       pgtype.Int8{Int64: roomLeaveEvent.RoomID, Valid: true},
		SourceUserID: roomLeaveEvent.UserID,
	}); err != nil {
		return fmt.Errorf("inbox: saving %s to DB: %w", e.Kind(), err)
	}
	s.logger.Debug("room_leave", "user_id", roomLeaveEvent.UserID, "room_id", roomLeaveEvent.RoomID)
	return nil
}

func (s *Service) handleConversationCreated(ctx context.Context, e event.Event) error {
	convEvent, ok := e.(event.ConversationCreatedEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}

	if err := s.store.UpsertInboxConversationCursor(ctx, dbstore.UpsertInboxConversationCursorParams{
		UserID:            convEvent.UserAID,
		RefRoomID:         pgtype.Int8{Valid: false},
		RefConversationID: pgtype.Int8{Int64: convEvent.ConversationID, Valid: true},
	}); err != nil {
		s.logger.Warn("inbox: failed to upsert conversation cursor", "user_id", convEvent.UserAID, "conversation_id", convEvent.ConversationID, "error", err)
	}

	if err := s.store.UpsertInboxConversationCursor(ctx, dbstore.UpsertInboxConversationCursorParams{
		UserID:            convEvent.UserBID,
		RefRoomID:         pgtype.Int8{Valid: false},
		RefConversationID: pgtype.Int8{Int64: convEvent.ConversationID, Valid: true},
	}); err != nil {
		s.logger.Warn("inbox: failed to upsert conversation cursor", "user_id", convEvent.UserBID, "conversation_id", convEvent.ConversationID, "error", err)
	}

	s.logger.Debug("conversation_created", "conversation_id", convEvent.ConversationID)
	return nil
}

func (s *Service) handleRoomMessageSent(ctx context.Context, e event.Event) error {
	msgEvent, ok := e.(event.RoomMessageSentEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}
	if err := s.store.UpdateInboxCursorOnRoomMessage(ctx, dbstore.UpdateInboxCursorOnRoomMessageParams{
		Body:     pgtype.Text{String: msgEvent.Body, Valid: true},
		SenderID: pgtype.Int8{Int64: msgEvent.SenderID, Valid: true},
		RoomID:   pgtype.Int8{Int64: msgEvent.RoomID, Valid: true},
	}); err != nil {
		s.logger.Warn("inbox: failed to update room cursor", "room_id", msgEvent.RoomID, "error", err)
	}
	return nil
}

func (s *Service) handleDMSent(ctx context.Context, e event.Event) error {
	msgEvent, ok := e.(event.DirectMessageSentEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}
	if err := s.store.UpdateInboxCursorOnDMMessage(ctx, dbstore.UpdateInboxCursorOnDMMessageParams{
		Body:           pgtype.Text{String: msgEvent.Body, Valid: true},
		SenderID:       pgtype.Int8{Int64: msgEvent.SenderID, Valid: true},
		ConversationID: pgtype.Int8{Int64: msgEvent.ConversationID, Valid: true},
	}); err != nil {
		s.logger.Warn("inbox: failed to update dm cursor", "conversation_id", msgEvent.ConversationID, "error", err)
	}
	return nil
}

// ListByUser returns the inbox feed for the given user up to the specified limit.
func (s *Service) ListByUser(ctx context.Context, userID int64, limit int32) ([]FeedEntry, error) {
	rows, err := s.store.ListInboxFeed(ctx, dbstore.ListInboxFeedParams{UserID: userID, Lim: limit})
	if err != nil {
		return nil, err
	}
	entries := make([]FeedEntry, len(rows))
	for i, row := range rows {
		entries[i] = toFeedEntry(row)
	}
	return entries, nil
}

func toFeedEntry(row dbstore.ListInboxFeedRow) FeedEntry {
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
