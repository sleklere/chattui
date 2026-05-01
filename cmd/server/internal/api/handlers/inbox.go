package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/sleklere/realtime-chat/cmd/server/internal/api/dto/response"
	"github.com/sleklere/realtime-chat/cmd/server/internal/auth"
	"github.com/sleklere/realtime-chat/cmd/server/internal/httpx"
	"github.com/sleklere/realtime-chat/cmd/server/internal/inbox"
)

// InboxHandler handles inbox-related HTTP endpoints.
type InboxHandler struct {
	logger   *slog.Logger
	inboxSvc *inbox.Service
}

// NewInboxHandler creates a new InboxHandler.
func NewInboxHandler(l *slog.Logger, s *inbox.Service) *InboxHandler {
	return &InboxHandler{logger: l, inboxSvc: s}
}

// List handles listing all inbox events for the authenticated user.
func (h *InboxHandler) List(w http.ResponseWriter, r *http.Request) error {
	claims, _ := auth.ClaimsFromCtx(r.Context())

	events, err := h.inboxSvc.ListByUser(r.Context(), claims.UserID, parseLimit(r))
	if err != nil {
		return err
	}

	res := make([]response.InboxRes, len(events))
	for i, e := range events {
		var readAt *time.Time
		if e.ReadAt.Valid {
			t := e.ReadAt.Time
			readAt = &t
		}

		res[i] = response.InboxRes{
			ID:             e.ID,
			Kind:           e.Kind,
			RoomID:         e.RoomID.Int64,
			SourceUserID:   e.SourceUserID,
			SourceUsername: e.SourceUsername,
			CreatedAt:      e.CreatedAt.Time,
			ReadAt:         readAt,
		}
	}
	return httpx.JSON(w, http.StatusOK, res)
}
