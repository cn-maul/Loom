package models

import (
	"errors"
	"fmt"
)

// Domain errors are declared here so the repository, service and handler layers
// can agree on a meaning without importing each other.
var (
	// ErrNotFound is returned when the addressed row does not exist. A write
	// against a missing row must fail instead of silently reporting success.
	ErrNotFound = errors.New("not found")
	// ErrInvalidInput covers malformed payloads and out-of-range query values.
	ErrInvalidInput = errors.New("invalid input")
	// ErrConflict covers requests that are well-formed but not allowed from the
	// row's current state, such as reopening a finished follow-up.
	ErrConflict = errors.New("conflict")
	// ErrPersonNotFound separates a missing parent person from a missing
	// follow-up, so the API can tell the caller which id was wrong.
	ErrPersonNotFound = errors.New("person not found")
	// ErrOrgNotFound is the organisation counterpart of ErrPersonNotFound.
	ErrOrgNotFound = errors.New("organization not found")
)

// Error pairs a domain kind with a message that reads on its own. Plain
// fmt.Errorf("...: %w", ErrX) is enough for errors.Is, but it appends the
// sentinel text to every message, so API responses ended in noise like
// "person_id and title are required: invalid input".
type Error struct {
	kind    error
	message string
}

func (e *Error) Error() string { return e.message }

// Unwrap exposes the kind so callers keep using errors.Is to classify.
func (e *Error) Unwrap() error { return e.kind }

// NewError builds a categorised error whose message stands alone.
func NewError(kind error, format string, args ...any) error {
	return &Error{kind: kind, message: fmt.Sprintf(format, args...)}
}
