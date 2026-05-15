// Package errs defines common domain-level sentinel errors shared across services.
package errs

import "errors"

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrForbidden is returned when an operation is not permitted for the caller.
	ErrForbidden = errors.New("forbidden")
	// ErrNoParams is returned when a required parameter is missing.
	ErrNoParams = errors.New("at least one parameter is required")
	// ErrAmbiguousParams is returned when mutually exclusive parameters are provided together.
	ErrAmbiguousParams = errors.New("parameters are ambiguous")
)
