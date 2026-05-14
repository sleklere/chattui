package dto

// Auth is the response for a successful login or registration.
type Auth struct {
	User      User   `json:"user"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}
