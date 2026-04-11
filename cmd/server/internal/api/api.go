// Package api contains the HTTP routing layer of the server, including
// middleware, endpoint handlers and helpers
package api

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/sleklere/realtime-chat/cmd/server/internal/api/handlers"
	"github.com/sleklere/realtime-chat/cmd/server/internal/auth"
	"github.com/sleklere/realtime-chat/cmd/server/internal/conversation"
	"github.com/sleklere/realtime-chat/cmd/server/internal/room"
	dbstore "github.com/sleklere/realtime-chat/cmd/server/internal/store"
	"github.com/sleklere/realtime-chat/cmd/server/internal/user"
	"github.com/sleklere/realtime-chat/cmd/server/internal/ws"
)

// API defines the HTTP API layer with logger and services used by handlers
type API struct {
	Logger     *slog.Logger
	AuthConfig *auth.Config
	Queries    *dbstore.Queries
	Hub        *ws.Hub

	AuthService         *auth.Service
	RoomService         *room.Service
	UserService         *user.Service
	ConversationService *conversation.Service
}

// RegisterAuthRoutes registers all authentication-related endpoints under /auth
func (a *API) registerAuthRoutes(r chi.Router) {
	h := handlers.NewAuthHandler(a.AuthService, a.Logger)
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", a.handle(h.Register))
		r.Post("/login", a.handle(h.Login))
	})
}

// registerRoomRoutes registers all room-related endpoints under /rooms
func (a *API) registerRoomRoutes(r chi.Router) {
	h := handlers.NewRoomHandler(a.Logger, a.Hub, a.RoomService)
	r.Route("/rooms", func(r chi.Router) {
		r.Post("/", a.handle(h.Create))
		r.Get("/", a.handle(h.List))
		r.Get("/{slug}", a.handle(h.GetBySlug))
		r.Post("/{roomID}/join", a.handle(h.Join))
		r.Delete("/{roomID}/leave", a.handle(h.Leave))
		r.Get("/{roomID}/messages", a.handle(h.Messages))
	})
}

func (a *API) registerUserRoutes(r chi.Router) {
	h := handlers.NewUserHandler(a.Logger, a.UserService)
	r.Route("/users", func(r chi.Router) {
		r.Get("/", a.handle(h.GetByUsername))
	})
}

func (a *API) registerConversationRoutes(r chi.Router) {
	h := handlers.NewConversationHandler(a.Logger, a.ConversationService)
	r.Route("/conversations", func(r chi.Router) {
		r.Get("/", a.handle(h.List))
		r.Get("/{conversationID}/messages", a.handle(h.ListMessages))
	})
}

// registerSystemRoutes registers system-level endpoints such as health checks
func (a *API) registerSystemRoutes(r chi.Router) {
	h := handlers.NewSystemHandler(a.Logger)
	r.Get("/healthz", a.handle(h.Health))
}
