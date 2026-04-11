package room

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sleklere/realtime-chat/cmd/server/internal/httpx"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
)

type Store interface {
	GetRoomBySlug(ctx context.Context, slug string) (dbstore.Room, error)
	CreateRoom(ctx context.Context, params dbstore.CreateRoomParams) (dbstore.Room, error)
	ListRooms(ctx context.Context) ([]dbstore.Room, error)
	JoinRoom(ctx context.Context, params dbstore.JoinRoomParams) error
	LeaveRoom(ctx context.Context, params dbstore.LeaveRoomParams) error
	ListMessagesByRoom(ctx context.Context, params dbstore.ListMessagesByRoomParams) ([]dbstore.ListMessagesByRoomRow, error)
}

type Service struct {
	logger *slog.Logger
	store  Store
}

func NewService(s Store, l *slog.Logger) *Service {
	return &Service{store: s, logger: l}
}

func (s *Service) Create(ctx context.Context, name string) (dbstore.Room, error) {
	slug := slugify(name)
	room, err := s.store.CreateRoom(ctx, dbstore.CreateRoomParams{
		Name: name,
		Slug: slug,
	})
	if err != nil {
		return dbstore.Room{}, err
	}
	return room, nil
}

func (s *Service) GetRoomBySlug(ctx context.Context, slug string) (dbstore.Room, error) {
	room, err := s.store.GetRoomBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dbstore.Room{}, httpx.New(http.StatusNotFound, "not_found", "room not found", err)
		}
		return dbstore.Room{}, err
	}

	return room, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func (s *Service) ListRooms(ctx context.Context) ([]dbstore.Room, error) {
	rooms, err := s.store.ListRooms(ctx)
	if err != nil {
		return nil, err
	}

	return rooms, nil
}

func (s *Service) Join(ctx context.Context, roomID int64, userID int64) error {
	return s.store.JoinRoom(ctx, dbstore.JoinRoomParams{
		RoomID: roomID,
		UserID: userID,
	})
}

func (s *Service) Leave(ctx context.Context, roomID int64, userID int64) error {
	return s.store.LeaveRoom(ctx, dbstore.LeaveRoomParams{
		RoomID: roomID,
		UserID: userID,
	})
}

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
