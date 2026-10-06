package checkproto

import (
	"context"
	"encoding/json"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientRESTAndWebSocket(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		var input map[string]string
		if e := json.NewDecoder(r.Body).Decode(&input); e != nil || input["username"] != "alice" {
			t.Error("invalid register request")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"user": User{ID: 1, Username: "alice"}, "token": "test-token", "expires_at": time.Now().Add(time.Hour).Unix()})
	})
	mux.HandleFunc("/api/v1/rooms/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing auth header")
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(Room{ID: 7, Name: "test", Slug: "test"})
	})
	mux.HandleFunc("/api/v1/ws", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing WS header")
		}
		conn, e := websocket.Accept(w, r, nil)
		if e != nil {
			t.Error(e)
			return
		}
		defer conn.CloseNow()
		_, b, e := conn.Read(r.Context())
		if e != nil {
			return
		}
		var event Event
		if e = json.Unmarshal(b, &event); e != nil || event.Payload.Content != "body" || event.Payload.RoomID != 7 {
			t.Error("incorrect WS send", e)
		}
		out := `{"type":"room_message","payload":{"room_id":7,"sender_id":1,"message_id":3,"content":"body"}}`
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(out))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(srv.URL, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/v1/ws")
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	if e := c.Register(ctx, "alice", "private"); e != nil {
		t.Fatal(e)
	}
	room, e := c.CreateRoom(ctx, "test")
	if e != nil || room.ID != 7 {
		t.Fatal(room, e)
	}
	if e = c.Connect(ctx); e != nil {
		t.Fatal(e)
	}
	if e = c.Send(ctx, "room_message", 7, "body"); e != nil {
		t.Fatal(e)
	}
	select {
	case ob := <-c.Events():
		if ob.Event.Payload.MessageID != 3 {
			t.Fatal(ob)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
