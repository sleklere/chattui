package messagestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/jackc/pgx/v5/pgtype"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
)

type testMetadata struct {
	err      error
	username string
}

func (m *testMetadata) ValidateMessage(context.Context, dbstore.CreateMessageParams) error {
	return m.err
}
func (m *testMetadata) Username(context.Context, int64) (string, error) { return m.username, m.err }
func int8(id int64) pgtype.Int8                                         { return pgtype.Int8{Int64: id, Valid: true} }

func openTest(t *testing.T, path string, metadata Metadata) *Badger {
	t.Helper()
	s, err := Open(path, metadata)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !s.db.IsClosed() {
			if err := s.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	return s
}

func TestHistoryOrderLimitAndIsolation(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), &testMetadata{username: "alice"})
	for i := 1; i <= 12; i++ {
		m, err := s.CreateMessage(ctx, dbstore.CreateMessageParams{RoomID: int8(1), SenderID: 1, Body: fmt.Sprintf("message %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if m.ID != int64(i) || !m.CreatedAt.Valid || m.CreatedAt.Time.IsZero() {
			t.Fatalf("unexpected message: %+v", m)
		}
	}
	for _, p := range []dbstore.CreateMessageParams{
		{RoomID: int8(2), SenderID: 2, Body: "other room"},
		{RoomID: int8(256), SenderID: 2, Body: "binary room ID"},
		{ConversationID: int8(1), SenderID: 1, Body: "private"},
	} {
		if _, err := s.CreateMessage(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.ListMessagesByRoom(ctx, dbstore.ListMessagesByRoomParams{RoomID: int8(1), Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].ID != 12 || rows[1].ID != 11 || rows[2].ID != 10 || rows[0].SenderUsername != "alice" {
		t.Fatalf("unexpected history: %+v", rows)
	}
	dms, err := s.ListMessagesByConversation(ctx, dbstore.ListMessagesByConversationParams{ConversationID: int8(1), Limit: 100})
	if err != nil || len(dms) != 1 || dms[0].Body != "private" {
		t.Fatalf("DM history: %+v, %v", dms, err)
	}
	binaryRows, err := s.ListMessagesByRoom(ctx, dbstore.ListMessagesByRoomParams{RoomID: int8(256), Limit: 100})
	if err != nil || len(binaryRows) != 1 || binaryRows[0].Body != "binary room ID" {
		t.Fatalf("binary room ID history: %+v, %v", binaryRows, err)
	}
	for _, id := range []int64{3, 257} {
		rows, err := s.ListMessagesByRoom(ctx, dbstore.ListMessagesByRoomParams{RoomID: int8(id), Limit: 100})
		if err != nil || len(rows) != 0 {
			t.Fatalf("empty room: %+v, %v", rows, err)
		}
	}
	rows, err = s.ListMessagesByRoom(ctx, dbstore.ListMessagesByRoomParams{RoomID: int8(1), Limit: 0})
	if err != nil || len(rows) != 0 {
		t.Fatalf("zero limit: %+v, %v", rows, err)
	}
	if _, err = s.ListMessagesByRoom(ctx, dbstore.ListMessagesByRoomParams{RoomID: int8(1), Limit: -1}); err == nil {
		t.Fatal("negative limit accepted")
	}
}

func TestReopenPreservesMessagesAndCounter(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir()
	metadata := &testMetadata{username: "alice"}
	s := openTest(t, path, metadata)
	first, err := s.CreateMessage(ctx, dbstore.CreateMessageParams{RoomID: int8(1), SenderID: 1, Body: "durable"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, metadata); err == nil {
		t.Fatal("second owner acquired the database")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, path, metadata)
	second, err := s.CreateMessage(ctx, dbstore.CreateMessageParams{RoomID: int8(1), SenderID: 1, Body: "after restart"})
	if err != nil || second.ID != first.ID+1 {
		t.Fatalf("counter: %+v, %v", second, err)
	}
	rows, err := s.ListMessagesByRoom(ctx, dbstore.ListMessagesByRoomParams{RoomID: int8(1), Limit: 10})
	if err != nil || len(rows) != 2 || rows[1].Body != first.Body || !rows[1].CreatedAt.Time.Equal(first.CreatedAt.Time) {
		t.Fatalf("reopened history: %+v, %v", rows, err)
	}
}

func TestConcurrentMessageIDs(t *testing.T) {
	s := openTest(t, t.TempDir(), &testMetadata{})
	const count = 64
	ids := make(chan int64, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := s.CreateMessage(context.Background(), dbstore.CreateMessageParams{RoomID: int8(1), SenderID: 1, Body: "concurrent"})
			if err != nil {
				errs <- err
				return
			}
			ids <- m.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	unique := map[int64]bool{}
	for id := range ids {
		if unique[id] {
			t.Errorf("duplicate ID %d", id)
		}
		unique[id] = true
	}
	if len(unique) != count {
		t.Fatalf("persisted %d of %d", len(unique), count)
	}
	rows, err := s.ListMessagesByRoom(context.Background(), dbstore.ListMessagesByRoomParams{RoomID: int8(1), Limit: count})
	if err != nil || len(rows) != count {
		t.Fatalf("history count: %d, %v", len(rows), err)
	}
}

func TestWriteFailuresDoNotConsumeIDs(t *testing.T) {
	metadata := &testMetadata{err: errors.New("metadata unavailable")}
	s := openTest(t, t.TempDir(), metadata)
	ctx := context.Background()
	p := dbstore.CreateMessageParams{RoomID: int8(1), SenderID: 1, Body: "hello"}
	if _, err := s.CreateMessage(ctx, p); !errors.Is(err, metadata.err) {
		t.Fatalf("metadata failure: %v", err)
	}
	metadata.err = nil
	for _, invalid := range []dbstore.CreateMessageParams{
		{SenderID: 1}, {RoomID: int8(1), ConversationID: int8(1), SenderID: 1},
		{RoomID: int8(-1), SenderID: 1}, {RoomID: int8(1), SenderID: 0},
	} {
		if _, err := s.CreateMessage(ctx, invalid); err == nil {
			t.Fatal("invalid message accepted")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.CreateMessage(canceled, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled write: %v", err)
	}
	if _, err := s.ListMessagesByRoom(canceled, dbstore.ListMessagesByRoomParams{RoomID: int8(1), Limit: 10}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read: %v", err)
	}
	m, err := s.CreateMessage(ctx, p)
	if err != nil || m.ID != 1 {
		t.Fatalf("ID after failures: %+v, %v", m, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMessage(ctx, p); !errors.Is(err, badger.ErrDBClosed) {
		t.Fatalf("closed write: %v", err)
	}
}

// Run a separate process that acknowledges a write and exits without Close.
func TestRecoverAfterProcessExit(t *testing.T) {
	path := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashWriter$")
	cmd.Env = append(os.Environ(), "CHATTUI_TEST_CRASH_BADGER_PATH="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("writer: %v\n%s", err, output)
	}
	s := openTest(t, path, &testMetadata{username: "alice"})
	rows, err := s.ListMessagesByRoom(context.Background(), dbstore.ListMessagesByRoomParams{RoomID: int8(1), Limit: 10})
	if err != nil || len(rows) != 1 || rows[0].Body != "before exit" {
		t.Fatalf("recovered history: %+v, %v", rows, err)
	}
	m, err := s.CreateMessage(context.Background(), dbstore.CreateMessageParams{RoomID: int8(1), SenderID: 1, Body: "recovered"})
	if err != nil || m.ID != 2 {
		t.Fatalf("recovered counter: %+v, %v", m, err)
	}
}

func TestCrashWriter(t *testing.T) {
	path := os.Getenv("CHATTUI_TEST_CRASH_BADGER_PATH")
	if path == "" {
		t.Skip("subprocess only")
	}
	s, err := Open(path, &testMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMessage(context.Background(), dbstore.CreateMessageParams{RoomID: int8(1), SenderID: 1, Body: "before exit"}); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}
