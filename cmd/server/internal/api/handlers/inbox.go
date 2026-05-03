package handlers

import (
	"log/slog"
	"net/http"

	"github.com/sleklere/realtime-chat/cmd/server/internal/api/dto/response"
	"github.com/sleklere/realtime-chat/cmd/server/internal/auth"
	"github.com/sleklere/realtime-chat/cmd/server/internal/httpx"
	"github.com/sleklere/realtime-chat/cmd/server/internal/inbox"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
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

// List handles listing the inbox feed for the authenticated user.
func (h *InboxHandler) List(w http.ResponseWriter, r *http.Request) error {
	claims, _ := auth.ClaimsFromCtx(r.Context())

	rows, err := h.inboxSvc.ListByUser(r.Context(), claims.UserID, parseLimit(r))
	if err != nil {
		return err
	}

	res := make([]response.InboxRes, len(rows))
	for i, row := range rows {
		res[i] = toInboxRes(row)
	}
	return httpx.JSON(w, http.StatusOK, res)
}

func toInboxRes(row dbstore.ListInboxFeedRow) response.InboxRes {
	r := response.InboxRes{
		EntryType: row.EntryType,
		CreatedAt: row.CreatedAt.Time,
	}

	if row.EntryType == "event" {
		r.Kind = row.Kind
		r.SourceUser = &response.InboxUser{
			ID:       row.SourceUserID,
			Username: row.SourceUsername,
		}
		if row.RefRoomID.Valid && row.RoomName.Valid {
			r.Room = &response.InboxRoom{
				ID:   row.RefRoomID.Int64,
				Name: row.RoomName.String,
			}
		}
		return r
	}

	// conversation entry
	r.UnreadCount = row.UnreadCount
	if row.RefConversationID.Valid {
		convID := row.RefConversationID.Int64
		r.RefConversationID = &convID
	}
	if row.RoomName.Valid {
		r.Room = &response.InboxRoom{
			ID:   row.RefRoomID.Int64,
			Name: row.RoomName.String,
		}
	}
	if row.PeerUsername != "" {
		r.SourceUser = &response.InboxUser{
			ID:       row.PeerID,
			Username: row.PeerUsername,
		}
	}
	if row.LastMessageBody.Valid {
		r.LastMessage = &response.InboxLastMessage{
			Body:     row.LastMessageBody.String,
			SenderID: row.LastMessageSenderID.Int64,
		}
	}
	return r
}
