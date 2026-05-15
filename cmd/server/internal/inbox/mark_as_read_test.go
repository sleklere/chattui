package inbox

import (
	"context"
	"errors"
	"testing"

	"github.com/sleklere/chattui/cmd/server/internal/errs"
	dbstore "github.com/sleklere/chattui/cmd/server/internal/store"
	"github.com/sleklere/chattui/cmd/server/internal/testhelper"
)

type stubStore struct {
	resetDMCalled   bool
	resetRoomCalled bool
	resetAllCalled  bool
}

func (s *stubStore) SaveInboxRoomEvent(_ context.Context, _ dbstore.SaveInboxRoomEventParams) error {
	return nil
}
func (s *stubStore) UpsertInboxConversationCursor(_ context.Context, _ dbstore.UpsertInboxConversationCursorParams) error {
	return nil
}
func (s *stubStore) UpdateInboxCursorOnRoomMessage(_ context.Context, _ dbstore.UpdateInboxCursorOnRoomMessageParams) ([]dbstore.UpdateInboxCursorOnRoomMessageRow, error) {
	return nil, nil
}
func (s *stubStore) UpdateInboxCursorOnDMMessage(_ context.Context, _ dbstore.UpdateInboxCursorOnDMMessageParams) ([]dbstore.UpdateInboxCursorOnDMMessageRow, error) {
	return nil, nil
}
func (s *stubStore) ListInboxFeed(_ context.Context, _ dbstore.ListInboxFeedParams) ([]dbstore.ListInboxFeedRow, error) {
	return nil, nil
}
func (s *stubStore) ResetRoomUnreadCount(_ context.Context, _ dbstore.ResetRoomUnreadCountParams) error {
	s.resetRoomCalled = true
	return nil
}
func (s *stubStore) ResetDMUnreadCount(_ context.Context, _ dbstore.ResetDMUnreadCountParams) error {
	s.resetDMCalled = true
	return nil
}
func (s *stubStore) ResetAllUnreadCount(_ context.Context, _ int64) error {
	s.resetAllCalled = true
	return nil
}

func newStubSvc(store *stubStore) *Service {
	return &Service{
		bus:    &testhelper.CaptureBus{},
		logger: testhelper.DiscardLogger(),
		store:  store,
	}
}

func ptrInt64(v int64) *int64 { return &v }

func TestMarkAsRead_Validation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		conversationID *int64
		roomID         *int64
		all            bool
		wantErr        error
	}{
		{"no params", nil, nil, false, errs.ErrNoParams},
		{"conversation + room", ptrInt64(1), ptrInt64(2), false, errs.ErrAmbiguousParams},
		{"conversation + all", ptrInt64(1), nil, true, errs.ErrAmbiguousParams},
		{"room + all", nil, ptrInt64(1), true, errs.ErrAmbiguousParams},
		{"all three", ptrInt64(1), ptrInt64(2), true, errs.ErrAmbiguousParams},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := newStubSvc(&stubStore{}).MarkAsRead(ctx, tc.conversationID, tc.roomID, tc.all, 1)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestMarkAsRead_DispatchesToCorrectStore(t *testing.T) {
	ctx := context.Background()

	t.Run("conversation_id calls ResetDMUnreadCount", func(t *testing.T) {
		store := &stubStore{}
		if err := newStubSvc(store).MarkAsRead(ctx, ptrInt64(1), nil, false, 99); err != nil {
			t.Fatal(err)
		}
		if !store.resetDMCalled {
			t.Error("expected ResetDMUnreadCount to be called")
		}
	})

	t.Run("room_id calls ResetRoomUnreadCount", func(t *testing.T) {
		store := &stubStore{}
		if err := newStubSvc(store).MarkAsRead(ctx, nil, ptrInt64(2), false, 99); err != nil {
			t.Fatal(err)
		}
		if !store.resetRoomCalled {
			t.Error("expected ResetRoomUnreadCount to be called")
		}
	})

	t.Run("all calls ResetAllUnreadCount", func(t *testing.T) {
		store := &stubStore{}
		if err := newStubSvc(store).MarkAsRead(ctx, nil, nil, true, 99); err != nil {
			t.Fatal(err)
		}
		if !store.resetAllCalled {
			t.Error("expected ResetAllUnreadCount to be called")
		}
	})
}
