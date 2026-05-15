package api

import (
	"fmt"

	"github.com/sleklere/chattui/pkg/dto"
)

// GetInbox returns the authenticated user's inbox feed up to the given limit.
func (c *Client) GetInbox(limit int) ([]dto.InboxFeed, error) {
	var entries []dto.InboxFeed
	if err := c.do("GET", fmt.Sprintf("/api/v1/inbox?limit=%d", limit), nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

type markAsReadReq struct {
	ConversationID *int64 `json:"conversation_id,omitempty"`
	RoomID         *int64 `json:"room_id,omitempty"`
	All            bool   `json:"all,omitempty"`
}

// MarkAsRead resets the unread count for a specific room, DM conversation, or all entries.
func (c *Client) MarkAsRead(conversationID *int64, roomID *int64, all bool) error {
	return c.do("POST", "/api/v1/inbox/read", markAsReadReq{
		ConversationID: conversationID,
		RoomID:         roomID,
		All:            all,
	}, nil)
}
