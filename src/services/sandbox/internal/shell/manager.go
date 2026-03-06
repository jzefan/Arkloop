package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"arkloop/services/sandbox/internal/logging"
	"arkloop/services/sandbox/internal/session"
)

type Service interface {
	Open(ctx context.Context, req Request) (*Response, error)
	Exec(ctx context.Context, req Request) (*Response, error)
	Read(ctx context.Context, req Request) (*Response, error)
	Write(ctx context.Context, req Request) (*Response, error)
	Signal(ctx context.Context, req Request) (*Response, error)
	Close(ctx context.Context, sessionID, orgID string) error
}

type Manager struct {
	compute       *session.Manager
	artifactStore artifactStore
	logger        *logging.JSONLogger

	mu       sync.Mutex
	sessions map[string]*managedSession
}

type managedSession struct {
	mu sync.Mutex

	compute      *session.Session
	orgID        string
	commandSeq   int64
	uploadedSeq  int64
	artifactSeen map[string]artifactVersion
}

type transportError struct {
	err error
}

func (e *transportError) Error() string {
	return e.err.Error()
}

func (e *transportError) Unwrap() error {
	return e.err
}

func NewManager(compute *session.Manager, store artifactStore, logger *logging.JSONLogger) *Manager {
	return &Manager{
		compute:       compute,
		artifactStore: store,
		logger:        logger,
		sessions:      make(map[string]*managedSession),
	}
}

func (m *Manager) Open(ctx context.Context, req Request) (*Response, error) {
	if err := ValidateTimeoutMs(req.TimeoutMs); err != nil {
		return nil, err
	}
	entry, created := m.getOrCreateEntry(req.SessionID, req.OrgID)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if err := ensureOrg(entry.orgID, req.OrgID); err != nil {
		return nil, err
	}

	if entry.compute != nil {
		result, err := m.invoke(ctx, entry, "shell_open", req)
		if err == nil {
			return m.toResponse(req.SessionID, entry, result), nil
		}
		if _, ok := err.(*transportError); !ok {
			return nil, err
		}
		entry.compute = nil
		entry.commandSeq = 0
		entry.uploadedSeq = 0
		entry.artifactSeen = nil
	}

	computeSession, err := m.compute.GetOrCreate(ctx, req.SessionID, req.Tier, req.OrgID)
	if err != nil {
		m.dropEntry(req.SessionID, entry)
		if strings.Contains(err.Error(), "org mismatch") {
			return nil, orgMismatchError()
		}
		return nil, fmt.Errorf("get shell compute session: %w", err)
	}

	entry.compute = computeSession
	entry.orgID = computeSession.OrgID
	if entry.artifactSeen == nil {
		entry.artifactSeen = make(map[string]artifactVersion)
	}

	result, err := m.invoke(ctx, entry, "shell_open", req)
	if err != nil {
		m.dropEntry(req.SessionID, entry)
		_ = m.compute.Delete(ctx, req.SessionID, req.OrgID)
		if _, ok := err.(*transportError); ok && !created {
			return nil, notFoundError()
		}
		return nil, err
	}
	return m.toResponse(req.SessionID, entry, result), nil
}

func (m *Manager) Exec(ctx context.Context, req Request) (*Response, error) {
	if err := ValidateTimeoutMs(req.TimeoutMs); err != nil {
		return nil, err
	}
	entry, err := m.getExistingEntry(req.SessionID, req.OrgID)
	if err != nil {
		return nil, err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := ensureOrg(entry.orgID, req.OrgID); err != nil {
		return nil, err
	}

	entry.commandSeq++
	result, err := m.invoke(ctx, entry, "shell_exec", req)
	if err != nil {
		if _, ok := err.(*transportError); ok {
			m.dropEntry(req.SessionID, entry)
			return nil, notFoundError()
		}
		entry.commandSeq--
		return nil, err
	}

	resp := m.toResponse(req.SessionID, entry, result)
	m.attachArtifacts(ctx, req.SessionID, entry, result, resp)
	return resp, nil
}

func (m *Manager) Read(ctx context.Context, req Request) (*Response, error) {
	entry, err := m.getExistingEntry(req.SessionID, req.OrgID)
	if err != nil {
		return nil, err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := ensureOrg(entry.orgID, req.OrgID); err != nil {
		return nil, err
	}

	result, err := m.invoke(ctx, entry, "shell_read", req)
	if err != nil {
		if _, ok := err.(*transportError); ok {
			m.dropEntry(req.SessionID, entry)
			return nil, notFoundError()
		}
		return nil, err
	}

	resp := m.toResponse(req.SessionID, entry, result)
	m.attachArtifacts(ctx, req.SessionID, entry, result, resp)
	return resp, nil
}

func (m *Manager) Write(ctx context.Context, req Request) (*Response, error) {
	entry, err := m.getExistingEntry(req.SessionID, req.OrgID)
	if err != nil {
		return nil, err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := ensureOrg(entry.orgID, req.OrgID); err != nil {
		return nil, err
	}

	result, err := m.invoke(ctx, entry, "shell_write", req)
	if err != nil {
		if _, ok := err.(*transportError); ok {
			m.dropEntry(req.SessionID, entry)
			return nil, notFoundError()
		}
		return nil, err
	}

	resp := m.toResponse(req.SessionID, entry, result)
	m.attachArtifacts(ctx, req.SessionID, entry, result, resp)
	return resp, nil
}

func (m *Manager) Signal(ctx context.Context, req Request) (*Response, error) {
	entry, err := m.getExistingEntry(req.SessionID, req.OrgID)
	if err != nil {
		return nil, err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := ensureOrg(entry.orgID, req.OrgID); err != nil {
		return nil, err
	}

	result, err := m.invoke(ctx, entry, "shell_signal", req)
	if err != nil {
		if _, ok := err.(*transportError); ok {
			m.dropEntry(req.SessionID, entry)
			return nil, notFoundError()
		}
		return nil, err
	}

	resp := m.toResponse(req.SessionID, entry, result)
	m.attachArtifacts(ctx, req.SessionID, entry, result, resp)
	return resp, nil
}

func (m *Manager) Close(ctx context.Context, sessionID, orgID string) error {
	entry, err := m.getExistingEntry(sessionID, orgID)
	if err != nil {
		return err
	}
	m.dropEntry(sessionID, entry)

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := ensureOrg(entry.orgID, orgID); err != nil {
		return err
	}
	if entry.compute == nil {
		return notFoundError()
	}

	_, shellErr := m.invoke(ctx, entry, "shell_close", Request{SessionID: sessionID})
	deleteErr := m.compute.Delete(ctx, sessionID, orgID)
	if deleteErr != nil && strings.Contains(deleteErr.Error(), "org mismatch") {
		return orgMismatchError()
	}
	if shellErr != nil {
		if _, ok := shellErr.(*transportError); ok && deleteErr == nil {
			return nil
		}
		return shellErr
	}
	if deleteErr != nil {
		return notFoundError()
	}
	return nil
}

func ensureOrg(boundOrgID, orgID string) error {
	if orgID != "" && boundOrgID != "" && orgID != boundOrgID {
		return orgMismatchError()
	}
	return nil
}

func (m *Manager) getOrCreateEntry(sessionID, orgID string) (*managedSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.sessions[sessionID]; ok {
		return entry, false
	}
	entry := &managedSession{orgID: orgID, artifactSeen: make(map[string]artifactVersion)}
	m.sessions[sessionID] = entry
	return entry, true
}

func (m *Manager) getExistingEntry(sessionID, orgID string) (*managedSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.sessions[sessionID]
	if !ok {
		return nil, notFoundError()
	}
	if err := ensureOrg(entry.orgID, orgID); err != nil {
		return nil, err
	}
	return entry, nil
}

func (m *Manager) dropEntry(sessionID string, entry *managedSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.sessions[sessionID]; ok && current == entry {
		delete(m.sessions, sessionID)
	}
}

func (m *Manager) invoke(ctx context.Context, entry *managedSession, action string, req Request) (*AgentShellResponse, error) {
	if entry.compute == nil {
		return nil, notFoundError()
	}
	entry.compute.TouchActivity()
	callTimeout := time.Duration(maxInt(req.TimeoutMs, req.YieldTimeMs, 5000)+5000) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	conn, err := entry.compute.Dial(ctx)
	if err != nil {
		return nil, &transportError{err: fmt.Errorf("connect to agent: %w", err)}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(callTimeout))

	env := AgentRequest{
		Action: action,
		Shell: &AgentShellRequest{
			Cwd:         req.Cwd,
			Command:     req.Command,
			Input:       req.Input,
			Signal:      req.Signal,
			Cursor:      req.Cursor,
			TimeoutMs:   req.TimeoutMs,
			YieldTimeMs: req.YieldTimeMs,
		},
	}
	if err := json.NewEncoder(conn).Encode(env); err != nil {
		return nil, &transportError{err: fmt.Errorf("send shell request: %w", err)}
	}

	respBody, err := io.ReadAll(conn)
	if err != nil {
		return nil, &transportError{err: fmt.Errorf("read shell response: %w", err)}
	}

	var resp AgentResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, &transportError{err: fmt.Errorf("decode shell response: %w", err)}
	}
	if resp.Error != "" {
		switch resp.Code {
		case CodeSessionBusy:
			return nil, busyError()
		case CodeSessionNotFound:
			return nil, notFoundError()
		case CodeInvalidCursor:
			return nil, invalidCursorError()
		case CodeNotRunning:
			return nil, notRunningError()
		case CodeSignalFailed:
			return nil, signalFailedError(resp.Error)
		default:
			return nil, errors.New(resp.Error)
		}
	}
	if resp.Shell == nil {
		return nil, &transportError{err: fmt.Errorf("shell response missing body")}
	}
	return resp.Shell, nil
}

func (m *Manager) attachArtifacts(ctx context.Context, sessionID string, entry *managedSession, result *AgentShellResponse, resp *Response) {
	if result == nil || result.Running || result.ExitCode == nil {
		return
	}
	if entry.commandSeq == 0 || entry.uploadedSeq >= entry.commandSeq {
		return
	}
	refs, nextKnown := collectArtifacts(ctx, entry.compute, sessionID, entry.commandSeq, m.artifactStore, entry.artifactSeen, m.logger)
	entry.uploadedSeq = entry.commandSeq
	entry.artifactSeen = nextKnown
	resp.Artifacts = refs
	if resp.Artifacts == nil {
		resp.Artifacts = []ArtifactRef{}
	}
}

func (m *Manager) toResponse(sessionID string, entry *managedSession, result *AgentShellResponse) *Response {
	resp := &Response{
		SessionID: sessionID,
		Status:    result.Status,
		Cwd:       result.Cwd,
		Output:    result.Output,
		Cursor:    result.Cursor,
		Running:   result.Running,
		Truncated: result.Truncated,
		TimedOut:  result.TimedOut,
		ExitCode:  result.ExitCode,
	}
	if result.Running {
		resp.Status = StatusRunning
	}
	if !result.Running && resp.Status == "" {
		resp.Status = StatusIdle
	}
	return resp
}

func maxInt(values ...int) int {
	max := 0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}
