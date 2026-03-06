package shell

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"sync"
	"testing"

	"arkloop/services/sandbox/internal/logging"
	"arkloop/services/sandbox/internal/session"
)

func TestManagerOpen_IdempotentReuse(t *testing.T) {
	agent := &fakeAgent{
		handler: func(req AgentRequest) AgentResponse {
			return AgentResponse{Action: req.Action, Shell: &AgentShellResponse{Status: StatusIdle, Cwd: "/workspace", Cursor: 0}}
		},
	}
	pool := &fakePool{agent: agent}
	mgr := session.NewManager(session.ManagerConfig{MaxSessions: 10, Pool: pool, MaxLifetimeSeconds: 3600})
	shellMgr := NewManager(mgr, nil, logging.NewJSONLogger("test", nil))

	for range 2 {
		resp, err := shellMgr.Open(context.Background(), Request{SessionID: "sess-1", Tier: "lite", OrgID: "org-a"})
		if err != nil {
			t.Fatalf("open failed: %v", err)
		}
		if resp.Status != StatusIdle {
			t.Fatalf("expected idle, got %s", resp.Status)
		}
	}

	if pool.acquireCount != 1 {
		t.Fatalf("expected acquire once, got %d", pool.acquireCount)
	}
}

func TestManagerExec_Busy(t *testing.T) {
	agent := &fakeAgent{
		handler: func(req AgentRequest) AgentResponse {
			switch req.Action {
			case "shell_open":
				return AgentResponse{Action: req.Action, Shell: &AgentShellResponse{Status: StatusIdle, Cwd: "/workspace", Cursor: 0}}
			case "shell_exec":
				return AgentResponse{Action: req.Action, Code: CodeSessionBusy, Error: "shell session is busy"}
			default:
				return AgentResponse{Action: req.Action, Shell: &AgentShellResponse{Status: StatusIdle, Cwd: "/workspace", Cursor: 0}}
			}
		},
	}
	pool := &fakePool{agent: agent}
	mgr := session.NewManager(session.ManagerConfig{MaxSessions: 10, Pool: pool, MaxLifetimeSeconds: 3600})
	shellMgr := NewManager(mgr, nil, logging.NewJSONLogger("test", nil))

	if _, err := shellMgr.Open(context.Background(), Request{SessionID: "sess-1", Tier: "lite", OrgID: "org-a"}); err != nil {
		t.Fatalf("open failed: %v", err)
	}
	_, err := shellMgr.Exec(context.Background(), Request{SessionID: "sess-1", OrgID: "org-a", Command: "sleep 1"})
	if err == nil {
		t.Fatal("expected busy error")
	}
	shellErr, ok := err.(*Error)
	if !ok || shellErr.Code != CodeSessionBusy {
		t.Fatalf("expected busy shell error, got %#v", err)
	}
}

func TestManagerOrgMismatch(t *testing.T) {
	agent := &fakeAgent{
		handler: func(req AgentRequest) AgentResponse {
			return AgentResponse{Action: req.Action, Shell: &AgentShellResponse{Status: StatusIdle, Cwd: "/workspace", Cursor: 0}}
		},
	}
	pool := &fakePool{agent: agent}
	mgr := session.NewManager(session.ManagerConfig{MaxSessions: 10, Pool: pool, MaxLifetimeSeconds: 3600})
	shellMgr := NewManager(mgr, nil, logging.NewJSONLogger("test", nil))

	if _, err := shellMgr.Open(context.Background(), Request{SessionID: "sess-1", Tier: "lite", OrgID: "org-a"}); err != nil {
		t.Fatalf("open failed: %v", err)
	}
	_, err := shellMgr.Read(context.Background(), Request{SessionID: "sess-1", OrgID: "org-b"})
	if err == nil {
		t.Fatal("expected org mismatch")
	}
	shellErr, ok := err.(*Error)
	if !ok || shellErr.Code != CodeOrgMismatch {
		t.Fatalf("expected org mismatch shell error, got %#v", err)
	}
}

func TestManagerClose_ReclaimsComputeSession(t *testing.T) {
	agent := &fakeAgent{
		handler: func(req AgentRequest) AgentResponse {
			return AgentResponse{Action: req.Action, Shell: &AgentShellResponse{Status: StatusIdle, Cwd: "/workspace", Cursor: 0}}
		},
	}
	pool := &fakePool{agent: agent}
	mgr := session.NewManager(session.ManagerConfig{MaxSessions: 10, Pool: pool, MaxLifetimeSeconds: 3600})
	shellMgr := NewManager(mgr, nil, logging.NewJSONLogger("test", nil))

	if _, err := shellMgr.Open(context.Background(), Request{SessionID: "sess-1", Tier: "lite", OrgID: "org-a"}); err != nil {
		t.Fatalf("open failed: %v", err)
	}
	if err := shellMgr.Close(context.Background(), "sess-1", "org-a"); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if pool.destroyCount != 1 {
		t.Fatalf("expected destroy once, got %d", pool.destroyCount)
	}
}

type fakePool struct {
	mu           sync.Mutex
	agent        *fakeAgent
	acquireCount int
	destroyCount int
}

func (p *fakePool) Acquire(_ context.Context, tier string) (*session.Session, *os.Process, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.acquireCount++
	return &session.Session{
		Tier:      tier,
		SocketDir: "fake-socket",
		Dial:      p.agent.Dial,
	}, nil, nil
}

func (p *fakePool) DestroyVM(_ *os.Process, _ string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.destroyCount++
}

func (p *fakePool) Ready() bool              { return true }
func (p *fakePool) Stats() session.PoolStats { return session.PoolStats{} }
func (p *fakePool) Drain(_ context.Context)  {}

type fakeAgent struct {
	handler func(req AgentRequest) AgentResponse
}

func (a *fakeAgent) Dial(_ context.Context) (net.Conn, error) {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		var req AgentRequest
		if err := json.NewDecoder(server).Decode(&req); err != nil {
			return
		}
		_ = json.NewEncoder(server).Encode(a.handler(req))
	}()
	return client, nil
}
