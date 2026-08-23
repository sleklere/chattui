package chatview

import (
	"strings"
	"testing"
	"time"
)

func TestRenderGroupsConsecutiveMessagesFromSameSender(t *testing.T) {
	now := time.Now()
	msgs := []Message{
		{SenderID: 1, Sender: "ada", Body: "first", At: now.Add(-2 * time.Minute)},
		{SenderID: 1, Sender: "ada", Body: "second", At: now.Add(-time.Minute)},
		{SenderID: 2, Sender: "santi", Body: "third", At: now},
	}

	out := Render(msgs, 2, 60)

	if got := strings.Count(out, "ada"); got != 1 {
		t.Errorf("sender header rendered %d times, want 1 for a grouped block", got)
	}
	if !strings.Contains(out, "you") {
		t.Error("own message should be labelled 'you'")
	}
	if !strings.Contains(out, "Today") {
		t.Error("expected a day separator for today's messages")
	}
}

func TestRenderSplitsBlocksAfterTheGroupWindow(t *testing.T) {
	now := time.Now()
	msgs := []Message{
		{SenderID: 1, Sender: "ada", Body: "first", At: now.Add(-time.Hour)},
		{SenderID: 1, Sender: "ada", Body: "second", At: now},
	}

	if got := strings.Count(Render(msgs, 2, 60), "ada"); got != 2 {
		t.Errorf("sender header rendered %d times, want 2 for messages an hour apart", got)
	}
}

func TestRenderInsertsDaySeparators(t *testing.T) {
	now := time.Now()
	msgs := []Message{
		{SenderID: 1, Sender: "ada", Body: "old", At: now.AddDate(0, 0, -1)},
		{SenderID: 1, Sender: "ada", Body: "new", At: now},
	}

	out := Render(msgs, 2, 60)
	if !strings.Contains(out, "Yesterday") || !strings.Contains(out, "Today") {
		t.Errorf("expected Yesterday and Today separators, got:\n%s", out)
	}
}
