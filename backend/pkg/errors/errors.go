package errors

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// Error represents an application error with context
type Error struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Err       error                  `json:"-"`
	Stack     []string               `json:"stack,omitempty"`
	Timestamp string                 `json:"timestamp"`
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Err
}

// WithDetail adds a detail field to the error
func (e *Error) WithDetail(key string, value interface{}) *Error {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// WithDetails adds multiple detail fields to the error
func (e *Error) WithDetails(details map[string]interface{}) *Error {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	for k, v := range details {
		e.Details[k] = v
	}
	return e
}

// New creates a new error with a code and message
func New(code, message string) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Details:   make(map[string]interface{}),
		Stack:     captureStack(),
		Timestamp: getTimestamp(),
	}
}

// Wrap wraps an existing error with additional context
func Wrap(err error, code, message string) *Error {
	if err == nil {
		return nil
	}

	var appErr *Error
	if As(err, &appErr) {
		// If it's already an Error, preserve the code if not provided
		if code == "" {
			code = appErr.Code
		}
		return &Error{
			Code:      code,
			Message:   message,
			Err:       err,
			Details:   appErr.Details,
			Stack:     appErr.Stack,
			Timestamp: appErr.Timestamp,
		}
	}

	return &Error{
		Code:      code,
		Message:   message,
		Err:       err,
		Details:   make(map[string]interface{}),
		Stack:     captureStack(),
		Timestamp: getTimestamp(),
	}
}

// Wrapf wraps an existing error with formatted message
func Wrapf(err error, code, format string, args ...interface{}) *Error {
	return Wrap(err, code, fmt.Sprintf(format, args...))
}

// Is checks if the error matches a target error
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As checks if the error can be assigned to target
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

// Unwrap returns the underlying error
func Unwrap(err error) error {
	return errors.Unwrap(err)
}

// Common error codes
const (
	ErrCodeInternal     = "INTERNAL_ERROR"
	ErrCodeNotFound     = "NOT_FOUND"
	ErrCodeUnauthorized = "UNAUTHORIZED"
	ErrCodeForbidden    = "FORBIDDEN"
	ErrCodeBadRequest   = "BAD_REQUEST"
	ErrCodeConflict     = "CONFLICT"
	ErrCodeValidation   = "VALIDATION_ERROR"
	ErrCodeTimeout      = "TIMEOUT"
	ErrCodeUnavailable  = "SERVICE_UNAVAILABLE"
)

// Common error constructors

// InternalError creates an internal server error
func InternalError(message string) *Error {
	return New(ErrCodeInternal, message)
}

// InternalErrorf creates an internal server error with formatting
func InternalErrorf(format string, args ...interface{}) *Error {
	return New(ErrCodeInternal, fmt.Sprintf(format, args...))
}

// NotFound creates a not found error
func NotFound(resource string) *Error {
	return New(ErrCodeNotFound, fmt.Sprintf("%s not found", resource))
}

// Unauthorized creates an unauthorized error
func Unauthorized(message string) *Error {
	if message == "" {
		message = "unauthorized"
	}
	return New(ErrCodeUnauthorized, message)
}

// Forbidden creates a forbidden error
func Forbidden(message string) *Error {
	if message == "" {
		message = "forbidden"
	}
	return New(ErrCodeForbidden, message)
}

// BadRequest creates a bad request error
func BadRequest(message string) *Error {
	return New(ErrCodeBadRequest, message)
}

// BadRequestf creates a bad request error with formatting
func BadRequestf(format string, args ...interface{}) *Error {
	return New(ErrCodeBadRequest, fmt.Sprintf(format, args...))
}

// Conflict creates a conflict error
func Conflict(message string) *Error {
	return New(ErrCodeConflict, message)
}

// ValidationError creates a validation error
func ValidationError(message string) *Error {
	return New(ErrCodeValidation, message)
}

// ValidationErrorf creates a validation error with formatting
func ValidationErrorf(format string, args ...interface{}) *Error {
	return New(ErrCodeValidation, fmt.Sprintf(format, args...))
}

// Timeout creates a timeout error
func Timeout(message string) *Error {
	return New(ErrCodeTimeout, message)
}

// Unavailable creates a service unavailable error
func Unavailable(message string) *Error {
	return New(ErrCodeUnavailable, message)
}

// Helper functions

// captureStack captures the current stack trace
func captureStack() []string {
	pc := make([]uintptr, 10)
	n := runtime.Callers(3, pc)
	frames := runtime.CallersFrames(pc[:n])

	var stack []string
	for {
		frame, more := frames.Next()
		stack = append(stack, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		if !more {
			break
		}
	}
	return stack
}

// getTimestamp returns current timestamp in RFC3339 format
func getTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// Format formats an error for logging
func Format(err error) string {
	if err == nil {
		return ""
	}

	var appErr *Error
	if As(err, &appErr) {
		var parts []string
		parts = append(parts, fmt.Sprintf("code=%s", appErr.Code))
		parts = append(parts, fmt.Sprintf("message=%q", appErr.Message))

		if appErr.Err != nil {
			parts = append(parts, fmt.Sprintf("error=%v", appErr.Err))
		}

		if len(appErr.Details) > 0 {
			var detailParts []string
			for k, v := range appErr.Details {
				detailParts = append(detailParts, fmt.Sprintf("%s=%v", k, v))
			}
			parts = append(parts, fmt.Sprintf("details=[%s]", strings.Join(detailParts, " ")))
		}

		return strings.Join(parts, " ")
	}

	return err.Error()
}
