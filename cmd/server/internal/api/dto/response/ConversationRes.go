package response

// ConversationRes is the response body for a conversation.
type ConversationRes struct {
	ID            int64  `json:"id"`
	PeerID        int64  `json:"peer_id"`
	PeerUsername  string `json:"peer_username"`
}
