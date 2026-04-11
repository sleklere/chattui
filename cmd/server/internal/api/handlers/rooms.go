package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	reqdto "github.com/sleklere/realtime-chat/cmd/server/internal/api/dto/request"
	"github.com/sleklere/realtime-chat/cmd/server/internal/api/dto/response"
	"github.com/sleklere/realtime-chat/cmd/server/internal/auth"
	"github.com/sleklere/realtime-chat/cmd/server/internal/httpx"
	"github.com/sleklere/realtime-chat/cmd/server/internal/room"
	"github.com/sleklere/realtime-chat/cmd/server/internal/ws"
)

// RoomHandler handles room-related HTTP requests.
type RoomHandler struct {
	logger  *slog.Logger
	hub     *ws.Hub
	roomSvc *room.Service
}

// NewRoomHandler creates a new RoomHandler with the given queries and logger.
func NewRoomHandler(l *slog.Logger, h *ws.Hub, s *room.Service) *RoomHandler {
	return &RoomHandler{logger: l, hub: h, roomSvc: s}
}

// Create handles room creation requests.
func (h *RoomHandler) Create(w http.ResponseWriter, r *http.Request) error {
	var req reqdto.CreateRoomReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return httpx.BadRequest("invalid_json", "invalid json", err)
	}
	if req.Name == "" {
		return httpx.BadRequest("missing_name", "room name is required", nil)
	}

	room, err := h.roomSvc.Create(r.Context(), req.Name)
	if err != nil {
		return err
	}

	return httpx.JSON(w, http.StatusCreated, response.RoomRes{
		ID:        room.ID,
		Name:      room.Name,
		Slug:      room.Slug,
		CreatedAt: room.CreatedAt.Time,
	})
}

// List handles listing all rooms.
func (h *RoomHandler) List(w http.ResponseWriter, r *http.Request) error {
	rooms, err := h.roomSvc.ListRooms(r.Context())
	if err != nil {
		return err
	}

	res := make([]response.RoomRes, len(rooms))
	for i, room := range rooms {
		res[i] = response.RoomRes{
			ID:        room.ID,
			Name:      room.Name,
			Slug:      room.Slug,
			CreatedAt: room.CreatedAt.Time,
		}
	}
	return httpx.JSON(w, http.StatusOK, res)
}

// GetBySlug handles fetching a room by its slug.
func (h *RoomHandler) GetBySlug(w http.ResponseWriter, r *http.Request) error {
	slug := chi.URLParam(r, "slug")
	room, err := h.roomSvc.GetRoomBySlug(r.Context(), slug)
	if err != nil {
		return err
	}

	return httpx.JSON(w, http.StatusOK,
		response.RoomRes{
			ID:        room.ID,
			Name:      room.Name,
			Slug:      room.Slug,
			CreatedAt: room.CreatedAt.Time,
		})
}

// Join handles joining a room.
func (h *RoomHandler) Join(w http.ResponseWriter, r *http.Request) error {
	claims, ok := auth.ClaimsFromCtx(r.Context())
	if !ok {
		return httpx.New(http.StatusUnauthorized, "unauthorized", "missing claims", nil)
	}

	roomID, err := strconv.ParseInt(chi.URLParam(r, "roomID"), 10, 64)
	if err != nil {
		return httpx.BadRequest("invalid_room_id", "invalid room id", err)
	}

	err = h.roomSvc.Join(r.Context(), roomID, claims.UserID)
	if err != nil {
		return err
	}

	h.hub.UpdateUserRoomState(roomID, claims.UserID, true)

	return httpx.JSON(w, http.StatusNoContent, nil)
}

// Leave handles leaving a room.
func (h *RoomHandler) Leave(w http.ResponseWriter, r *http.Request) error {
	claims, ok := auth.ClaimsFromCtx(r.Context())
	if !ok {
		return httpx.New(http.StatusUnauthorized, "unauthorized", "missing claims", nil)
	}

	roomID, err := strconv.ParseInt(chi.URLParam(r, "roomID"), 10, 64)
	if err != nil {
		return httpx.BadRequest("invalid_room_id", "invalid room id", err)
	}

	err = h.roomSvc.Leave(r.Context(), roomID, claims.UserID)
	if err != nil {
		return err
	}

	h.hub.UpdateUserRoomState(roomID, claims.UserID, false)

	return httpx.JSON(w, http.StatusNoContent, nil)
}

// Messages handles fetching paginated message history for a room.
func (h *RoomHandler) Messages(w http.ResponseWriter, r *http.Request) error {
	roomID, err := strconv.ParseInt(chi.URLParam(r, "roomID"), 10, 64)
	if err != nil {
		return httpx.BadRequest("invalid_room_id", "invalid room id", err)
	}

	msgs, err := h.roomSvc.GetMessagesByRoomID(r.Context(), roomID, parseLimit(r))
	if err != nil {
		return err
	}

	res := make([]response.RoomMessageRes, len(msgs))
	for i, m := range msgs {
		res[i] = response.RoomMessageRes{
			MessageRes: response.MessageRes{
				ID:        m.ID,
				SenderID:  m.SenderID,
				Body:      m.Body,
				CreatedAt: m.CreatedAt.Time,
			},
			RoomID:         m.RoomID.Int64,
			SenderUsername: m.SenderUsername,
		}
	}
	return httpx.JSON(w, http.StatusOK, res)
}
