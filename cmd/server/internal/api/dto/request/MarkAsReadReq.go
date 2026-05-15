package request

// MarkAsReadReq is the request body for the mark-as-read endpoint.
type MarkAsReadReq struct {
	ConversationID *int64 `json:"conversation_id"`
	RoomID         *int64 `json:"room_id"`
	All            bool   `json:"all"`
}
