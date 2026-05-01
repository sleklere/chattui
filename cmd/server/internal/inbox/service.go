package inbox

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sleklere/realtime-chat/cmd/server/internal/bus"
	"github.com/sleklere/realtime-chat/cmd/server/internal/event"
)

// Service handles inbox events and persists them to the inbox tables.
type Service struct {
	bus    bus.Bus
	logger *slog.Logger
}

// NewService creates an inbox Service and registers its event handlers on the bus.
func NewService(b bus.Bus, l *slog.Logger) *Service {
	s := &Service{bus: b, logger: l}
	b.Subscribe("room_join", s.handleRoomJoined)
	b.Subscribe("room_leave", s.handleRoomLeft)
	return s
}

func (s *Service) handleRoomJoined(_ context.Context, e event.Event) error {
	roomJoinEvent, ok := e.(event.RoomJoinedEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}
	s.logger.Debug("room_join", "user_id", roomJoinEvent.UserID, "room_id", roomJoinEvent.RoomID)

	return nil
}

func (s *Service) handleRoomLeft(_ context.Context, e event.Event) error {
	roomLeaveEvent, ok := e.(event.RoomLeftEvent)
	if !ok {
		return fmt.Errorf("inbox: unexpected event type %T", e)
	}
	s.logger.Debug("room_leave", "user_id", roomLeaveEvent.UserID, "room_id", roomLeaveEvent.RoomID)

	return nil
}
