# realtime-chat

Terminal-based real-time chat app. Rooms, direct messages, and WebSocket-powered messaging — all in the terminal.

## Stack

- **Server:** Go, [chi](https://github.com/go-chi/chi), PostgreSQL (pgx + sqlc + goose), WebSocket ([nhooyr.io/websocket](https://github.com/coder/websocket)), JWT auth
- **Client:** Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI

## Getting started

### Server (Docker — recommended)

```bash
cp .env.example .env   # set JWT_SECRET at minimum
docker-compose up -d
```

This starts PostgreSQL and the server (with migrations applied automatically) on port 8080.

### Server (manual)

```bash
cp .env.example .env   # fill in DB_URL, PORT, JWT_SECRET
docker-compose up -d postgres
make migrate
make run-server
```

### Client

```bash
make run-client
```

## Features

- Register / login with JWT auth
- Create rooms, join/leave
- Real-time room messaging via WebSocket
- Direct messages (1-to-1)
- Message history (REST)
- Multiple themes: Catppuccin, Rose-Pine, Kanagawa

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
      ui/          # Screens (auth, rooms, chat, dm)
```

## Architecture

The server follows a **DDD-lite** approach: bounded context packages (`auth`, `room`, `user`, `conversation`) contain business logic and define their own `Store` interfaces. The `api` package is the HTTP delivery layer — it translates requests into domain calls and responses. The `store` package is the persistence adapter (sqlc-generated, implements the Store interfaces).

Dependency direction: `api` → domain packages → `store`. Domain packages don't know about HTTP or the concrete DB layer.

**What's intentionally omitted:** domain packages use the sqlc-generated types directly (e.g., `dbstore.Room`) rather than defining their own domain types that would require a mapping layer. For a project of this scale, that extra indirection adds boilerplate without practical benefit.

## WebSocket protocol

All messages use an envelope: `{"type": "<type>", "payload": {...}, "timestamp": "<RFC3339>"}`.

Client → server types: `room_message`, `direct_message`, `join_room`, `leave_room`, `user_typing`.
