// command server is the entrypoint for the chat server
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/sleklere/chattui/cmd/server/internal/api"
	"github.com/sleklere/chattui/cmd/server/internal/api/handlers"
	"github.com/sleklere/chattui/cmd/server/internal/auth"
	"github.com/sleklere/chattui/cmd/server/internal/bus"
	"github.com/sleklere/chattui/cmd/server/internal/conversation"
	"github.com/sleklere/chattui/cmd/server/internal/db"
	"github.com/sleklere/chattui/cmd/server/internal/inbox"
	"github.com/sleklere/chattui/cmd/server/internal/messagestore"
	"github.com/sleklere/chattui/cmd/server/internal/room"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
	"github.com/sleklere/chattui/cmd/server/internal/user"
	"github.com/sleklere/chattui/cmd/server/internal/ws"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system env")
	}

	ctx := context.Background()

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	pool, err := db.NewPool(ctx)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	err = pool.Ping(ctx)
	if err != nil {
		log.Fatalf("err pinging pool: %v", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}

	authCfg := &auth.Config{
		JWTSecret: []byte(jwtSecret),
		Issuer:    "chattui",
		AccessTTL: 15 * time.Minute,
	}

	bus := bus.NewBus(logger)

	queries := dbstore.New(pool)
	authSvc := auth.NewService(queries, logger, authCfg)
	var roomOptions []room.Option
	var conversationOptions []conversation.Option
	backend := getenv("MESSAGE_STORE", "postgres")
	switch backend {
	case "postgres":
	case "badger":
		path := getenv("BADGER_PATH", "data/messages.badger")
		messages, err := messagestore.Open(path, messagestore.NewPostgresMetadata(pool))
		if err != nil {
			panic(err)
		}
		defer func() {
			if err := messages.Close(); err != nil {
				logger.Error("closing message store", "error", err)
			}
		}()
		roomOptions = append(roomOptions, room.WithMessageStore(messages))
		conversationOptions = append(conversationOptions, conversation.WithMessageStore(messages))
		logger.Info("Badger message storage opened", "path", path)
	default:
		panic("unsupported MESSAGE_STORE: " + backend)
	}
	logger.Info("message storage selected", "backend", backend)
	roomSvc := room.NewService(queries, logger, bus, pool, roomOptions...)
	userSvc := user.NewService(queries, logger)
	convSvc := conversation.NewService(queries, logger, bus, pool, conversationOptions...)
	inboxSvc := inbox.NewService(bus, logger, queries)
	hub := ws.NewHub(bus, logger)
	go hub.Run()

	wsHandler := handlers.NewWSHandler(hub, roomSvc, convSvc, authCfg, logger)

	a := &api.API{
		Logger:     logger,
		AuthConfig: authCfg,
		Hub:        hub,
		WSHandler:  wsHandler,

		AuthService:         authSvc,
		RoomService:         roomSvc,
		UserService:         userSvc,
		ConversationService: convSvc,
		InboxService:        inboxSvc,
	}

	addr := ":" + getenv("PORT", "8080")

	r := api.NewRouter(a)

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  10 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
