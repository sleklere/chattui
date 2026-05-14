package api

import "github.com/sleklere/chattui/pkg/dto"

import "fmt"

// ListRooms returns all available rooms.
func (c *Client) ListRooms() ([]dto.Room, error) {
	var rooms []dto.Room
	err := c.do("GET", "/api/v1/rooms", nil, &rooms)
	return rooms, err
}

// CreateRoom creates a new room with the given name.
func (c *Client) CreateRoom(name string) (dto.Room, error) {
	var room dto.Room
	err := c.do("POST", "/api/v1/rooms", CreateRoomRequest{Name: name}, &room)
	return room, err
}

// JoinRoom adds the current user to a room.
func (c *Client) JoinRoom(roomID int64) error {
	return c.do("POST", fmt.Sprintf("/api/v1/rooms/%d/join", roomID), nil, nil)
}

// LeaveRoom removes the current user from a room.
func (c *Client) LeaveRoom(roomID int64) error {
	return c.do("DELETE", fmt.Sprintf("/api/v1/rooms/%d/leave", roomID), nil, nil)
}

// GetMessages retrieves messages for a room with the given limit.
func (c *Client) GetMessages(roomID int64, limit int) ([]dto.Message, error) {
	var messages []dto.Message
	path := fmt.Sprintf("/api/v1/rooms/%d/messages?limit=%d", roomID, limit)
	err := c.do("GET", path, nil, &messages)
	return messages, err
}
