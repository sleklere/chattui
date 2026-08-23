# chattui

Terminal-based real-time chat app. Rooms, direct messages, and WebSocket-powered messaging — all in the terminal.

## Stack

- **Server:** Go, [chi](https://github.com/go-chi/chi), PostgreSQL (pgx + sqlc + goose), WebSocket ([nhooyr.io/websocket](https://github.com/coder/websocket)), JWT auth
- **Client:** Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI

## Getting started

Requires Docker. No Go or PostgreSQL needed.

```bash
cp .env.example .env   # set JWT_SECRET at minimum
docker compose up -d --build           # PostgreSQL + server (migrations auto) on :8080
docker compose run --rm client         # TUI client
```

For local development against a running Go toolchain, see the `Makefile`.

## Features

- Register / login with JWT auth
- Create rooms, join/leave
- Real-time room messaging via WebSocket
- Direct messages (1-to-1)
- Message history (REST)
- Notification inbox (room events + unread message counts)
- Multiple themes: Catppuccin, Rose-Pine, Kanagawa (`t` on the rooms screen)
- Shared app frame: tab bar with unread badges, live connection indicator, contextual key bar
- Grouped message transcript with per-user colors, day separators and word wrap
- Filterable lists (`/`), key reference overlay (`?`), empty and loading states

## Project layout

```
cmd/
  server/          # HTTP + WebSocket server
    internal/
      api/         # Delivery layer: router, middleware, handlers, DTOs
      auth/        # Bounded context: JWT auth, bcrypt, domain errors
      room/        # Bounded context: rooms, membership
      user/        # Bounded context: user lookup
      conversation/ # Bounded context: DM conversations
      ws/          # WebSocket hub + client dispatch
      store/       # sqlc-generated DB layer (persistence adapter)
      httpx/       # HTTP helpers (error types, JSON writer)
      db/          # DB connection pool
    migrations/    # goose migrations
    queries/       # SQL query files
  client/          # Bubble Tea TUI
    internal/
      api/         # HTTP client
      ws/          # WebSocket client
      ui/          # Screens (auth, rooms, chat, dm, dmchat, inbox)
        hud/       # Persistent frame: top bar, key bar, modals, help overlay
        components/# List rows, badges, empty states, rules
        chatview/  # Message history + composer rendering
        theme/     # Color palettes
```

## Architecture

The server follows a **DDD-lite** approach: bounded context packages (`auth`, `room`, `user`, `conversation`) contain business logic and define their own `Store` interfaces. The `api` package is the HTTP delivery layer — it translates requests into domain calls and responses. The `store` package is the persistence adapter (sqlc-generated, implements the Store interfaces).

Dependency direction: `api` → domain packages → `store`. Domain packages don't know about HTTP or the concrete DB layer.

**What's intentionally omitted:** domain packages use the sqlc-generated types directly (e.g., `dbstore.Room`) rather than defining their own domain types that would require a mapping layer. For a project of this scale, that extra indirection adds boilerplate without practical benefit.

