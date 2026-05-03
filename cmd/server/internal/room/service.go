package room

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sleklere/chattui/cmd/server/internal/bus"
	"github.com/sleklere/chattui/cmd/server/internal/db"
	"github.com/sleklere/chattui/cmd/server/internal/errs"
	"github.com/sleklere/chattui/cmd/server/internal/event"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

// Store defines the persistence methods required by the room Service.
type Store interface {
	GetRoomBySlug(ctx context.Context, slug string) (dbstore.Room, error)
	CreateRoom(ctx context.Context, params dbstore.CreateRoomParams) (dbstore.Room, error)
	ListRooms(ctx context.Context) ([]dbstore.Room, error)
	JoinRoom(ctx context.Context, params dbstore.JoinRoomParams) error
	LeaveRoom(ctx context.Context, params dbstore.LeaveRoomParams) error
	ListMessagesByRoom(ctx context.Context, params dbstore.ListMessagesByRoomParams) ([]dbstore.ListMessagesByRoomRow, error)
	GetRoomsForUser(ctx context.Context, userID int64) ([]dbstore.Room, error)
	CreateMessage(ctx context.Context, params dbstore.CreateMessageParams) (dbstore.Message, error)
}

// Service provides room-related business logic.
type Service struct {
	logger *slog.Logger
	store  Store
	db     db.Beginner
	bus    bus.Bus
}

// NewService creates a new room Service.
func NewService(s Store, l *slog.Logger, b bus.Bus, d db.Beginner) *Service {
	return &Service{store: s, logger: l, bus: b, db: d}
}

// Create creates a new room with the given name and adds the creator as a member.
// Both operations run inside a transaction so a JoinRoom failure never leaves an orphaned room.
func (s *Service) Create(ctx context.Context, name string, creatorID int64) (dbstore.Room, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return dbstore.Room{}, err
	}
	// Rollback is a no-op if Commit already succeeded; the error is intentionally ignored.
	defer tx.Rollback(ctx) //nolint:errcheck

	q := dbstore.New(tx)
	slug := slugify(name)
	room, err := q.CreateRoom(ctx, dbstore.CreateRoomParams{Name: name, Slug: slug})
	if err != nil {
		return dbstore.Room{}, err
	}
	if err := q.JoinRoom(ctx, dbstore.JoinRoomParams{RoomID: room.ID, UserID: creatorID}); err != nil {
		return dbstore.Room{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return dbstore.Room{}, err
	}

	s.bus.Publish(ctx, event.RoomJoinedEvent{RoomID: room.ID, UserID: creatorID})
	return room, nil
}

// GetRoomBySlug returns the room with the given slug.
func (s *Service) GetRoomBySlug(ctx context.Context, slug string) (dbstore.Room, error) {
	room, err := s.store.GetRoomBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dbstore.Room{}, errs.ErrNotFound
		}
		return dbstore.Room{}, err
	}

	return room, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), "-")
	return s
}

// ListRooms returns all rooms.
func (s *Service) ListRooms(ctx context.Context) ([]dbstore.Room, error) {
	rooms, err := s.store.ListRooms(ctx)
	if err != nil {
		return nil, err
	}

	return rooms, nil
}

// Join adds the user to the room and publishes a RoomJoinedEvent.
func (s *Service) Join(ctx context.Context, roomID int64, userID int64) error {
	err := s.store.JoinRoom(ctx, dbstore.JoinRoomParams{
		RoomID: roomID,
		UserID: userID,
	})
	if err != nil {
		return err
	}

	// Published after commit. If the process crashes between commit and publish,
	// the event is lost. Acceptable tradeoff for now; outbox pattern is the upgrade path.
	s.bus.Publish(ctx, event.RoomJoinedEvent{RoomID: roomID, UserID: userID})

	return nil
}

// Leave removes the user from the room and publishes a RoomLeftEvent.
func (s *Service) Leave(ctx context.Context, roomID int64, userID int64) error {
	err := s.store.LeaveRoom(ctx, dbstore.LeaveRoomParams{
		RoomID: roomID,
		UserID: userID,
	})
	if err != nil {
		return err
	}

	// Published after commit. If the process crashes between commit and publish,
	// the event is lost. Acceptable tradeoff for now; outbox pattern is the upgrade path.
	s.bus.Publish(ctx, event.RoomLeftEvent{RoomID: roomID, UserID: userID})

	return nil
}

// GetRoomsForUser returns all rooms the user is a member of.
func (s *Service) GetRoomsForUser(ctx context.Context, userID int64) ([]dbstore.Room, error) {
	return s.store.GetRoomsForUser(ctx, userID)
}

// GetMessagesByRoomID returns messages for the given room up to the specified limit.
func (s *Service) GetMessagesByRoomID(ctx context.Context, roomID int64, limit int32) ([]dbstore.ListMessagesByRoomRow, error) {
	msgs, err := s.store.ListMessagesByRoom(ctx, dbstore.ListMessagesByRoomParams{
		RoomID: pgtype.Int8{Int64: roomID, Valid: true},
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	return msgs, nil
}

// SendRoomMessage persists a room message and publishes a RoomMessageSentEvent.
func (s *Service) SendRoomMessage(ctx context.Context, roomID int64, senderID int64, body string) (dbstore.Message, error) {
	dbMsg, err := s.store.CreateMessage(ctx, dbstore.CreateMessageParams{
		RoomID:   pgtype.Int8{Int64: roomID, Valid: true},
		SenderID: senderID,
		Body:     body,
	})
	if err != nil {
		return dbMsg, err
	}

	s.bus.Publish(ctx,
		event.RoomMessageSentEvent{
			RoomID:    dbMsg.RoomID.Int64,
			SenderID:  senderID,
			MessageID: dbMsg.ID,
			Body:      body,
		})

	return dbMsg, nil
}
