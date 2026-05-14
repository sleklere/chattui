package api

// AuthRequest represents the login or register request body.
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// CreateRoomRequest represents the request body for creating a room.
type CreateRoomRequest struct {
	Name string `json:"name"`
}

// Error represents a structured error response from the server.
type Error struct {
	Code    string `json:"code,omitempty"`
	Error   string `json:"error"`
	Details any    `json:"details,omitempty"`
}
