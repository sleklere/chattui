package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/sleklere/chattui/cmd/server/internal/auth"
	"github.com/sleklere/chattui/cmd/server/internal/errs"
	"github.com/sleklere/chattui/cmd/server/internal/httpx"
)

// AppHandler is a handler that returns an error
type AppHandler func(http.ResponseWriter, *http.Request) error

// Handle adapts an AppHandler and centralizes error handling
func (a *API) handle(h AppHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err != nil {
			a.writeError(w, r, err)
		}
	}
}

func (a *API) validateJWT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			err := httpx.New(http.StatusUnauthorized, "missing_token", "missing bearer token", errors.New("missing bearer token"))
			a.writeError(w, r, err)
			return
		}
		tokenStr := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))

		claims, err := auth.ParseToken(tokenStr, a.AuthConfig)
		if err != nil {
			herr := httpx.New(http.StatusUnauthorized, "invalid_token", "invalid bearer token", err)
			a.writeError(w, r, herr)
			return
		}

		ctx := auth.NewClaimsContext(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var he *httpx.HTTPError
	switch {
	case errors.As(err, &he):
		// already an HTTPError
	case errors.Is(err, errs.ErrNotFound):
		he = &httpx.HTTPError{Status: http.StatusNotFound, Code: "not_found", Msg: "not found", Err: err}
	case errors.Is(err, errs.ErrForbidden):
		he = &httpx.HTTPError{Status: http.StatusForbidden, Code: "forbidden", Msg: "forbidden", Err: err}
	case errors.Is(err, errs.ErrNoParams):
		he = &httpx.HTTPError{Status: http.StatusBadRequest, Code: "no_params", Msg: err.Error(), Err: err}
	case errors.Is(err, errs.ErrAmbiguousParams):
		he = &httpx.HTTPError{Status: http.StatusBadRequest, Code: "ambiguous_params", Msg: err.Error(), Err: err}
	default:
		he = &httpx.HTTPError{Status: http.StatusInternalServerError, Code: "internal", Msg: "internal error", Err: err}
	}

	reqID := middleware.GetReqID(r.Context())
	a.Logger.Error("http error", "status", he.Status, "code", he.Code, "msg", he.Msg, "req_id", reqID, "err", he.Err)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(he.Status)

	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      he.Msg,
		"code":       he.Code,
		"request_id": reqID,
	})
}
