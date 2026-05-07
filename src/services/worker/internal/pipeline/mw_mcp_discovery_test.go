package pipeline

import (
	"context"
	"testing"

	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/mcp"
	"arkloop/services/worker/internal/tools"
	builtin "arkloop/services/worker/internal/tools/builtin"
)

func TestMCPDiscoveryMiddleware_NoCachePassThrough(t *testing.T) {
	mw := NewMCPDiscoveryMiddleware(
		nil, // no cache
		nil, // no queryer
		nil, // no events repo
		map[string]tools.Executor{"echo": builtin.EchoExecutor{}},
		[]llm.ToolSpec{builtin.EchoLlmSpec},
		map[string]struct{}{"echo": {}},
		tools.NewRegistry(),
	)

	rc := &RunContext{
		Emitter: events.NewEmitter("test"),
	}

	var reached bool
	h := Build([]RunMiddleware{mw}, func(_ context.Context, rc *RunContext) error {
		reached = true
		if len(rc.ToolExecutors) != 1 {
			t.Fatalf("expected 1 executor, got %d", len(rc.ToolExecutors))
		}
		if len(rc.ToolSpecs) != 1 {
			t.Fatalf("expected 1 spec, got %d", len(rc.ToolSpecs))
		}
		return nil
	})
	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reached {
		t.Fatal("terminal handler was not called")
	}
}

func TestBuildMCPDiscoveryEventData_TrimsSlowServers(t *testing.T) {
	data := buildMCPDiscoveryEventData(
		"failed",
		4200,
		3000,
		mcp.CacheResult{TTL: 30},
		mcp.DiscoverDiagnostics{
			ServerCount: 4,
			ToolCount:   7,
			Servers: []mcp.ServerDiagnostics{
				{ServerID: "slow-ok", Transport: "stdio", DurationMs: 4100, Outcome: "ok", ToolCount: 3},
				{ServerID: "fast-ok", Transport: "stdio", DurationMs: 200, Outcome: "ok", ToolCount: 2},
				{ServerID: "timeout", Transport: "streamable_http", DurationMs: 3900, Outcome: "list_failed", ErrorClass: "timeout"},
				{ServerID: "empty", Transport: "stdio", DurationMs: 3200, Outcome: "empty"},
			},
		},
		mcp.TimeoutError{Message: "timed out"},
	)

	if got := data["status"]; got != "failed" {
		t.Fatalf("unexpected status: %v", got)
	}
	if got := data["server_failed"]; got != 1 {
		t.Fatalf("unexpected server_failed: %v", got)
	}
	servers, ok := data["servers"].([]map[string]any)
	if !ok {
		t.Fatalf("expected server summaries, got %T", data["servers"])
	}
	if len(servers) != 3 {
		t.Fatalf("expected 3 summarized servers, got %d", len(servers))
	}
	if servers[0]["server_id"] != "slow-ok" {
		t.Fatalf("expected slowest server first, got %v", servers[0]["server_id"])
	}
}
