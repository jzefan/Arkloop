package kb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"arkloop/services/shared/embedding"
	wdata "arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	errorArgsInvalid      = "tool.args_invalid"
	errorPermissionDenied = "tool.permission_denied"
	errorSearchFailed     = "tool.search_failed"
	errorNotConfigured    = "config.missing"
)

type ChunksReader interface {
	Search(ctx context.Context, kbID uuid.UUID, query []float32, k int) ([]wdata.KBChunkHit, error)
	Dim() int
}

type AccessChecker interface {
	IsActorWorkspaceMember(ctx context.Context, accountID, kbID uuid.UUID) (bool, error)
}

type Executor struct {
	chunks    ChunksReader
	embedder  embedding.Embedder
	access    AccessChecker
	configErr error
}

func NewExecutor(chunks ChunksReader, embedder embedding.Embedder, access AccessChecker) *Executor {
	return &Executor{chunks: chunks, embedder: embedder, access: access}
}

func NewToolExecutor(pool *pgxpool.Pool) *Executor {
	if pool == nil {
		return &Executor{configErr: errors.New("database pool not configured")}
	}
	chunks, err := wdata.NewKBChunksRepository(pool)
	if err != nil {
		return &Executor{configErr: err}
	}
	apiKey := firstNonEmpty(os.Getenv("ARK_EMBED_API_KEY"), os.Getenv("ARK_API_KEY"))
	if apiKey == "" {
		return &Executor{configErr: errors.New("ARK_EMBED_API_KEY/ARK_API_KEY not configured")}
	}
	embedder := embedding.NewDoubao(embedding.DoubaoConfig{
		BaseURL:    firstNonEmpty(os.Getenv("ARK_EMBED_BASE_URL"), "https://ark.cn-beijing.volces.com/api/v3"),
		APIKey:     apiKey,
		Model:      firstNonEmpty(os.Getenv("ARK_EMBED_MODEL"), "doubao-embedding-text-240715"),
		BatchSize:  embedBatchSizeFromEnv(),
		MaxRetries: 3,
		Dim:        chunks.Dim(),
	})
	return NewExecutor(chunks, embedder, DBAccessChecker{Pool: pool})
}

func (e *Executor) IsNotConfigured() bool {
	return e == nil || e.configErr != nil || e.chunks == nil || e.embedder == nil || e.access == nil
}

func (e *Executor) Execute(ctx context.Context, toolName string, args map[string]any, execCtx tools.ExecutionContext, toolCallID string) tools.ExecutionResult {
	if e.IsNotConfigured() {
		return errResult(errorNotConfigured, "kb_search is not configured")
	}
	accountID := uuid.Nil
	if execCtx.AccountID != nil {
		accountID = *execCtx.AccountID
	}
	if accountID == uuid.Nil {
		return errResult(errorPermissionDenied, "kb_search requires an account context")
	}
	kbID, err := uuid.Parse(strings.TrimSpace(asString(args["kb_id"])))
	if err != nil || kbID == uuid.Nil {
		return errResult(errorArgsInvalid, "kb_id is required and must be a UUID")
	}
	query := strings.TrimSpace(asString(args["query"]))
	if query == "" {
		return errResult(errorArgsInvalid, "query is required")
	}
	k := intFromAny(args["k"], 8)
	if k <= 0 {
		k = 8
	}
	if k > 50 {
		k = 50
	}
	ok, err := e.access.IsActorWorkspaceMember(ctx, accountID, kbID)
	if err != nil {
		return errResult(errorSearchFailed, "access check failed: "+err.Error())
	}
	if !ok {
		return errResult(errorPermissionDenied, "caller is not a member of this KB workspace")
	}
	vecs, err := e.embedder.Embed(ctx, []string{query})
	if err != nil || len(vecs) != 1 {
		if err == nil {
			err = fmt.Errorf("expected 1 embedding vector, got %d", len(vecs))
		}
		return errResult(errorSearchFailed, "embed query failed: "+err.Error())
	}
	hits, err := e.chunks.Search(ctx, kbID, vecs[0], k)
	if err != nil {
		return errResult(errorSearchFailed, "search failed: "+err.Error())
	}
	out := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		out = append(out, map[string]any{
			"document_ref": hit.DocumentRef,
			"ordinal":      hit.Ordinal,
			"heading_path": hit.HeadingPath,
			"chunk_type":   hit.ChunkType,
			"text":         hit.Text,
			"score":        hit.Score,
			"metadata":     hit.Metadata,
		})
	}
	return tools.ExecutionResult{ResultJSON: map[string]any{"hits": out}}
}

type DBAccessChecker struct {
	Pool *pgxpool.Pool
}

func (c DBAccessChecker) IsActorWorkspaceMember(ctx context.Context, accountID, kbID uuid.UUID) (bool, error) {
	if c.Pool == nil {
		return false, nil
	}
	var ok bool
	err := c.Pool.QueryRow(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM knowledge_bases
	WHERE id = $1 AND account_id = $2
)`, kbID, accountID).Scan(&ok)
	return ok, err
}

func errResult(class, msg string) tools.ExecutionResult {
	return tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: class, Message: msg}}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func intFromAny(v any, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case jsonNumber:
		if i, err := strconv.Atoi(t.String()); err == nil {
			return i
		}
	}
	return fallback
}

type jsonNumber interface{ String() string }

func embedBatchSizeFromEnv() int {
	if raw := strings.TrimSpace(os.Getenv("ARK_EMBED_BATCH")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	return 32
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if cleaned := strings.TrimSpace(value); cleaned != "" {
			return cleaned
		}
	}
	return ""
}
