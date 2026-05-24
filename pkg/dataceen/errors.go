package dataceen

import "fmt"

// Error is raised for any failure inside the Dataceen client pipeline:
// HTTP non-success responses, GraphQL errors, server-side ResponseCode != 0,
// JSON parse failures, and transport errors that exhausted retries.
//
// Mirrors the C# DataceenException (HResult/SourceQuery) and the TS
// DataceenError class.
type Error struct {
	// ResponseCode is the server-side ResponseCode if available, the negated
	// HTTP status as a fallback (e.g. -503 for HTTP 503), or -1 for
	// client-side failures (JSON parse, envelope shape, transport).
	ResponseCode int
	// SourceQuery is the GraphQL query that produced this error. Optional.
	SourceQuery string
	// Message is the human-readable reason.
	Message string
	// Cause wraps the underlying error, if any. Use errors.Is/As to inspect.
	Cause error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("dataceen: %s (code=%d): %v", e.Message, e.ResponseCode, e.Cause)
	}
	return fmt.Sprintf("dataceen: %s (code=%d)", e.Message, e.ResponseCode)
}

func (e *Error) Unwrap() error { return e.Cause }

// newError is a small constructor that keeps callers terse.
func newError(message string, responseCode int, sourceQuery string, cause error) *Error {
	return &Error{
		ResponseCode: responseCode,
		SourceQuery:  sourceQuery,
		Message:      message,
		Cause:        cause,
	}
}
