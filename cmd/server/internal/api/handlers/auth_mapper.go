package handlers

import (
	"github.com/sleklere/realtime-chat/cmd/server/internal/api/dto/response"
	"github.com/sleklere/realtime-chat/cmd/server/internal/auth"
)

func authResultToRes(r auth.Result) response.AuthRes {
	return response.AuthRes{
		User: response.UserRes{
			ID:        r.UserID,
			Username:  r.Username,
			CreatedAt: r.CreatedAt,
		},
		Token:     r.Token,
		ExpiresAt: r.ExpiresAt,
	}
}
