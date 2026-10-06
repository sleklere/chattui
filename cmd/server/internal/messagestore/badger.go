package messagestore

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/jackc/pgx/v5/pgtype"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

var counterKey = []byte("meta/v1/last-message-id")

// Metadata validates references and resolves current usernames in PostgreSQL.
// These checks and a Badger commit cannot form a single transaction.
type Metadata interface {
	ValidateMessage(context.Context, dbstore.CreateMessageParams) error
	Username(context.Context, int64) (string, error)
}

// Badger stores messages on disk. One process owns its directory. The mutex
// serializes ID allocation and insertion; both are persisted in one transaction.
type Badger struct {
	db       *badger.DB
	metadata Metadata
	writeMu  sync.Mutex
}

var _ Store = (*Badger)(nil)

// Open enables synchronous writes so acknowledged commits are flushed to disk.
func Open(path string, metadata Metadata) (*Badger, error) {
	if path == "" || metadata == nil {
		return nil, errors.New("badger: directory and metadata are required")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, fmt.Errorf("badger: create directory: %w", err)
	}
	db, err := badger.Open(badger.DefaultOptions(path).WithSyncWrites(true))
	if err != nil {
		return nil, fmt.Errorf("badger: open: %w", err)
	}
	return &Badger{db: db, metadata: metadata}, nil
}

func (s *Badger) Close() error { return s.db.Close() }

// record is the on-disk v1 format, independent of pgx's nullable types.
type record struct {
	ID             int64     `json:"id"`
	RoomID         int64     `json:"room_id,omitempty"`
	ConversationID int64     `json:"conversation_id,omitempty"`
	SenderID       int64     `json:"sender_id"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}

func (r record) message() dbstore.Message {
	return dbstore.Message{
		ID: r.ID, RoomID: pgtype.Int8{Int64: r.RoomID, Valid: r.RoomID != 0},
		ConversationID: pgtype.Int8{Int64: r.ConversationID, Valid: r.ConversationID != 0},
		SenderID:       r.SenderID, Body: r.Body,
		CreatedAt: pgtype.Timestamptz{Time: r.CreatedAt, Valid: true},
	}
}

// Binary big-endian IDs preserve numeric order during lexicographic iteration.
func chatPrefix(kind byte, id int64) []byte {
	prefix := append([]byte("message/v1/"), kind)
	return binary.BigEndian.AppendUint64(prefix, uint64(id))
}

func (s *Badger) CreateMessage(ctx context.Context, p dbstore.CreateMessageParams) (dbstore.Message, error) {
	if err := ctx.Err(); err != nil {
		return dbstore.Message{}, err
	}
	if p.SenderID <= 0 || p.RoomID.Valid == p.ConversationID.Valid ||
		(p.RoomID.Valid && p.RoomID.Int64 <= 0) || (p.ConversationID.Valid && p.ConversationID.Int64 <= 0) {
		return dbstore.Message{}, errors.New("badger: message requires a sender and exactly one positive chat ID")
	}
	if err := s.metadata.ValidateMessage(ctx, p); err != nil {
		return dbstore.Message{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var r record
	err := s.db.Update(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var last uint64
		item, err := tx.Get(counterKey)
		if err == nil {
			err = item.Value(func(v []byte) error {
				if len(v) != 8 {
					return errors.New("badger: invalid message counter")
				}
				last = binary.BigEndian.Uint64(v)
				return nil
			})
		}
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if last >= math.MaxInt64 {
			return errors.New("badger: message IDs exhausted")
		}
		r = record{ID: int64(last + 1), SenderID: p.SenderID, Body: p.Body, CreatedAt: time.Now().UTC()}
		var prefix []byte
		if p.RoomID.Valid {
			r.RoomID = p.RoomID.Int64
			prefix = chatPrefix('r', r.RoomID)
		} else {
			r.ConversationID = p.ConversationID.Int64
			prefix = chatPrefix('c', r.ConversationID)
		}
		value, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if err := tx.Set(binary.BigEndian.AppendUint64(prefix, uint64(r.ID)), value); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return tx.Set(counterKey, binary.BigEndian.AppendUint64(nil, uint64(r.ID)))
	})
	if err != nil {
		return dbstore.Message{}, fmt.Errorf("badger: create message: %w", err)
	}
	return r.message(), nil
}

func (s *Badger) list(ctx context.Context, kind byte, id pgtype.Int8, limit int32) ([]dbstore.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, errors.New("badger: negative limit")
	}
	messages := make([]dbstore.Message, 0)
	if !id.Valid || limit == 0 {
		return messages, nil
	}
	if id.Int64 <= 0 {
		return nil, errors.New("badger: invalid chat ID")
	}
	prefix := chatPrefix(kind, id.Int64)
	err := s.db.View(func(tx *badger.Txn) error {
		options := badger.DefaultIteratorOptions
		options.Reverse = true
		options.Prefix = prefix
		it := tx.NewIterator(options)
		defer it.Close()
		// Seek past every possible message ID in this chat, not into the next chat.
		upper := binary.BigEndian.AppendUint64(append([]byte(nil), prefix...), math.MaxUint64)
		for it.Seek(upper); it.ValidForPrefix(prefix) && len(messages) < int(limit); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var r record
			if err := it.Item().Value(func(v []byte) error { return json.Unmarshal(v, &r) }); err != nil {
				return err
			}
			messages = append(messages, r.message())
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("badger: list messages: %w", err)
	}
	return messages, nil
}

func (s *Badger) ListMessagesByConversation(ctx context.Context, p dbstore.ListMessagesByConversationParams) ([]dbstore.Message, error) {
	return s.list(ctx, 'c', p.ConversationID, p.Limit)
}

func (s *Badger) ListMessagesByRoom(ctx context.Context, p dbstore.ListMessagesByRoomParams) ([]dbstore.ListMessagesByRoomRow, error) {
	messages, err := s.list(ctx, 'r', p.RoomID, p.Limit)
	if err != nil {
		return nil, err
	}
	rows := make([]dbstore.ListMessagesByRoomRow, 0, len(messages))
	usernames := make(map[int64]string)
	for _, m := range messages {
		username, ok := usernames[m.SenderID]
		if !ok {
			username, err = s.metadata.Username(ctx, m.SenderID)
			if err != nil {
				return nil, err
			}
			usernames[m.SenderID] = username
		}
		rows = append(rows, dbstore.ListMessagesByRoomRow{ID: m.ID, RoomID: m.RoomID, SenderID: m.SenderID, Body: m.Body, CreatedAt: m.CreatedAt, SenderUsername: username})
	}
	return rows, nil
}
