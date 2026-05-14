package handlers

import (
	"github.com/sleklere/chattui/cmd/server/internal/auth"
	"github.com/sleklere/chattui/pkg/dto"
)

func authResultToRes(r auth.Result) dto.Auth {
	return dto.Auth{
		User: dto.User{
			ID:        r.UserID,
			Username:  r.Username,
			CreatedAt: r.CreatedAt,
		},
		Token:     r.Token,
		ExpiresAt: r.ExpiresAt,
	}
}
