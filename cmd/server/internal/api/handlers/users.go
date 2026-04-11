package handlers

import (
	"log/slog"
	"net/http"

	"github.com/sleklere/realtime-chat/cmd/server/internal/api/dto/response"
	"github.com/sleklere/realtime-chat/cmd/server/internal/httpx"
	"github.com/sleklere/realtime-chat/cmd/server/internal/user"
)

// UserHandler handles user-related HTTP requests.
type UserHandler struct {
	logger  *slog.Logger
	userSvc *user.Service
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(l *slog.Logger, s *user.Service) *UserHandler {
	return &UserHandler{logger: l, userSvc: s}
}

// GetByUsername handles searching a user by username query param.
func (h *UserHandler) GetByUsername(w http.ResponseWriter, r *http.Request) error {
	username := r.URL.Query().Get("username")
	if username == "" {
		return httpx.BadRequest("missing_username", "username query param is required", nil)
	}

	u, err := h.userSvc.GetByUsername(r.Context(), username)
	if err != nil {
		return err
	}

	return httpx.JSON(w, http.StatusOK, response.UserRes{
		ID:        u.ID,
		Username:  u.Username,
		CreatedAt: u.CreatedAt.Time,
	})
}
