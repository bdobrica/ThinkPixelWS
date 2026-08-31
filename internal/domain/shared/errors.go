package shared

import (
	"errors"
	"fmt"
)

// ErrorCode is a stable, machine-readable classification. It is deliberately
// transport-neutral; HTTP adapters map codes to RFC 7807 responses.
type ErrorCode string

const (
	CodeInvalidArgument ErrorCode = "invalid_argument"
	CodeNotFound        ErrorCode = "not_found"
	CodeConflict        ErrorCode = "conflict"
	CodeUnauthorized    ErrorCode = "unauthorized"
	CodeForbidden       ErrorCode = "forbidden"
	CodeExpired         ErrorCode = "expired"
	CodeUnavailable     ErrorCode = "unavailable"
	CodeTooLarge        ErrorCode = "too_large"
	CodeInternal        ErrorCode = "internal"
)

// Error carries a stable code and safe message while retaining an internal
// cause for errors.Is/errors.As. Callers must not put secrets in Message.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

func WrapError(code ErrorCode, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// ErrorCodeOf returns the stable code of err, including wrapped Errors.
func ErrorCodeOf(err error) ErrorCode {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return CodeInternal
}
