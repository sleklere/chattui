package api

import "github.com/sleklere/chattui/pkg/dto"

import "fmt"

// GetInbox returns the authenticated user's inbox feed up to the given limit.
func (c *Client) GetInbox(limit int) ([]dto.InboxFeed, error) {
	var entries []dto.InboxFeed
	if err := c.do("GET", fmt.Sprintf("/api/v1/inbox?limit=%d", limit), nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}
