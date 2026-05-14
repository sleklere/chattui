package api

import "github.com/sleklere/chattui/pkg/dto"

import (
	"fmt"
	"net/url"
)

// GetUserByUsername looks up a user by their username.
func (c *Client) GetUserByUsername(username string) (dto.User, error) {
	var user dto.User
	path := fmt.Sprintf("/api/v1/users?username=%s", url.QueryEscape(username))
	if err := c.do("GET", path, nil, &user); err != nil {
		return dto.User{}, err
	}
	return user, nil
}
