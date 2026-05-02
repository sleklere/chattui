package api

import "fmt"

// GetInbox returns the authenticated user's inbox events up to the given limit.
func (c *Client) GetInbox(limit int) ([]InboxEvent, error) {
	var events []InboxEvent
	if err := c.do("GET", fmt.Sprintf("/api/v1/inbox?limit=%d", limit), nil, &events); err != nil {
		return nil, err
	}
	return events, nil
}
