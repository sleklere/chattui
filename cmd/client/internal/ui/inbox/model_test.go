package inbox

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/sleklere/chattui/pkg/dto"
)

func convEntry(convID int64, peerUsername string, unread int64) dto.InboxFeed {
	return dto.InboxFeed{
		EntryType:         "conversation",
		RefConversationID: &convID,
		SourceUser:        &dto.InboxUser{ID: 1, Username: peerUsername},
		UnreadCount:       unread,
	}
}

func roomEntry(roomID int64, roomName string, unread int64) dto.InboxFeed {
	return dto.InboxFeed{
		EntryType:   "conversation",
		Room:        &dto.InboxRoom{ID: roomID, Name: roomName},
		UnreadCount: unread,
	}
}

func toItems(entries ...dto.InboxFeed) []list.Item {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = entryItem{entry: e}
	}
	return items
}

func entryAt(items []list.Item, i int) dto.InboxFeed {
	return items[i].(entryItem).entry
}

// ---------------------------------------------------------------------------
// sameEntry
// ---------------------------------------------------------------------------

func TestSameEntry_ConversationMatch(t *testing.T) {
	a := convEntry(42, "alice", 1)
	b := convEntry(42, "alice", 3)
	if !sameEntry(a, b) {
		t.Fatal("expected same entry for matching conversation_id")
	}
}

func TestSameEntry_ConversationNoMatch(t *testing.T) {
	a := convEntry(42, "alice", 1)
	b := convEntry(99, "bob", 1)
	if sameEntry(a, b) {
		t.Fatal("expected different entries for different conversation_id")
	}
}

func TestSameEntry_RoomMatch(t *testing.T) {
	a := roomEntry(10, "general", 0)
	b := roomEntry(10, "general", 5)
	if !sameEntry(a, b) {
		t.Fatal("expected same entry for matching room_id")
	}
}

func TestSameEntry_RoomNoMatch(t *testing.T) {
	a := roomEntry(10, "general", 0)
	b := roomEntry(20, "random", 0)
	if sameEntry(a, b) {
		t.Fatal("expected different entries for different room_id")
	}
}

func TestSameEntry_RoomVsConversation(t *testing.T) {
	a := roomEntry(10, "general", 0)
	b := convEntry(10, "alice", 0)
	if sameEntry(a, b) {
		t.Fatal("expected different entries for room vs conversation")
	}
}

// ---------------------------------------------------------------------------
// upsertEntry
// ---------------------------------------------------------------------------

func TestUpsertEntry_UpdateMovesToTop(t *testing.T) {
	items := toItems(
		convEntry(1, "alice", 0),
		convEntry(42, "bob", 1),
		roomEntry(10, "general", 0),
	)

	updated := convEntry(42, "bob", 3)
	result := upsertEntry(items, updated)

	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	top := entryAt(result, 0)
	if top.RefConversationID == nil || *top.RefConversationID != 42 {
		t.Fatalf("expected updated entry at top, got %+v", top)
	}
	if top.UnreadCount != 3 {
		t.Fatalf("expected unread_count=3, got %d", top.UnreadCount)
	}
}

func TestUpsertEntry_AddPrependsNewEntry(t *testing.T) {
	items := toItems(
		convEntry(1, "alice", 0),
		roomEntry(10, "general", 0),
	)

	newEntry := convEntry(99, "charlie", 1)
	result := upsertEntry(items, newEntry)

	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	top := entryAt(result, 0)
	if top.RefConversationID == nil || *top.RefConversationID != 99 {
		t.Fatalf("expected new entry at top, got %+v", top)
	}
}

func TestUpsertEntry_EmptyList(t *testing.T) {
	result := upsertEntry([]list.Item{}, convEntry(1, "alice", 1))
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
}

func TestUpsertEntry_PreservesOtherOrder(t *testing.T) {
	items := toItems(
		convEntry(1, "alice", 0),
		convEntry(2, "bob", 0),
		convEntry(42, "charlie", 0),
	)

	result := upsertEntry(items, convEntry(42, "charlie", 5))

	// 42 moves to top, 1 and 2 stay in relative order
	if entryAt(result, 0).RefConversationID == nil || *entryAt(result, 0).RefConversationID != 42 {
		t.Fatal("expected conv 42 at top")
	}
	if entryAt(result, 1).RefConversationID == nil || *entryAt(result, 1).RefConversationID != 1 {
		t.Fatal("expected conv 1 at index 1")
	}
	if entryAt(result, 2).RefConversationID == nil || *entryAt(result, 2).RefConversationID != 2 {
		t.Fatal("expected conv 2 at index 2")
	}
}
