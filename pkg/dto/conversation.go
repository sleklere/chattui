package dto

// Conversation represents a DM conversation in API responses.
type Conversation struct {
	ID           int64  `json:"id"`
	PeerID       int64  `json:"peer_id"`
	PeerUsername string `json:"peer_username"`
}
