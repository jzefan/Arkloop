package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"arkloop/services/worker/internal/tools"
)

const (
	errorSandboxError       = "tool.sandbox_error"
	errorSandboxUnavailable = "tool.sandbox_unavailable"
	errorSandboxTimeout     = "tool.sandbox_timeout"
	errorArgsInvalid        = "tool.args_invalid"
	errorNotConfigured      = "tool.not_configured"

	defaultTimeoutMs  = 30_000
	maxOutputBytes    = 32 * 1024
	httpClientTimeout = 5 * time.Minute

	shellActionOpen   = "open"
	shellActionExec   = "exec"
	shellActionRead   = "read"
	shellActionWrite  = "write"
	shellActionSignal = "signal"
	shellActionClose  = "close"

	shellSignalINT = "SIGINT"
)

type execRequest struct {
	SessionID string `json:"session_id"`
	OrgID     string `json:"org_id,omitempty"`
	Tier      string `json:"tier"`
	Language  string `json:"language"`
	Code      string `json:"code"`
	TimeoutMs int    `json:"timeout_ms"`
}

type execResponse struct {
	SessionID  string        `json:"session_id"`
	Stdout     string        `json:"stdout"`
	Stderr     string        `json:"stderr"`
	ExitCode   int           `json:"exit_code"`
	DurationMs int64         `json:"duration_ms"`
	Artifacts  []artifactRef `json:"artifacts,omitempty"`
}

type shellRequest struct {
	SessionID   string `json:"session_id"`
	OrgID       string `json:"org_id,omitempty"`
	Tier        string `json:"tier,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	Command     string `json:"command,omitempty"`
	Input       string `json:"input,omitempty"`
	Signal      string `json:"signal,omitempty"`
	Cursor      uint64 `json:"cursor,omitempty"`
	TimeoutMs   int    `json:"timeout_ms,omitempty"`
	YieldTimeMs int    `json:"yield_time_ms,omitempty"`
}

type shellResponse struct {
	SessionID string        `json:"session_id"`
	Status    string        `json:"status"`
	Cwd       string        `json:"cwd"`
	Output    string        `json:"output"`
	Cursor    uint64        `json:"cursor"`
	Running   bool          `json:"running"`
	Truncated bool          `json:"truncated"`
	TimedOut  bool          `json:"timed_out"`
	ExitCode  *int          `json:"exit_code,omitempty"`
	Artifacts []artifactRef `json:"artifacts,omitempty"`
}

type shellArgs struct {
	Action      string
	Cwd         string
	Command     string
	Input       string
	Signal      string
	Cursor      uint64
	HasCursor   bool
	TimeoutMs   int
	YieldTimeMs int
}

type artifactRef struct {
	Key      string `json:"key"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

type ToolExecutor struct {
	baseURL   string
	authToken string
	client    *http.Client
}

func NewToolExecutor(baseURL, authToken string) *ToolExecutor {
	return &ToolExecutor{
		baseURL:   baseURL,
		authToken: authToken,
		client: &http.Client{
			Timeout: httpClientTimeout,
		},
	}
}

func (e *ToolExecutor) Execute(
	ctx context.Context,
	toolName string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	toolCallID string,
) tools.ExecutionResult {
	started := time.Now()

	if e.baseURL == "" {
		return errResult(errorNotConfigured, "sandbox service not configured", started)
	}

	switch toolName {
	case "python_execute":
		return e.executePython(ctx, args, execCtx, started)
	case "shell_execute":
		return e.executeShell(ctx, args, execCtx, toolCallID, started)
	default:
		return errResult(errorArgsInvalid, fmt.Sprintf("unknown sandbox tool: %s", toolName), started)
	}
}

func (e *ToolExecutor) executePython(
	ctx context.Context,
	args map[string]any,
	execCtx tools.ExecutionContext,
	started time.Time,
) tools.ExecutionResult {
	code, _ := args["code"].(string)
	if code == "" {
		return errResult(errorArgsInvalid, "parameter code is required", started)
	}

	payload, err := json.Marshal(execRequest{
		SessionID: execCtx.RunID.String(),
		OrgID:     resolveOrgID(execCtx),
		Tier:      resolveTier(execCtx.Budget),
		Language:  "python",
		Code:      code,
		TimeoutMs: resolveTimeoutMs(args),
	})
	if err != nil {
		return errResult(errorSandboxError, fmt.Sprintf("marshal request failed: %s", err.Error()), started)
	}

	resp, reqErr := e.doJSONRequest(ctx, http.MethodPost, e.baseURL+"/v1/exec", payload, resolveOrgID(execCtx))
	if reqErr != nil {
		return errResult(reqErr.errorClass, reqErr.message, started)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return errResult(errorSandboxError, "read response body failed", started)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mapHTTPError(resp.StatusCode, respBody, started)
	}

	var result execResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return errResult(errorSandboxError, "decode response failed", started)
	}

	resultJSON := map[string]any{
		"stdout":      truncateOutput(result.Stdout),
		"stderr":      truncateOutput(result.Stderr),
		"exit_code":   result.ExitCode,
		"duration_ms": result.DurationMs,
	}
	if len(result.Artifacts) > 0 {
		resultJSON["artifacts"] = result.Artifacts
	}
	return tools.ExecutionResult{
		ResultJSON: resultJSON,
		DurationMs: durationMs(started),
	}
}

func (e *ToolExecutor) executeShell(
	ctx context.Context,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
	started time.Time,
) tools.ExecutionResult {
	reqArgs, argErr := parseShellArgs(args)
	if argErr != nil {
		return tools.ExecutionResult{Error: argErr, DurationMs: durationMs(started)}
	}

	sessionID := shellSessionID(execCtx.RunID.String())
	orgID := resolveOrgID(execCtx)
	request := shellRequest{
		SessionID:   sessionID,
		OrgID:       orgID,
		Tier:        resolveTier(execCtx.Budget),
		Cwd:         reqArgs.Cwd,
		Command:     reqArgs.Command,
		Input:       reqArgs.Input,
		Signal:      reqArgs.Signal,
		TimeoutMs:   reqArgs.TimeoutMs,
		YieldTimeMs: reqArgs.YieldTimeMs,
	}
	if reqArgs.HasCursor {
		request.Cursor = reqArgs.Cursor
	}

	resp, body, reqErr := e.sendShellRequest(ctx, reqArgs.Action, request, orgID)
	if reqErr != nil {
		return errResult(reqErr.errorClass, reqErr.message, started)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return mapHTTPError(resp.StatusCode, body, started)
	}
	defer resp.Body.Close()

	result := shellResponse{Status: "closed"}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			return errResult(errorSandboxError, "decode response failed", started)
		}
	}

	resultJSON := map[string]any{
		"status":      result.Status,
		"cwd":         result.Cwd,
		"output":      result.Output,
		"cursor":      result.Cursor,
		"running":     result.Running,
		"timed_out":   result.TimedOut,
		"truncated":   result.Truncated,
		"duration_ms": durationMs(started),
	}
	if result.ExitCode != nil {
		resultJSON["exit_code"] = *result.ExitCode
	}
	if len(result.Artifacts) > 0 {
		resultJSON["artifacts"] = result.Artifacts
	}
	return tools.ExecutionResult{ResultJSON: resultJSON, DurationMs: durationMs(started)}
}

type requestError struct {
	errorClass string
	message    string
}

func (e *ToolExecutor) sendShellRequest(
	ctx context.Context,
	action string,
	request shellRequest,
	orgID string,
) (*http.Response, []byte, *requestError) {
	method := http.MethodPost
	endpoint := e.baseURL + shellActionPath(action, request.SessionID)
	var payload []byte
	var err error
	if action != shellActionClose {
		payload, err = json.Marshal(request)
		if err != nil {
			return nil, nil, &requestError{errorClass: errorSandboxError, message: fmt.Sprintf("marshal request failed: %s", err.Error())}
		}
	}

	resp, reqErr := e.doJSONRequest(ctx, methodForShellAction(action, method), endpoint, payload, orgID)
	if reqErr != nil {
		return nil, nil, reqErr
	}
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		resp.Body.Close()
		return nil, nil, &requestError{errorClass: errorSandboxError, message: "read response body failed"}
	}
	return resp, body, nil
}

func (e *ToolExecutor) doJSONRequest(
	ctx context.Context,
	method, endpoint string,
	payload []byte,
	orgID string,
) (*http.Response, *requestError) {
	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, &requestError{errorClass: errorSandboxError, message: fmt.Sprintf("build request failed: %s", err.Error())}
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if e.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+e.authToken)
	}
	if orgID != "" {
		req.Header.Set("X-Org-ID", orgID)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		if isContextDeadline(err) {
			return nil, &requestError{errorClass: errorSandboxTimeout, message: "sandbox request timed out"}
		}
		return nil, &requestError{errorClass: errorSandboxUnavailable, message: fmt.Sprintf("sandbox request failed: %s", err.Error())}
	}
	return resp, nil
}

func parseShellArgs(args map[string]any) (shellArgs, *tools.ExecutionError) {
	action, _ := args["action"].(string)
	command, _ := args["command"].(string)
	if strings.TrimSpace(action) == "" && strings.TrimSpace(command) != "" {
		action = shellActionExec
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return shellArgs{}, shellArgsError("parameter action is required")
	}

	request := shellArgs{
		Action:      action,
		Cwd:         readStringArg(args, "cwd"),
		Command:     command,
		Input:       readStringArg(args, "input"),
		Signal:      readStringArg(args, "signal"),
		TimeoutMs:   resolveTimeoutMs(args),
		YieldTimeMs: readIntArg(args, "yield_time_ms"),
	}
	if cursor, ok := readUint64Arg(args, "cursor"); ok {
		request.Cursor = cursor
		request.HasCursor = true
	}

	switch action {
	case shellActionOpen, shellActionRead, shellActionClose:
		return request, nil
	case shellActionExec:
		if strings.TrimSpace(request.Command) == "" {
			return shellArgs{}, shellArgsError("parameter command is required for action exec")
		}
		return request, nil
	case shellActionWrite:
		if request.Input == "" {
			return shellArgs{}, shellArgsError("parameter input is required for action write")
		}
		return request, nil
	case shellActionSignal:
		if strings.TrimSpace(request.Signal) == "" {
			request.Signal = shellSignalINT
		}
		return request, nil
	default:
		return shellArgs{}, shellArgsError("parameter action is invalid")
	}
}

func shellArgsError(message string) *tools.ExecutionError {
	return &tools.ExecutionError{ErrorClass: errorArgsInvalid, Message: message}
}

func resolveOrgID(execCtx tools.ExecutionContext) string {
	if execCtx.OrgID == nil {
		return ""
	}
	return execCtx.OrgID.String()
}

func shellSessionID(runID string) string {
	return runID + "/shell/default"
}

func shellActionPath(action, sessionID string) string {
	switch action {
	case shellActionOpen:
		return "/v1/shell/open"
	case shellActionExec:
		return "/v1/shell/exec"
	case shellActionRead:
		return "/v1/shell/read"
	case shellActionWrite:
		return "/v1/shell/write"
	case shellActionSignal:
		return "/v1/shell/signal"
	case shellActionClose:
		return "/v1/shell/session/" + sessionID
	default:
		return "/v1/shell/exec"
	}
}

func methodForShellAction(action, fallback string) string {
	if action == shellActionClose {
		return http.MethodDelete
	}
	return fallback
}

func readStringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func readIntArg(args map[string]any, key string) int {
	value, ok := args[key]
	if !ok {
		return 0
	}
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case int64:
		return int(number)
	case json.Number:
		parsed, err := number.Int64()
		if err == nil {
			return int(parsed)
		}
	}
	return 0
}

func readUint64Arg(args map[string]any, key string) (uint64, bool) {
	value, ok := args[key]
	if !ok {
		return 0, false
	}
	switch number := value.(type) {
	case float64:
		if number < 0 {
			return 0, false
		}
		return uint64(number), true
	case int:
		if number < 0 {
			return 0, false
		}
		return uint64(number), true
	case int64:
		if number < 0 {
			return 0, false
		}
		return uint64(number), true
	case uint64:
		return number, true
	case json.Number:
		parsed, err := number.Int64()
		if err != nil || parsed < 0 {
			return 0, false
		}
		return uint64(parsed), true
	default:
		return 0, false
	}
}

func resolveTier(budget map[string]any) string {
	if budget != nil {
		if tier, ok := budget["sandbox_tier"].(string); ok && tier != "" {
			return tier
		}
	}
	return "lite"
}

func resolveTimeoutMs(args map[string]any) int {
	if v, ok := args["timeout_ms"]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		case json.Number:
			if i, err := n.Int64(); err == nil {
				return int(i)
			}
		}
	}
	return defaultTimeoutMs
}

func truncateOutput(s string) string {
	if len(s) <= maxOutputBytes {
		return s
	}
	return s[:maxOutputBytes] + fmt.Sprintf("\n... (truncated, total %d bytes)", len(s))
}

func mapHTTPError(statusCode int, body []byte, started time.Time) tools.ExecutionResult {
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &parsed)

	errorClass := errorSandboxError
	if statusCode == http.StatusGatewayTimeout || parsed.Code == "timeout" {
		errorClass = errorSandboxTimeout
	}
	if statusCode == http.StatusServiceUnavailable || statusCode == http.StatusBadGateway {
		errorClass = errorSandboxUnavailable
	}

	message := parsed.Message
	if message == "" {
		message = fmt.Sprintf("sandbox service returned %d", statusCode)
	}

	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: errorClass,
			Message:    message,
			Details: map[string]any{
				"status_code": statusCode,
				"code":        parsed.Code,
			},
		},
		DurationMs: durationMs(started),
	}
}

func errResult(errorClass, message string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: errorClass,
			Message:    message,
		},
		DurationMs: durationMs(started),
	}
}

func isContextDeadline(err error) bool {
	if err == context.DeadlineExceeded {
		return true
	}
	if unwrap, ok := err.(interface{ Unwrap() error }); ok {
		return isContextDeadline(unwrap.Unwrap())
	}
	return false
}

func durationMs(started time.Time) int {
	elapsed := time.Since(started)
	millis := int(elapsed / time.Millisecond)
	if millis < 0 {
		return 0
	}
	return millis
}
