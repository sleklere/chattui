package handlers

import (
	"log/slog"
	"net/http"

	"github.com/sleklere/chattui/cmd/server/internal/api/dto/response"
	"github.com/sleklere/chattui/cmd/server/internal/auth"
	"github.com/sleklere/chattui/cmd/server/internal/httpx"
	"github.com/sleklere/chattui/cmd/server/internal/inbox"
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
	for i, e := range rows {
		res[i] = toInboxRes(e)
	}
	return httpx.JSON(w, http.StatusOK, res)
}

func toInboxRes(e inbox.FeedEntry) response.InboxRes {
	r := response.InboxRes{
		EntryType: e.EntryType,
		CreatedAt: e.CreatedAt,
	}
	if e.EntryType == "event" {
		r.Kind = e.Kind
		r.SourceUser = &response.InboxUser{ID: e.SourceUserID, Username: e.SourceUsername}
		if e.RefRoomID != nil && e.RefRoomName != nil {
			r.Room = &response.InboxRoom{ID: *e.RefRoomID, Name: *e.RefRoomName}
		}
		return r
	}
	// conversation entry
	r.UnreadCount = e.UnreadCount
	r.RefConversationID = e.RefConversationID
	if e.RefRoomID != nil && e.RefRoomName != nil {
		r.Room = &response.InboxRoom{ID: *e.RefRoomID, Name: *e.RefRoomName}
	}
	if e.PeerUsername != "" {
		r.SourceUser = &response.InboxUser{ID: e.PeerID, Username: e.PeerUsername}
	}
	if e.LastMessageBody != nil {
		senderID := int64(0)
		if e.LastMessageSenderID != nil {
			senderID = *e.LastMessageSenderID
		}
		r.LastMessage = &response.InboxLastMessage{Body: *e.LastMessageBody, SenderID: senderID}
	}
	return r
}
