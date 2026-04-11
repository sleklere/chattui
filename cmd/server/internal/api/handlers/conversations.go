package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sleklere/realtime-chat/cmd/server/internal/api/dto/response"
	"github.com/sleklere/realtime-chat/cmd/server/internal/auth"
	"github.com/sleklere/realtime-chat/cmd/server/internal/conversation"
	"github.com/sleklere/realtime-chat/cmd/server/internal/httpx"
)

// ConversationHandler handles conversation-related HTTP requests.
type ConversationHandler struct {
	logger          *slog.Logger
	conversationSvc *conversation.Service
}

// NewConversationHandler creates a new ConversationHandler.
func NewConversationHandler(l *slog.Logger, s *conversation.Service) *ConversationHandler {
	return &ConversationHandler{logger: l, conversationSvc: s}
}

// List handles listing all conversations for the authenticated user.
func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) error {
	claims, _ := auth.ClaimsFromCtx(r.Context())

	convs, err := h.conversationSvc.ListByUser(r.Context(), claims.UserID, parseLimit(r))
	if err != nil {
		return err
	}

	res := make([]response.ConversationRes, len(convs))
	for i, c := range convs {
		res[i] = response.ConversationRes{
			ID:           c.ID,
			PeerID:       c.PeerID,
			PeerUsername: c.PeerUsername,
		}
	}
	return httpx.JSON(w, http.StatusOK, res)
}

// ListMessages handles fetching paginated message history for a conversation.
func (h *ConversationHandler) ListMessages(w http.ResponseWriter, r *http.Request) error {
	conversationID, err := strconv.ParseInt(chi.URLParam(r, "conversationID"), 10, 64)
	if err != nil {
		return httpx.BadRequest("invalid_conversation_id", "invalid conversation id", err)
	}

	msgs, err := h.conversationSvc.ListMessages(r.Context(), conversationID, parseLimit(r))
	if err != nil {
		return err
	}

	res := make([]response.ConversationMessageRes, len(msgs))
	for i, m := range msgs {
		res[i] = response.ConversationMessageRes{
			MessageRes: response.MessageRes{
				ID:        m.ID,
				SenderID:  m.SenderID,
				Body:      m.Body,
				CreatedAt: m.CreatedAt.Time,
			},
			ConversationID: m.ConversationID.Int64,
		}
	}
	return httpx.JSON(w, http.StatusOK, res)
}
