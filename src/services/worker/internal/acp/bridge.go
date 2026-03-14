package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"arkloop/services/worker/internal/events"
)

// transport abstracts the sandbox ACP client for testing.
type transport interface {
	Start(ctx context.Context, req StartRequest) (*StartResponse, error)
	Write(ctx context.Context, req WriteRequest) error
	Read(ctx context.Context, req ReadRequest) (*ReadResponse, error)
	Stop(ctx context.Context, req StopRequest) error
	Wait(ctx context.Context, req WaitRequest) (*WaitResponse, error)
}

const (
	defaultPollInterval = 500 * time.Millisecond
	defaultReadMaxBytes = 32 * 1024
)

// BridgeConfig holds configuration for a single ACP bridge run.
type BridgeConfig struct {
	SandboxBaseURL   string
	SandboxAuthToken string
	SessionID        string // sandbox session ID (typically run ID)
	AccountID        string
	Tier             string
	Command          []string          // agent launch command, e.g. ["opencode","acp","--cwd","/workspace"]
	Cwd              string            // workspace directory inside sandbox
	Env              map[string]string

	PollInterval   time.Duration // how often to read stdout, default 500ms
	ReadMaxBytes   int           // max bytes per read, default 32KB
	KillGraceMs    int           // configurable kill grace period
	CleanupDelayMs int           // configurable cleanup delay
}

// Bridge manages a single ACP session lifecycle.
type Bridge struct {
	tr           transport
	config       BridgeConfig
	processID    string // set after Start
	acpSessionID string // set after session/new
	agentVersion string // set after Start
	cursor       uint64 // read cursor for stdout
	msgIDSeq     int    // JSON-RPC message ID sequence
}

// NewBridge creates a Bridge. The transport is typically a *Client.
func NewBridge(tr transport, config BridgeConfig) *Bridge {
	if config.PollInterval == 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.ReadMaxBytes == 0 {
		config.ReadMaxBytes = defaultReadMaxBytes
	}
	return &Bridge{tr: tr, config: config}
}

func (b *Bridge) nextID() int {
	b.msgIDSeq++
	return b.msgIDSeq
}

// Run executes the full ACP lifecycle: start -> session/new -> prompt -> poll -> cleanup.
// It yields RunEvents through the provided callback. ctx cancellation triggers session/cancel.
func (b *Bridge) Run(ctx context.Context, prompt string, emitter events.Emitter, yield func(events.RunEvent) error) error {
	cmd := b.config.Command
	if len(cmd) == 0 {
		return fmt.Errorf("acp bridge: command not configured")
	}

	resp, err := b.tr.Start(ctx, StartRequest{
		SessionID:      b.config.SessionID,
		AccountID:      b.config.AccountID,
		Tier:           b.config.Tier,
		Command:        cmd,
		Cwd:            b.config.Cwd,
		Env:            b.config.Env,
		KillGraceMs:    b.config.KillGraceMs,
		CleanupDelayMs: b.config.CleanupDelayMs,
	})
	if err != nil {
		return fmt.Errorf("bridge: start opencode process: %w", err)
	}
	b.processID = resp.ProcessID
	b.agentVersion = resp.AgentVersion
	slog.Info("acp: agent process started", "process_id", b.processID, "session_id", b.config.SessionID, "command", cmd[0])
	defer b.cleanup()

	if err := b.sendMessage(ctx, NewSessionNewMessage(b.nextID(), SessionModeCode, b.config.Cwd)); err != nil {
		return fmt.Errorf("bridge: send session/new: %w", err)
	}
	if err := b.waitForSessionNew(ctx); err != nil {
		return fmt.Errorf("bridge: wait for session/new response: %w", err)
	}
	slog.Info("acp: session created", "acp_session_id", b.acpSessionID)

	if err := yield(emitter.Emit("run.started", map[string]any{
		"status":        "working",
		"command":       cmd,
		"agent_version": b.agentVersion,
		"session_id":    b.config.SessionID,
	}, nil, nil)); err != nil {
		return err
	}

	if err := b.sendMessage(ctx, NewSessionPromptMessage(b.nextID(), b.acpSessionID, prompt)); err != nil {
		return fmt.Errorf("bridge: send session/prompt: %w", err)
	}

	return b.pollUpdates(ctx, emitter, yield)
}

func (b *Bridge) sendMessage(ctx context.Context, msg ACPMessage) error {
	data, err := MarshalMessage(msg)
	if err != nil {
		return err
	}
	return b.tr.Write(ctx, WriteRequest{
		SessionID: b.config.SessionID,
		AccountID: b.config.AccountID,
		ProcessID: b.processID,
		Data:      string(data),
	})
}

func (b *Bridge) read(ctx context.Context) (*ReadResponse, error) {
	return b.tr.Read(ctx, ReadRequest{
		SessionID: b.config.SessionID,
		AccountID: b.config.AccountID,
		ProcessID: b.processID,
		Cursor:    b.cursor,
		MaxBytes:  b.config.ReadMaxBytes,
	})
}

func (b *Bridge) waitForSessionNew(ctx context.Context) error {
	for {
		resp, err := b.read(ctx)
		if err != nil {
			return fmt.Errorf("bridge: read stdout: %w", err)
		}
		b.cursor = resp.NextCursor

		if resp.Data != "" {
			if sid, ok := parseSessionNewResponse(resp.Data); ok {
				b.acpSessionID = sid
				return nil
			}
		}

		if resp.Exited {
			return fmt.Errorf("bridge: opencode process exited before session/new response (check sandbox logs)")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(b.config.PollInterval):
		}
	}
}

// parseSessionNewResponse extracts session_id from a JSON-RPC result in stdout data.
func parseSessionNewResponse(data string) (string, bool) {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg ACPMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Result == nil {
			continue
		}
		raw, err := json.Marshal(msg.Result)
		if err != nil {
			continue
		}
		var result SessionNewResult
		if err := json.Unmarshal(raw, &result); err != nil {
			continue
		}
		if result.SessionID != "" {
			return result.SessionID, true
		}
	}
	return "", false
}

func (b *Bridge) pollUpdates(ctx context.Context, emitter events.Emitter, yield func(events.RunEvent) error) error {
	for {
		resp, err := b.read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return b.handleCancellation(emitter, yield)
			}
			return fmt.Errorf("bridge: read stdout: %w", err)
		}
		b.cursor = resp.NextCursor

		if resp.Data != "" {
			updates, err := ParseUpdates(resp.Data)
			if err != nil {
				slog.Warn("acp: parse updates failed", "error", err)
			}
			for _, u := range updates {
				// Handle permission requests: auto-approve and send response
				if u.Type == UpdateTypePermission {
					slog.Info("acp: permission requested",
						"permission_id", u.PermissionID,
						"description", u.Content,
						"sensitive", u.Sensitive,
						"session_id", b.config.SessionID,
					)
					ev := emitter.Emit("acp.permission_required", map[string]any{
						"permission_id": u.PermissionID,
						"description":   u.Content,
						"sensitive":     u.Sensitive,
						"approved":      true,
						"reason":        "auto-approved by governance policy",
					}, nil, nil)
					if err := yield(ev); err != nil {
						return err
					}
					if b.acpSessionID != "" {
						approveMsg := NewSessionPermissionMessage(
							b.nextID(), b.acpSessionID, u.PermissionID, true, "auto-approved",
						)
						if sendErr := b.sendMessage(ctx, approveMsg); sendErr != nil {
							slog.Warn("acp: send permission response failed", "error", sendErr)
						}
					}
					continue
				}

				ev, ok := mapUpdateToEvent(u, emitter)
				if !ok || ev.Type == "run.started" {
					continue // run.started already emitted before poll loop
				}
				if err := yield(ev); err != nil {
					return err
				}
				if u.Type == UpdateTypeComplete || u.Type == UpdateTypeError {
					return nil
				}
			}
		}

		if resp.Exited {
			errClass := "acp.process_exited"
			diagnostic := map[string]any{
				"error_class":   errClass,
				"message":       "opencode process exited unexpectedly",
				"layer":         "opencode",
				"process_id":    b.processID,
				"command":       b.config.Command,
				"agent_version": b.agentVersion,
			}
			if resp.ErrorSummary != "" {
				diagnostic["error_summary"] = resp.ErrorSummary
			}
			if resp.Stderr != "" && len(resp.Stderr) <= 1024 {
				diagnostic["stderr_tail"] = resp.Stderr
			} else if resp.Stderr != "" {
				diagnostic["stderr_tail"] = resp.Stderr[len(resp.Stderr)-1024:]
			}
			return yield(emitter.Emit("run.failed", diagnostic, nil, &errClass))
		}

		select {
		case <-ctx.Done():
			return b.handleCancellation(emitter, yield)
		case <-time.After(b.config.PollInterval):
		}
	}
}

func (b *Bridge) handleCancellation(emitter events.Emitter, yield func(events.RunEvent) error) error {
	if b.acpSessionID != "" {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := b.sendMessage(cancelCtx, NewSessionCancelMessage(b.nextID(), b.acpSessionID)); err != nil {
			slog.Warn("acp: send session/cancel failed", "error", err)
		}
	}
	return yield(emitter.Emit("run.cancelled", map[string]any{
		"reason": "context_cancelled",
	}, nil, nil))
}

func (b *Bridge) cleanup() {
	if b.processID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := b.tr.Stop(ctx, StopRequest{
		SessionID: b.config.SessionID,
		AccountID: b.config.AccountID,
		ProcessID: b.processID,
	}); err != nil {
		slog.Warn("acp: stop process failed", "error", err, "process_id", b.processID)
	}
}

func mapUpdateToEvent(update SessionUpdateParams, emitter events.Emitter) (events.RunEvent, bool) {
	switch update.Type {
	case UpdateTypeStatus:
		if update.Status == StatusWorking {
			return emitter.Emit("run.started", map[string]any{"status": "working"}, nil, nil), true
		}
		return events.RunEvent{}, false

	case UpdateTypeTextDelta:
		return emitter.Emit("message.delta", map[string]any{
			"content_delta": update.Content,
			"role":          "assistant",
		}, nil, nil), true

	case UpdateTypeToolCall:
		name := update.Name
		// tool_call_update with status "completed" has result content
		if update.Status == "completed" {
			return emitter.Emit("tool.result", map[string]any{
				"tool_name": update.Name,
				"output":    update.Output,
			}, &name, nil), true
		}
		return emitter.Emit("tool.call", map[string]any{
			"tool_name": update.Name,
			"arguments": update.Arguments,
		}, &name, nil), true

	case UpdateTypeToolResult:
		name := update.Name
		return emitter.Emit("tool.result", map[string]any{
			"tool_name": update.Name,
			"output":    update.Output,
		}, &name, nil), true

	case UpdateTypeComplete:
		return emitter.Emit("run.completed", map[string]any{
			"summary": update.Summary,
		}, nil, nil), true

	case UpdateTypeError:
		errClass := "acp.agent_error"
		return emitter.Emit("run.failed", map[string]any{
			"error_class": errClass,
			"message":     update.Message,
			"layer":       "opencode",
		}, nil, &errClass), true
	}

	return events.RunEvent{}, false
}
