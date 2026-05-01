package response

import "time"

// InboxRes is the response body for a single inbox event.
type InboxRes struct {
	ID             int64      `json:"id"`
	Kind           string     `json:"kind"`
	RoomID         int64      `json:"room_id"`
	CreatedAt      time.Time  `json:"created_at"`
	SourceUserID   int64      `json:"source_user_id"`
	SourceUsername string     `json:"source_username"`
	ReadAt         *time.Time `json:"read_at"`
}
