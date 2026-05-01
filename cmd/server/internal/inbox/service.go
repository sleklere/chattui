package inbox

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sleklere/realtime-chat/cmd/server/internal/bus"
	"github.com/sleklere/realtime-chat/cmd/server/internal/event"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
)

// Store defines the persistence methods required by the inbox Service.
type Store interface {
	SaveRoomEvent(ctx context.Context, params dbstore.SaveRoomEventParams) error
	FindEventsByUserID(ctx context.Context, params dbstore.FindEventsByUserIDParams) ([]dbstore.FindEventsByUserIDRow, error)
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
	return svc
}

func (s *Service) handleRoomJoined(ctx context.Context, e event.Event) error {
	roomJoinEvent, ok := e.(event.RoomJoinedEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}
	err := s.store.SaveRoomEvent(
		ctx,
		dbstore.SaveRoomEventParams{
			Kind:         e.Kind(),
			RoomID:       pgtype.Int8{Int64: roomJoinEvent.RoomID, Valid: true},
			SourceUserID: roomJoinEvent.UserID})
	if err != nil {
		return fmt.Errorf("inbox: saving %s to DB: %w", e.Kind(), err)
	}
	s.logger.Debug("room_join", "user_id", roomJoinEvent.UserID, "room_id", roomJoinEvent.RoomID)

	return nil
}

func (s *Service) handleRoomLeft(ctx context.Context, e event.Event) error {
	roomLeaveEvent, ok := e.(event.RoomLeftEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}
	err := s.store.SaveRoomEvent(
		ctx,
		dbstore.SaveRoomEventParams{
			Kind:         e.Kind(),
			RoomID:       pgtype.Int8{Int64: roomLeaveEvent.RoomID, Valid: true},
			SourceUserID: roomLeaveEvent.UserID,
		},
	)
	if err != nil {
		return fmt.Errorf("inbox: saving %s to DB: %w", e.Kind(), err)
	}
	s.logger.Debug("room_leave", "user_id", roomLeaveEvent.UserID, "room_id", roomLeaveEvent.RoomID)

	return nil
}

// ListByUser returns inbox events for the given user up to the specified limit.
func (s *Service) ListByUser(ctx context.Context, userID int64, limit int32) ([]dbstore.FindEventsByUserIDRow, error) {
	events, err := s.store.FindEventsByUserID(ctx, dbstore.FindEventsByUserIDParams{UserID: userID, Limit: limit})
	if err != nil {
		return make([]dbstore.FindEventsByUserIDRow, 0), err
	}

	return events, nil
}
