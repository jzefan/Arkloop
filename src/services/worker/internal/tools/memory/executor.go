package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/memory"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	errorArgsInvalid     = "tool.args_invalid"
	errorProviderFailure = "tool.memory_provider_error"
	errorIdentityMissing = "tool.memory_identity_missing"

	defaultSearchLimit = 5
)

// ToolExecutor 实现 tools.Executor，将 memory_search/read/write/forget 分发到 MemoryProvider。
// pool 非 nil 时 search 操作优先读 PG 快照缓存，避免每次 tool call 都打到 OpenViking。
type ToolExecutor struct {
	provider     memory.MemoryProvider
	pool         *pgxpool.Pool
	snapshotRepo data.MemorySnapshotRepository
}

func NewToolExecutor(provider memory.MemoryProvider, pool *pgxpool.Pool) *ToolExecutor {
	return &ToolExecutor{
		provider: provider,
		pool:     pool,
	}
}

func (e *ToolExecutor) Execute(
	ctx context.Context,
	toolName string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()

	ident, err := buildIdentity(execCtx)
	if err != nil {
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: errorIdentityMissing,
				Message:    err.Error(),
			},
			DurationMs: durationMs(started),
		}
	}

	switch toolName {
	case "memory_search":
		return e.search(ctx, args, ident, started)
	case "memory_read":
		return e.read(ctx, args, ident, started)
	case "memory_write":
		return e.write(ctx, args, ident, started)
	case "memory_forget":
		return e.forget(ctx, args, ident, started)
	default:
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: "tool.not_registered",
				Message:    "unknown memory tool: " + toolName,
			},
			DurationMs: durationMs(started),
		}
	}
}

func (e *ToolExecutor) search(ctx context.Context, args map[string]any, ident memory.MemoryIdentity, started time.Time) tools.ExecutionResult {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return argError("query must be a non-empty string", started)
	}

	scope := parseScope(args)
	limit := parseLimit(args, defaultSearchLimit)

	// user scope 优先从 PG 快照缓存读取，避免 OpenViking 并发瓶颈
	if scope == memory.MemoryScopeUser && e.pool != nil {
		cached, found, err := e.snapshotRepo.GetHits(ctx, e.pool, ident.OrgID, ident.UserID, ident.AgentID)
		if err != nil {
			slog.WarnContext(ctx, "memory: snapshot cache read failed", "err", err.Error())
		} else if found {
			results := make([]map[string]any, 0, min(len(cached), limit))
			for i, h := range cached {
				if i >= limit {
					break
				}
				results = append(results, map[string]any{
					"uri":      h.URI,
					"abstract": h.Abstract,
				})
			}
			return tools.ExecutionResult{
				ResultJSON: map[string]any{"hits": results},
				DurationMs: durationMs(started),
			}
		}
	}

	hits, err := e.provider.Find(ctx, ident, scope, query, limit)
	if err != nil {
		return providerError("search", err, started)
	}

	results := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		results = append(results, map[string]any{
			"uri":      h.URI,
			"abstract": h.Abstract,
		})
	}

	// Find 成功时异步回写缓存，供后续 tool call 使用
	if scope == memory.MemoryScopeUser && e.pool != nil && len(hits) > 0 {
		cachedHits := make([]data.MemoryHitCache, len(hits))
		for i, h := range hits {
			cachedHits[i] = data.MemoryHitCache{
				URI: h.URI, Abstract: h.Abstract, Score: h.Score,
				MatchReason: h.MatchReason, IsLeaf: h.IsLeaf,
			}
		}
		go func() {
			uCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = e.snapshotRepo.UpsertWithHits(uCtx, e.pool, ident.OrgID, ident.UserID, ident.AgentID, "", cachedHits)
		}()
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{"hits": results},
		DurationMs: durationMs(started),
	}
}

func (e *ToolExecutor) read(ctx context.Context, args map[string]any, ident memory.MemoryIdentity, started time.Time) tools.ExecutionResult {
	uri, ok := args["uri"].(string)
	if !ok || strings.TrimSpace(uri) == "" {
		return argError("uri must be a non-empty string", started)
	}

	layer := memory.MemoryLayerOverview
	if depth, ok := args["depth"].(string); ok && depth == "full" {
		layer = memory.MemoryLayerRead
	}

	content, err := e.provider.Content(ctx, ident, uri, layer)
	if err != nil {
		return providerError("read", err, started)
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{"content": content},
		DurationMs: durationMs(started),
	}
}

func (e *ToolExecutor) write(ctx context.Context, args map[string]any, ident memory.MemoryIdentity, started time.Time) tools.ExecutionResult {
	category, ok := args["category"].(string)
	if !ok || strings.TrimSpace(category) == "" {
		return argError("category must be a non-empty string", started)
	}
	key, ok := args["key"].(string)
	if !ok || strings.TrimSpace(key) == "" {
		return argError("key must be a non-empty string", started)
	}
	content, ok := args["content"].(string)
	if !ok || strings.TrimSpace(content) == "" {
		return argError("content must be a non-empty string", started)
	}

	scope := parseScope(args)

	// scope 前缀让 OpenViking LLM 提取时能区分 user/agent 命名空间
	writable := "[" + string(scope) + "/" + category + "/" + key + "] " + content

	entry := memory.MemoryEntry{Content: writable}
	if err := e.provider.Write(ctx, ident, scope, entry); err != nil {
		return providerError("write", err, started)
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{"status": "ok"},
		DurationMs: durationMs(started),
	}
}

func (e *ToolExecutor) forget(ctx context.Context, args map[string]any, ident memory.MemoryIdentity, started time.Time) tools.ExecutionResult {
	uri, ok := args["uri"].(string)
	if !ok || strings.TrimSpace(uri) == "" {
		return argError("uri must be a non-empty string", started)
	}

	if err := e.provider.Delete(ctx, ident, uri); err != nil {
		return providerError("forget", err, started)
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{"status": "ok"},
		DurationMs: durationMs(started),
	}
}

// --- helpers ---

func buildIdentity(execCtx tools.ExecutionContext) (memory.MemoryIdentity, error) {
	if execCtx.UserID == nil {
		return memory.MemoryIdentity{}, fmt.Errorf("user_id not available, memory operations require authenticated user")
	}
	orgID := uuid.Nil
	if execCtx.OrgID != nil {
		orgID = *execCtx.OrgID
	}
	return memory.MemoryIdentity{
		OrgID:   orgID,
		UserID:  *execCtx.UserID,
		AgentID: execCtx.AgentID,
	}, nil
}

func parseScope(args map[string]any) memory.MemoryScope {
	if s, ok := args["scope"].(string); ok && s == "agent" {
		return memory.MemoryScopeAgent
	}
	return memory.MemoryScopeUser
}

func parseLimit(args map[string]any, fallback int) int {
	switch v := args["limit"].(type) {
	case float64:
		if n := int(v); n >= 1 && n <= 20 {
			return n
		}
	case int:
		if v >= 1 && v <= 20 {
			return v
		}
	case int64:
		if v >= 1 && v <= 20 {
			return int(v)
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n >= 1 && n <= 20 {
			return int(n)
		}
	}
	return fallback
}

func argError(msg string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: errorArgsInvalid,
			Message:    msg,
		},
		DurationMs: durationMs(started),
	}
}

func providerError(op string, err error, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: errorProviderFailure,
			Message:    "memory " + op + " failed: " + err.Error(),
		},
		DurationMs: durationMs(started),
	}
}

func durationMs(started time.Time) int {
	ms := int(time.Since(started) / time.Millisecond)
	if ms < 0 {
		return 0
	}
	return ms
}
