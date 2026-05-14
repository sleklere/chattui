// Package dto defines the shared data transfer objects used by the HTTP API.
// Both the server (response serialization) and the client (response deserialization)
// import from this package to guarantee wire format consistency.
package dto

import "time"

// User represents a user in API responses.
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}
