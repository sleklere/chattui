// Package checkproto exercises the public REST and WebSocket protocol without changing the server.
package checkproto

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}
type Room struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}
type Conversation struct {
	ID     int64 `json:"id"`
	PeerID int64 `json:"peer_id"`
}
type HistoryMessage struct {
	ID             int64  `json:"id"`
	RoomID         *int64 `json:"room_id"`
	ConversationID *int64 `json:"conversation_id"`
	SenderID       int64  `json:"sender_id"`
	Body           string `json:"body"`
}
type authResponse struct {
	User      User   `json:"user"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}
type Event struct {
	Type    string `json:"type"`
	Payload struct {
		RoomID     int64  `json:"room_id"`
		ToUserID   int64  `json:"to_user_id"`
		SenderID   int64  `json:"sender_id"`
		FromUserID int64  `json:"from_user_id"`
		Content    string `json:"content"`
		MessageID  int64  `json:"message_id"`
	} `json:"payload"`
}
type Observation struct {
	UserID int64     `json:"user_id"`
	Event  Event     `json:"event"`
	At     time.Time `json:"at"`
}

// Client keeps credentials in memory only. Never marshal Client or errors containing request headers.
type Client struct {
	HTTPURL, WSURL string
	HTTP           *http.Client
	token          string
	expiresAt      int64
	User           User
	mu             sync.Mutex
	conn           *websocket.Conn
	events         chan Observation
	readErr        chan error
	done           chan struct{}
	cancel         context.CancelFunc
}

func validateEndpoints(httpURL, wsURL string) error {
	for _, raw := range []string{httpURL, wsURL} {
		u, e := url.Parse(raw)
		if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("endpoint must not contain credentials, query or fragment")
		}
		host, _, e := net.SplitHostPort(u.Host)
		if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return errors.New("endpoint must bind numeric loopback address and port")
		}
	}
	return nil
}
func NewClient(httpURL, wsURL string) *Client {
	return &Client{HTTPURL: strings.TrimRight(httpURL, "/"), WSURL: wsURL, HTTP: &http.Client{Timeout: 5 * time.Second}}
}
func (c *Client) request(ctx context.Context, method, path string, body any, status int, out any) error {
	var r io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		r = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, c.HTTPURL+"/api/v1"+path, r)
	if e != nil {
		return e
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return fmt.Errorf("%s %s: %w", method, path, e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		return fmt.Errorf("%s %s: status %d (expected %d)", method, path, resp.StatusCode, status)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
	}
	return nil
}
func (c *Client) auth(ctx context.Context, route, username, password string) error {
	var a authResponse
	if e := c.request(ctx, "POST", route, map[string]string{"username": username, "password": password}, 200, &a); e != nil {
		return e
	}
	if a.User.ID <= 0 || a.User.Username != username || a.Token == "" || a.ExpiresAt <= time.Now().Unix() {
		return errors.New("invalid auth response or expired token")
	}
	c.User, c.token, c.expiresAt = a.User, a.Token, a.ExpiresAt
	return nil
}
func (c *Client) Register(ctx context.Context, username, password string) error {
	return c.auth(ctx, "/auth/register", username, password)
}
func (c *Client) Login(ctx context.Context, username, password string) error {
	return c.auth(ctx, "/auth/login", username, password)
}
func (c *Client) ValidFor(d time.Duration) error {
	if c.token == "" || time.Unix(c.expiresAt, 0).Before(time.Now().Add(d+5*time.Second)) {
		return errors.New("JWT expires before run and drain finish")
	}
	return nil
}
func (c *Client) CreateRoom(ctx context.Context, name string) (Room, error) {
	var v Room
	e := c.request(ctx, "POST", "/rooms/", map[string]string{"name": name}, 201, &v)
	return v, e
}
func (c *Client) JoinRoom(ctx context.Context, id int64) error {
	return c.request(ctx, "POST", "/rooms/"+strconv.FormatInt(id, 10)+"/join", nil, 204, nil)
}
func (c *Client) Lookup(ctx context.Context, name string) (User, error) {
	var v User
	e := c.request(ctx, "GET", "/users/?username="+url.QueryEscape(name), nil, 200, &v)
	return v, e
}
func (c *Client) Conversations(ctx context.Context) ([]Conversation, error) {
	var v []Conversation
	e := c.request(ctx, "GET", "/conversations/?limit=100", nil, 200, &v)
	return v, e
}
func (c *Client) History(ctx context.Context, kind string, id int64) ([]HistoryMessage, error) {
	if kind != "rooms" && kind != "conversations" {
		return nil, errors.New("invalid history kind")
	}
	var v []HistoryMessage
	e := c.request(ctx, "GET", "/"+kind+"/"+strconv.FormatInt(id, 10)+"/messages?limit=100", nil, 200, &v)
	return v, e
}
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return errors.New("already connected")
	}
	reqHeader := http.Header{}
	reqHeader.Set("Authorization", "Bearer "+c.token)
	conn, _, e := websocket.Dial(ctx, c.WSURL, &websocket.DialOptions{HTTPHeader: reqHeader})
	if e != nil {
		return fmt.Errorf("websocket dial failed: %w", e)
	}
	readCtx, cancel := context.WithCancel(context.Background())
	c.conn = conn
	c.cancel = cancel
	c.events = make(chan Observation, 4096)
	c.readErr = make(chan error, 1)
	c.done = make(chan struct{})
	events, readErr, done, userID := c.events, c.readErr, c.done, c.User.ID
	go func() {
		defer close(done)
		defer close(events)
		for {
			_, b, err := conn.Read(readCtx)
			if err != nil {
				if readCtx.Err() == nil {
					readErr <- fmt.Errorf("websocket read failed: %w", err)
				}
				return
			}
			var v Event
			if err = json.Unmarshal(b, &v); err != nil {
				readErr <- fmt.Errorf("websocket invalid event: %w", err)
				return
			}
			select {
			case events <- Observation{UserID: userID, Event: v, At: time.Now()}:
			default:
				readErr <- errors.New("observation buffer overflow")
				return
			}
		}
	}()
	return nil
}
func (c *Client) Events() <-chan Observation { return c.events }
func (c *Client) ReadError() <-chan error    { return c.readErr }
func (c *Client) Send(ctx context.Context, kind string, target int64, body string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("not connected")
	}
	var payload any
	switch kind {
	case "room_message":
		payload = map[string]any{"room_id": target, "content": body}
	case "direct_message":
		payload = map[string]any{"to_user_id": target, "content": body}
	default:
		return errors.New("unknown message type")
	}
	b, e := json.Marshal(map[string]any{"type": kind, "payload": payload, "timestamp": time.Now().UTC()})
	if e != nil {
		return e
	}
	return c.conn.Write(ctx, websocket.MessageText, b)
}
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.cancel()
		_ = c.conn.CloseNow()
		<-c.done
		c.conn = nil
	}
}
