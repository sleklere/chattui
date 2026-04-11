// Package errs defines common domain-level sentinel errors shared across services.
package errs

import "errors"

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrForbidden is returned when an operation is not permitted for the caller.
	ErrForbidden = errors.New("forbidden")
)
