package inbox

import (
	"context"
	"errors"
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
	UpdateInboxCursorOnRoomMessage(ctx context.Context, params dbstore.UpdateInboxCursorOnRoomMessageParams) ([]dbstore.UpdateInboxCursorOnRoomMessageRow, error)
	UpdateInboxCursorOnDMMessage(ctx context.Context, params dbstore.UpdateInboxCursorOnDMMessageParams) ([]dbstore.UpdateInboxCursorOnDMMessageRow, error)
	ListInboxFeed(ctx context.Context, params dbstore.ListInboxFeedParams) ([]dbstore.ListInboxFeedRow, error)
	ResetRoomUnreadCount(ctx context.Context, params dbstore.ResetRoomUnreadCountParams) error
	ResetDMUnreadCount(ctx context.Context, params dbstore.ResetDMUnreadCountParams) error
	ResetAllUnreadCount(ctx context.Context, userID int64) error
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
	feedRows, err := s.store.UpdateInboxCursorOnRoomMessage(ctx, dbstore.UpdateInboxCursorOnRoomMessageParams{
		Body:     pgtype.Text{String: msgEvent.Body, Valid: true},
		SenderID: pgtype.Int8{Int64: msgEvent.SenderID, Valid: true},
		RoomID:   pgtype.Int8{Int64: msgEvent.RoomID, Valid: true},
	})
	if err != nil {
		s.logger.Warn("inbox: failed to update room cursor", "room_id", msgEvent.RoomID, "error", err)
	}
	for _, row := range feedRows {
		entry := feedEntryFromRoomUpdate(row)
		s.bus.Publish(ctx, toInboxEntryUpdatedEvent(row.UserID, entry))
	}
	return nil
}

func (s *Service) handleDMSent(ctx context.Context, e event.Event) error {
	msgEvent, ok := e.(event.DirectMessageSentEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}
	feedRows, err := s.store.UpdateInboxCursorOnDMMessage(ctx, dbstore.UpdateInboxCursorOnDMMessageParams{
		Body:           pgtype.Text{String: msgEvent.Body, Valid: true},
		SenderID:       pgtype.Int8{Int64: msgEvent.SenderID, Valid: true},
		RecipientID:    msgEvent.RecipientID,
		ConversationID: pgtype.Int8{Int64: msgEvent.ConversationID, Valid: true},
	})
	if err != nil {
		s.logger.Warn("inbox: failed to update dm cursor", "conversation_id", msgEvent.ConversationID, "error", err)
	}
	for _, row := range feedRows {
		entry := feedEntryFromDMUpdate(row)
		s.bus.Publish(ctx, toInboxEntryUpdatedEvent(row.UserID, entry))
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
		entries[i] = feedEntryFromList(row)
	}
	return entries, nil
}

// MarkAsRead resets the unread count for a specific room, DM conversation, or all entries for the user.
func (s *Service) MarkAsRead(ctx context.Context, conversationID, roomID *int64, all bool, userID int64) error {
	set := 0
	if conversationID != nil {
		set++
	}
	if roomID != nil {
		set++
	}
	if all {
		set++
	}
	if set == 0 {
		return errors.New("no_params")
	}
	if set > 1 {
		return errors.New("extra_params")
	}

	var err error
	if conversationID != nil {
		err = s.store.ResetDMUnreadCount(ctx,
			dbstore.ResetDMUnreadCountParams{
				ConversationID: pgtype.Int8{Int64: *conversationID, Valid: true},
				UserID:         userID,
			})
	} else if roomID != nil {
		err = s.store.ResetRoomUnreadCount(ctx,
			dbstore.ResetRoomUnreadCountParams{
				RoomID: pgtype.Int8{Int64: *roomID, Valid: true},
				UserID: userID,
			})
	} else if all {
		err = s.store.ResetAllUnreadCount(ctx, userID)
	}

	return err
}
