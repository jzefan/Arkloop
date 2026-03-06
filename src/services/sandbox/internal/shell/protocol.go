package shell

const (
	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusClosed  = "closed"

	SignalINT  = "SIGINT"
	SignalTERM = "SIGTERM"
	SignalKILL = "SIGKILL"

	defaultYieldTimeMs = 1000
	maxYieldTimeMs     = 30000
	maxTimeoutMs       = 300000
)

type Request struct {
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

type ArtifactRef struct {
	Key      string `json:"key"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

type Response struct {
	SessionID string        `json:"session_id"`
	Status    string        `json:"status"`
	Cwd       string        `json:"cwd"`
	Output    string        `json:"output"`
	Cursor    uint64        `json:"cursor"`
	Running   bool          `json:"running"`
	Truncated bool          `json:"truncated"`
	TimedOut  bool          `json:"timed_out"`
	ExitCode  *int          `json:"exit_code,omitempty"`
	Artifacts []ArtifactRef `json:"artifacts,omitempty"`
}

type AgentRequest struct {
	Action     string                  `json:"action"`
	Shell      *AgentShellRequest      `json:"shell,omitempty"`
	Checkpoint *AgentCheckpointRequest `json:"checkpoint,omitempty"`
}

type AgentShellRequest struct {
	Cwd         string            `json:"cwd,omitempty"`
	Command     string            `json:"command,omitempty"`
	Input       string            `json:"input,omitempty"`
	Signal      string            `json:"signal,omitempty"`
	Cursor      uint64            `json:"cursor,omitempty"`
	TimeoutMs   int               `json:"timeout_ms,omitempty"`
	YieldTimeMs int               `json:"yield_time_ms,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

type AgentCheckpointRequest struct {
	Archive string `json:"archive,omitempty"`
}

type AgentCheckpointResponse struct {
	Cwd     string            `json:"cwd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Archive string            `json:"archive,omitempty"`
}

type AgentResponse struct {
	Action     string                   `json:"action"`
	Shell      *AgentShellResponse      `json:"shell,omitempty"`
	Checkpoint *AgentCheckpointResponse `json:"checkpoint,omitempty"`
	Code       string                   `json:"code,omitempty"`
	Error      string                   `json:"error,omitempty"`
}

type AgentShellResponse struct {
	Status    string `json:"status"`
	Cwd       string `json:"cwd"`
	Output    string `json:"output"`
	Cursor    uint64 `json:"cursor"`
	Running   bool   `json:"running"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timed_out"`
	ExitCode  *int   `json:"exit_code,omitempty"`
}

func NormalizeYieldTimeMs(value int) int {
	if value <= 0 {
		return defaultYieldTimeMs
	}
	if value > maxYieldTimeMs {
		return maxYieldTimeMs
	}
	return value
}

func NormalizeTimeoutMs(value int) int {
	if value <= 0 {
		return 30_000
	}
	return value
}

func ValidateTimeoutMs(value int) *Error {
	if value > maxTimeoutMs {
		return timeoutTooLargeError()
	}
	return nil
}
