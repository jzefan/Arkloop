package process

import (
	"fmt"
	"strconv"
	"strings"

	"arkloop/services/shared/skillstore"
)

const (
	ModeBuffered = "buffered"
	ModeFollow   = "follow"
	ModeStdin    = "stdin"
	ModePTY      = "pty"

	StatusRunning    = "running"
	StatusExited     = "exited"
	StatusTerminated = "terminated"
	StatusTimedOut   = "timed_out"
	StatusCancelled  = "cancelled"

	StreamStdout = "stdout"
	StreamStderr = "stderr"
	StreamPTY    = "pty"
	StreamSystem = "system"

	defaultWaitMs        = 500
	defaultBufferedLimit = 30_000
	defaultFollowLimit   = 30_000
	maxWaitMs            = 30_000
	maxTimeoutMs         = 1_800_000
)

type Size struct {
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}

type ExecCommandRequest struct {
	SessionID     string                     `json:"session_id"`
	AccountID     string                     `json:"account_id,omitempty"`
	ProfileRef    string                     `json:"profile_ref,omitempty"`
	WorkspaceRef  string                     `json:"workspace_ref,omitempty"`
	EnabledSkills []skillstore.ResolvedSkill `json:"enabled_skills,omitempty"`
	Tier          string                     `json:"tier,omitempty"`
	Command       string                     `json:"command"`
	Mode          string                     `json:"mode,omitempty"`
	Cwd           string                     `json:"cwd,omitempty"`
	TimeoutMs     int                        `json:"timeout_ms,omitempty"`
	Size          *Size                      `json:"size,omitempty"`
	Env           map[string]*string         `json:"env,omitempty"`
}

type ContinueProcessRequest struct {
	SessionID  string  `json:"session_id"`
	AccountID  string  `json:"account_id,omitempty"`
	ProcessRef string  `json:"process_ref"`
	Cursor     string  `json:"cursor"`
	WaitMs     int     `json:"wait_ms,omitempty"`
	StdinText  *string `json:"stdin_text,omitempty"`
	InputSeq   *int64  `json:"input_seq,omitempty"`
	CloseStdin bool    `json:"close_stdin,omitempty"`
}

type TerminateProcessRequest struct {
	SessionID  string `json:"session_id"`
	AccountID  string `json:"account_id,omitempty"`
	ProcessRef string `json:"process_ref"`
}

type ResizeProcessRequest struct {
	SessionID  string `json:"session_id"`
	AccountID  string `json:"account_id,omitempty"`
	ProcessRef string `json:"process_ref"`
	Rows       int    `json:"rows"`
	Cols       int    `json:"cols"`
}

type OutputItem struct {
	Seq    uint64 `json:"seq"`
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

type ArtifactRef struct {
	Key      string `json:"key"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

type Response struct {
	Status           string        `json:"status"`
	ProcessRef       string        `json:"process_ref,omitempty"`
	Stdout           string        `json:"stdout,omitempty"`
	Stderr           string        `json:"stderr,omitempty"`
	ExitCode         *int          `json:"exit_code,omitempty"`
	Cursor           string        `json:"cursor,omitempty"`
	NextCursor       string        `json:"next_cursor,omitempty"`
	Items            []OutputItem  `json:"items,omitempty"`
	HasMore          bool          `json:"has_more,omitempty"`
	AcceptedInputSeq *int64        `json:"accepted_input_seq,omitempty"`
	Truncated        bool          `json:"truncated,omitempty"`
	OutputRef        string        `json:"output_ref,omitempty"`
	Artifacts        []ArtifactRef `json:"artifacts,omitempty"`
}

type AgentExecRequest struct {
	Command   string             `json:"command"`
	Mode      string             `json:"mode,omitempty"`
	Cwd       string             `json:"cwd,omitempty"`
	TimeoutMs int                `json:"timeout_ms,omitempty"`
	Size      *Size              `json:"size,omitempty"`
	Env       map[string]*string `json:"env,omitempty"`
}

type AgentRefRequest struct {
	ProcessRef string `json:"process_ref"`
}

type AgentResizeRequest struct {
	ProcessRef string `json:"process_ref"`
	Rows       int    `json:"rows"`
	Cols       int    `json:"cols"`
}

type AgentRequest struct {
	Action           string                  `json:"action"`
	ExecCommand      *AgentExecRequest       `json:"process_exec,omitempty"`
	ContinueProcess  *ContinueProcessRequest `json:"process_continue,omitempty"`
	TerminateProcess *AgentRefRequest        `json:"process_terminate,omitempty"`
	ResizeProcess    *AgentResizeRequest     `json:"process_resize,omitempty"`
}

type AgentResponse struct {
	Action  string    `json:"action"`
	Process *Response `json:"process,omitempty"`
	Code    string    `json:"code,omitempty"`
	Error   string    `json:"error,omitempty"`
}

func CursorString(seq uint64) string {
	return strconv.FormatUint(seq, 10)
}

func ParseCursor(value string) (uint64, error) {
	if value == "" {
		return 0, fmt.Errorf("cursor is required")
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cursor must be an unsigned integer")
	}
	return parsed, nil
}

func NormalizeMode(value string) string {
	switch strings.TrimSpace(value) {
	case "":
		return ModeBuffered
	case ModeFollow, ModeStdin, ModePTY:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(value)
	}
}

func NormalizeWaitMs(value int) int {
	if value <= 0 {
		return defaultWaitMs
	}
	if value > maxWaitMs {
		return maxWaitMs
	}
	return value
}

func NormalizeTimeoutMs(mode string, value int) int {
	switch NormalizeMode(mode) {
	case ModeBuffered:
		if value <= 0 {
			return defaultBufferedLimit
		}
	default:
		if value <= 0 {
			return defaultFollowLimit
		}
	}
	if value > maxTimeoutMs {
		return maxTimeoutMs
	}
	return value
}

func ValidateMode(value string) *Error {
	switch strings.TrimSpace(value) {
	case "", ModeBuffered, ModeFollow, ModeStdin, ModePTY:
		return nil
	default:
		return invalidModeError(value)
	}
}

func ValidateExecRequest(req ExecCommandRequest) *Error {
	mode := NormalizeMode(req.Mode)
	if err := ValidateMode(mode); err != nil {
		return err
	}
	if strings.TrimSpace(req.Command) == "" {
		return newError(CodeInvalidMode, "command is required", 400)
	}
	if mode != ModeBuffered && req.TimeoutMs <= 0 {
		return timeoutRequiredError()
	}
	if req.TimeoutMs > maxTimeoutMs {
		return timeoutTooLargeError()
	}
	if req.Size != nil {
		if mode != ModePTY || req.Size.Rows <= 0 || req.Size.Cols <= 0 {
			return invalidSizeError()
		}
	}
	return nil
}

func ValidateContinueRequest(req ContinueProcessRequest) *Error {
	if strings.TrimSpace(req.ProcessRef) == "" {
		return processNotFoundError()
	}
	if strings.TrimSpace(req.Cursor) == "" {
		return invalidCursorError()
	}
	if req.StdinText != nil {
		if req.InputSeq == nil {
			return inputSeqRequiredError()
		}
		if req.InputSeq != nil && *req.InputSeq <= 0 {
			return inputSeqInvalidError()
		}
	} else if req.InputSeq != nil {
		return inputSeqInvalidError()
	}
	return nil
}

func ValidateResizeRequest(req ResizeProcessRequest) *Error {
	if strings.TrimSpace(req.ProcessRef) == "" {
		return processNotFoundError()
	}
	if req.Rows <= 0 || req.Cols <= 0 {
		return invalidSizeError()
	}
	return nil
}
