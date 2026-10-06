package checkproto

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestLoadRoomMembershipBounds(t *testing.T) {
	c := LoadConfig{HTTPURL: "http://127.0.0.1:8080", WSURL: "ws://127.0.0.1:8080/api/v1/ws", ResultsDir: t.TempDir(), Profile: RoomProfile, Users: 4, Rate: 2, MaxRate: 2, Duration: time.Second, MaxDuration: time.Second, Drain: time.Second, Steps: 1}
	for _, tc := range []struct {
		rooms, members int
		valid          bool
	}{{0, 0, true}, {2, 2, true}, {16, 4, false}, {-1, 2, false}, {17, 2, false}, {2, -1, false}, {2, 1, false}, {2, 5, false}} {
		c.Rooms = tc.rooms
		c.MembersPerRoom = tc.members
		if (c.Validate() == nil) != tc.valid {
			t.Errorf("rooms=%d members=%d expected valid=%t", tc.rooms, tc.members, tc.valid)
		}
	}
	c.Rooms = 16
	c.MembersPerRoom = 4
	c.Rate = 16
	c.MaxRate = 16
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.Rate = 2
	c.MaxRate = 2
	c.Profile = DMProfile
	c.Rooms = 2
	c.MembersPerRoom = 2
	if c.Validate() == nil {
		t.Fatal("accepted room configuration for DM")
	}
	c.Profile = RoomProfile
	c.Rooms = 2
	c.MembersPerRoom = 2
	c.Seed = -1
	if c.Validate() == nil {
		t.Fatal("accepted negative seed")
	}
	c.Seed = 17
	c.MaxRate = math.NaN()
	if c.Validate() == nil {
		t.Fatal("accepted nonfinite max rate")
	}
}
func TestVerifyWarmupFailsOnMissingDuplicateAndDisconnect(t *testing.T) {
	x := Expected{Type: "room_message", Body: "warm", SenderID: 1, TargetID: 10, Recipients: []int64{1, 2}, SentAt: time.Now()}
	ob := func(user int64) Observation {
		r := Observation{UserID: user, At: time.Now()}
		r.Event.Type = "room_message"
		r.Event.Payload.SenderID = 1
		r.Event.Payload.RoomID = 10
		r.Event.Payload.MessageID = 5
		r.Event.Payload.Content = "warm"
		return r
	}
	a, b := ob(1), ob(2)
	for _, tc := range []struct {
		obs         []Observation
		disconnects int
		reason      string
	}{{[]Observation{a}, 0, "missing"}, {[]Observation{a, b, b}, 0, "duplicate"}, {[]Observation{a, b}, 1, "disconnect"}} {
		err := verifyWarmup([]Expected{x}, tc.obs, tc.disconnects)
		if err == nil || !strings.Contains(err.Error(), tc.reason) {
			t.Fatalf("expected %s: %v", tc.reason, err)
		}
	}
	if e := verifyWarmup([]Expected{x}, []Observation{a, b}, 0); e != nil {
		t.Fatal(e)
	}
}
