package handlers

import (
	"github.com/sleklere/chattui/cmd/server/internal/api/dto/response"
	"github.com/sleklere/chattui/cmd/server/internal/auth"
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
