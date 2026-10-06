package checkproto

import (
	"testing"
	"time"
)

func TestVerifyFailures(t *testing.T) {
	now := time.Now()
	sample := Expected{Type: "room_message", Body: "unique", SenderID: 1, TargetID: 9, Recipients: []int64{1, 2}, SentAt: now}
	event := func(user, sender, target, id int64, body string) Observation {
		o := Observation{UserID: user, At: now.Add(time.Millisecond)}
		o.Event.Type = "room_message"
		o.Event.Payload.SenderID = sender
		o.Event.Payload.RoomID = target
		o.Event.Payload.MessageID = id
		o.Event.Payload.Content = body
		return o
	}
	tests := []struct {
		name         string
		observations []Observation
		field        string
	}{{"missing", []Observation{event(1, 1, 9, 5, "unique")}, "missing"}, {"duplicate", []Observation{event(1, 1, 9, 5, "unique"), event(1, 1, 9, 5, "unique"), event(2, 1, 9, 5, "unique")}, "duplicate"}, {"wrong recipient", []Observation{event(1, 1, 9, 5, "unique"), event(2, 1, 9, 5, "unique"), event(3, 1, 9, 5, "unique")}, "wrong"}, {"wrong sender", []Observation{event(1, 9, 9, 5, "unique"), event(2, 1, 9, 5, "unique")}, "mismatch"}, {"late data", []Observation{event(1, 1, 9, 5, "unique"), event(2, 1, 9, 5, "unique")}, "mismatch"}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := sample
			if tc.name == "late data" {
				x.Deadline = now.Add(time.Microsecond)
			}
			v := Verify([]Expected{x}, tc.observations)
			if v.Err() == nil {
				t.Fatal("false positive", v)
			}
			switch tc.field {
			case "missing":
				if len(v.Missing) == 0 {
					t.Fatal(v)
				}
			case "duplicate":
				if len(v.Duplicate) == 0 {
					t.Fatal(v)
				}
			case "wrong":
				if len(v.WrongRecipient) == 0 {
					t.Fatal(v)
				}
			case "mismatch":
				if len(v.Mismatch) == 0 {
					t.Fatal(v)
				}
			}
		})
	}
	good := Verify([]Expected{sample}, []Observation{event(1, 1, 9, 5, "unique"), event(2, 1, 9, 5, "unique")})
	if e := good.Err(); e != nil {
		t.Fatal(e)
	}
}
func TestVerifyHistoryStrict(t *testing.T) {
	room := int64(9)
	expected := []Expected{{Body: "a", MessageID: 7, SenderID: 1}}
	for _, m := range [][]HistoryMessage{{}, {{ID: 7, RoomID: &room, SenderID: 2, Body: "a"}}, {{ID: 7, RoomID: &room, SenderID: 1, Body: "a"}, {ID: 7, RoomID: &room, SenderID: 1, Body: "a"}}} {
		if VerifyHistory(m, expected, "rooms", room) == nil {
			t.Fatalf("accepted invalid history: %+v", m)
		}
	}
	if e := VerifyHistory([]HistoryMessage{{ID: 7, RoomID: &room, SenderID: 1, Body: "a"}}, expected, "rooms", room); e != nil {
		t.Fatal(e)
	}
}
func TestLoadConfigBounds(t *testing.T) {
	c := LoadConfig{HTTPURL: "http://127.0.0.1:8080", WSURL: "ws://127.0.0.1:8080/api/v1/ws", ResultsDir: t.TempDir(), Profile: RoomProfile, Users: 2, Rate: 1, Duration: time.Second, Drain: time.Second, Steps: 1, MaxRate: 1, MaxDuration: time.Second}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.MaxDuration = 15 * time.Minute
	if c.Validate() == nil {
		t.Fatal("accepted unbounded duration")
	}
	c.MaxDuration = time.Second
	c.Rate = 200
	if c.Validate() == nil {
		t.Fatal("accepted excessive rate")
	}
}
