package handlers

import (
	"net/http"
	"strconv"
)

const (
	defaultMessageLimit = 50
	maxMessageLimit     = 100
)

func parseLimit(r *http.Request) int32 {
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.ParseInt(l, 10, 32)
		if err == nil && n > 0 && n <= maxMessageLimit {
			return int32(n)
		}
	}
	return defaultMessageLimit
}
