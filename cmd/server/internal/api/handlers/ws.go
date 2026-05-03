package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/sleklere/chattui/cmd/server/internal/auth"
	"github.com/sleklere/chattui/cmd/server/internal/conversation"
	"github.com/sleklere/chattui/cmd/server/internal/httpx"
	"github.com/sleklere/chattui/cmd/server/internal/room"
	"github.com/sleklere/chattui/cmd/server/internal/ws"
)

// WSHandler handles WebSocket upgrade requests.
type WSHandler struct {
	hub             *ws.Hub
	roomSvc         *room.Service
	conversationSvc *conversation.Service
	authConfig      *auth.Config
	logger          *slog.Logger
}

// NewWSHandler creates a new WSHandler.
func NewWSHandler(hub *ws.Hub, roomSvc *room.Service, conversationSvc *conversation.Service, authConfig *auth.Config, logger *slog.Logger) *WSHandler {
	return &WSHandler{hub: hub, roomSvc: roomSvc, conversationSvc: conversationSvc, authConfig: authConfig, logger: logger}
}

// Upgrade handles the HTTP→WebSocket upgrade, authenticates via query param token, and starts the client pumps.
func (h *WSHandler) Upgrade(w http.ResponseWriter, r *http.Request) error {
	tokenQueryParam := r.URL.Query().Get("token")
	tokenHeader := r.Header.Get("Authorization")
	if tokenQueryParam == "" && tokenHeader == "" {
		return httpx.New(http.StatusUnauthorized, "missing_token", "missing token", errors.New("missing token"))
	}

	token := tokenQueryParam
	if tokenQueryParam == "" {
		token = strings.ReplaceAll(tokenHeader, "Bearer ", "")
	}

	claims, err := auth.ParseToken(token, h.authConfig)
	if err != nil {
		return httpx.New(http.StatusUnauthorized, "invalid_token", "invalid token", err)
	}

	// important to fetch rooms before accepting ws
	rooms, err := h.roomSvc.GetRoomsForUser(r.Context(), claims.UserID)
	if err != nil {
		return httpx.New(http.StatusInternalServerError, "rooms_error", "error fetching user rooms", err)
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	if err != nil {
		return httpx.New(http.StatusInternalServerError, "upgrade_error", "error upgrading handshake to ws conn", err)
	}

	roomIDs := make(map[int64]bool)
	for _, r := range rooms {
		roomIDs[r.ID] = true
	}

	client := ws.NewClient(h.hub, conn, h.roomSvc, h.conversationSvc, claims.UserID, claims.Username, roomIDs, h.logger)
	h.hub.Register(client)

	go client.WritePump(context.Background())
	client.ReadPump(context.Background())

	return nil
}
