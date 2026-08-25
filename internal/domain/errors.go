package domain

import "fmt"

type ErrorCode string

const (
	CodeInvalid      ErrorCode = "invalid_argument"
	CodeNotFound     ErrorCode = "not_found"
	CodeConflict     ErrorCode = "version_conflict"
	CodeInvalidState ErrorCode = "invalid_state"
	CodeForbidden    ErrorCode = "forbidden"
	CodeFrozen       ErrorCode = "batch_frozen"
	CodeGateBlocked  ErrorCode = "release_gate_blocked"
)

type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details any       `json:"details,omitempty"`
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

func NewError(code ErrorCode, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func NewDetailedError(code ErrorCode, details any, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Details: details}
}

func ErrorCodeOf(err error) ErrorCode {
	if typed, ok := err.(*Error); ok {
		return typed.Code
	}
	return "internal_error"
}
