package process

import (
	"fmt"
	"net/http"
)

const (
	CodeProcessNotFound       = "process.not_found"
	CodeProcessBusy           = "process.busy"
	CodeCursorExpired         = "process.cursor_expired"
	CodeInvalidCursor         = "process.invalid_cursor"
	CodeNotRunning            = "process.not_running"
	CodeInputSeqRequired      = "process.input_seq_required"
	CodeInputSeqInvalid       = "process.input_seq_invalid"
	CodeStdinNotSupported     = "process.stdin_not_supported"
	CodeCloseStdinUnsupported = "process.close_stdin_unsupported"
	CodeResizeNotSupported    = "process.resize_not_supported"
	CodeInvalidMode           = "process.invalid_mode"
	CodeInvalidSize           = "process.invalid_size"
	CodeTimeoutRequired       = "process.timeout_required"
	CodeTimeoutTooLarge       = "process.timeout_too_large"
	CodeAccountMismatch       = "sandbox.account_mismatch"
	CodeMaxSessionsExceeded   = "process.max_sessions_exceeded"
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

func processNotFoundError() *Error {
	return newError(CodeProcessNotFound, "process not found", http.StatusNotFound)
}

func busyError() *Error {
	return newError(CodeProcessBusy, "process is busy", http.StatusConflict)
}

func NewTerminalStateError() *Error {
	return newError(CodeProcessBusy, "process did not reach terminal state", http.StatusConflict)
}

func cursorExpiredError() *Error {
	return newError(CodeCursorExpired, "cursor has expired", http.StatusConflict)
}

func invalidCursorError() *Error {
	return newError(CodeInvalidCursor, "cursor is invalid", http.StatusBadRequest)
}

func notRunningError() *Error {
	return newError(CodeNotRunning, "process is not running", http.StatusConflict)
}

func inputSeqRequiredError() *Error {
	return newError(CodeInputSeqRequired, "input_seq is required when stdin_text is provided", http.StatusBadRequest)
}

func inputSeqInvalidError() *Error {
	return newError(CodeInputSeqInvalid, "input_seq must be positive", http.StatusBadRequest)
}

func stdinNotSupportedError() *Error {
	return newError(CodeStdinNotSupported, "process does not accept stdin", http.StatusConflict)
}

func closeStdinUnsupportedError(mode string) *Error {
	return newError(CodeCloseStdinUnsupported, fmt.Sprintf("close_stdin is not supported for mode: %s", mode), http.StatusConflict)
}

func resizeNotSupportedError() *Error {
	return newError(CodeResizeNotSupported, "process is not a PTY session", http.StatusConflict)
}

func invalidModeError(mode string) *Error {
	return newError(CodeInvalidMode, fmt.Sprintf("unsupported mode: %s", mode), http.StatusBadRequest)
}

func invalidSizeError() *Error {
	return newError(CodeInvalidSize, "size is only supported for pty mode and rows/cols must be positive", http.StatusBadRequest)
}

func timeoutRequiredError() *Error {
	return newError(CodeTimeoutRequired, "timeout_ms is required for follow, stdin, and pty modes", http.StatusBadRequest)
}

func timeoutTooLargeError() *Error {
	return newError(CodeTimeoutTooLarge, "timeout_ms must not exceed 1800000", http.StatusBadRequest)
}

func accountMismatchError() *Error {
	return newError(CodeAccountMismatch, "session belongs to another account", http.StatusForbidden)
}

func maxSessionsExceededError(maxSessions int) *Error {
	return newError(CodeMaxSessionsExceeded, fmt.Sprintf("max process sessions reached: %d", maxSessions), http.StatusServiceUnavailable)
}
