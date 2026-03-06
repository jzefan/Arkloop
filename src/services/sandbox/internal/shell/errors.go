package shell

import "net/http"

const (
	CodeSessionBusy     = "shell.session_busy"
	CodeSessionNotFound = "shell.session_not_found"
	CodeInvalidCursor   = "shell.invalid_cursor"
	CodeNotRunning      = "shell.not_running"
	CodeSignalFailed    = "shell.signal_failed"
	CodeTimeoutTooLarge = "shell.timeout_too_large"
	CodeOrgMismatch     = "sandbox.org_mismatch"
)

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *Error) Error() string {
	return e.Message
}

func newError(code, message string, httpStatus int) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: httpStatus}
}

func busyError() *Error {
	return newError(CodeSessionBusy, "shell session is busy", http.StatusConflict)
}

func notFoundError() *Error {
	return newError(CodeSessionNotFound, "shell session not found", http.StatusNotFound)
}

func invalidCursorError() *Error {
	return newError(CodeInvalidCursor, "cursor is ahead of available output", http.StatusBadRequest)
}

func notRunningError() *Error {
	return newError(CodeNotRunning, "shell session is not running", http.StatusConflict)
}

func signalFailedError(message string) *Error {
	if message == "" {
		message = "failed to signal foreground process"
	}
	return newError(CodeSignalFailed, message, http.StatusInternalServerError)
}

func timeoutTooLargeError() *Error {
	return newError(CodeTimeoutTooLarge, "timeout_ms must not exceed 300000", http.StatusBadRequest)
}

func orgMismatchError() *Error {
	return newError(CodeOrgMismatch, "session belongs to another org", http.StatusForbidden)
}
