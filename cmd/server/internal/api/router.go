// Package api contains the HTTP routing layer of the server, including
// middleware and endpoint handlers
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sleklere/realtime-chat/cmd/server/internal/api/handlers"
)

// NewRouter initializes the HTTP router with default middlewares and base routes.
func NewRouter(a *API) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	wsHandler := handlers.NewWSHandler(a.Hub, a.Queries, a.AuthConfig, a.Logger)
	r.Get("/api/v1/ws", a.handle(wsHandler.Upgrade))

	// group routes to be able to separate timeout middleware from ws endpoint
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(60e9)) // 1min

		a.registerSystemRoutes(r)

		r.Route("/api/v1", func(api chi.Router) {
			a.registerAuthRoutes(api)

			api.Group(func(protectedRouter chi.Router) {
				protectedRouter.Use(a.validateJWT)
				a.registerRoomRoutes(protectedRouter)
				a.registerUserRoutes(protectedRouter)
				a.registerConversationRoutes(protectedRouter)
			})
		})
	})

	return r
}
