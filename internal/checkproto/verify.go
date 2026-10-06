package checkproto

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Expected has a unique body and exactly the recipients that must observe it live.
// Recipients can be empty when everyone is offline; history is checked separately.
type Expected struct {
	Type       string    `json:"type"`
	Body       string    `json:"body"`
	SenderID   int64     `json:"sender_id"`
	TargetID   int64     `json:"target_id"`
	Recipients []int64   `json:"recipients"`
	SentAt     time.Time `json:"sent_at"`
	Deadline   time.Time `json:"deadline,omitempty"`
	MessageID  int64     `json:"message_id,omitempty"`
}
type Check struct {
	Missing        []string        `json:"missing,omitempty"`
	Duplicate      []string        `json:"duplicate,omitempty"`
	WrongRecipient []string        `json:"wrong_recipient,omitempty"`
	Mismatch       []string        `json:"mismatch,omitempty"`
	Unknown        []string        `json:"unknown,omitempty"`
	Correlated     int             `json:"correlated"`
	Latencies      []time.Duration `json:"-"`
}

func (c Check) Err() error {
	if len(c.Missing)+len(c.Duplicate)+len(c.WrongRecipient)+len(c.Mismatch)+len(c.Unknown) > 0 {
		return fmt.Errorf("protocol verification: missing=%v duplicate=%v wrong_recipient=%v mismatch=%v unknown=%v", c.Missing, c.Duplicate, c.WrongRecipient, c.Mismatch, c.Unknown)
	}
	return nil
}

// Verify checks every observation up to the end of the caller's bounded collection window.
// It does not trust the WS timestamp: the clock is the runner's receive time.
func Verify(expected []Expected, observations []Observation) Check {
	out := Check{}
	byBody := map[string]*Expected{}
	seen := map[string]map[int64]bool{}
	ids := map[int64]string{}
	for i := range expected {
		x := &expected[i]
		if x.Body == "" || byBody[x.Body] != nil {
			out.Mismatch = append(out.Mismatch, "non-unique expected body")
			continue
		}
		byBody[x.Body] = x
		seen[x.Body] = map[int64]bool{}
	}
	for _, o := range observations {
		p := o.Event.Payload
		if o.Event.Type != "room_message" && o.Event.Type != "direct_message" {
			continue
		}
		x := byBody[p.Content]
		if x == nil {
			out.Unknown = append(out.Unknown, p.Content)
			continue
		}
		if seen[x.Body][o.UserID] {
			out.Duplicate = append(out.Duplicate, x.Body)
			continue
		}
		seen[x.Body][o.UserID] = true
		allowed := false
		for _, id := range x.Recipients {
			if id == o.UserID {
				allowed = true
				break
			}
		}
		if !allowed {
			out.WrongRecipient = append(out.WrongRecipient, x.Body)
			continue
		}
		senderID := p.SenderID
		if x.Type == "direct_message" {
			senderID = p.FromUserID
		}
		if (!x.Deadline.IsZero() && o.At.After(x.Deadline)) || x.Type != o.Event.Type || p.MessageID <= 0 || senderID != x.SenderID || (x.Type == "room_message" && p.RoomID != x.TargetID) || (x.Type == "direct_message" && p.ToUserID != x.TargetID) || (x.MessageID != 0 && x.MessageID != p.MessageID) {
			out.Mismatch = append(out.Mismatch, x.Body)
			continue
		}
		if old := ids[p.MessageID]; old != "" && old != x.Body {
			out.Mismatch = append(out.Mismatch, "message ID reused")
		}
		ids[p.MessageID] = x.Body
		x.MessageID = p.MessageID
		out.Correlated++
		out.Latencies = append(out.Latencies, o.At.Sub(x.SentAt))
	}
	for _, x := range expected {
		for _, id := range x.Recipients {
			if !seen[x.Body][id] {
				out.Missing = append(out.Missing, fmt.Sprintf("%s recipient %d", x.Body, id))
			}
		}
	}
	return out
}

// VerifyHistory asserts the complete small E2E set. Under load use VerifyRecentHistory instead.
func VerifyHistory(messages []HistoryMessage, expected []Expected, kind string, target int64) error {
	if len(expected) > 100 {
		return errors.New("full history requires at most 100 expected messages")
	}
	if len(messages) != len(expected) {
		return fmt.Errorf("history count %d, expected %d", len(messages), len(expected))
	}
	byBody := map[string]Expected{}
	for _, x := range expected {
		if x.MessageID <= 0 {
			return fmt.Errorf("no server ID for %s", x.Body)
		}
		byBody[x.Body] = x
	}
	ids := map[int64]bool{}
	for _, m := range messages {
		x, ok := byBody[m.Body]
		if !ok || ids[m.ID] || m.ID != x.MessageID || m.SenderID != x.SenderID || (kind == "rooms" && (m.RoomID == nil || *m.RoomID != target)) || (kind == "conversations" && (m.ConversationID == nil || *m.ConversationID != target)) {
			return fmt.Errorf("invalid history entry ID %d", m.ID)
		}
		ids[m.ID] = true
	}
	return nil
}

// VerifyRecentHistory compares only the last n expected entries. It cannot claim
// completeness of older messages because REST exposes no cursor beyond 100.
func VerifyRecentHistory(messages []HistoryMessage, expected []Expected, kind string, target int64) error {
	if len(expected) > 100 {
		expected = expected[len(expected)-100:]
	}
	if len(messages) > len(expected) {
		messages = messages[:len(expected)]
	}
	return VerifyHistory(messages, expected, kind, target)
}
func Percentiles(values []time.Duration) map[string]time.Duration {
	out := map[string]time.Duration{}
	if len(values) == 0 {
		return out
	}
	a := append([]time.Duration(nil), values...)
	sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
	for _, p := range []struct {
		name string
		rank int
	}{{"p50", 50}, {"p95", 95}, {"p99", 99}} {
		i := (len(a)*p.rank+99)/100 - 1
		out[p.name] = a[i]
	}
	return out
}
