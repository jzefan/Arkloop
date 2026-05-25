package kb

import (
	"context"
	"encoding/json"
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
	errorConfirmation     = "tool.confirmation_required"
	errorPermissionDenied = "tool.permission_denied"
	errorSearchFailed     = "tool.search_failed"
	errorNotConfigured    = "config.missing"
)

type ChunksReader interface {
	Search(ctx context.Context, kbID uuid.UUID, query []float32, k int) ([]wdata.KBChunkHit, error)
	ListHeadings(ctx context.Context, kbID, docID uuid.UUID) ([]wdata.KBChunkHit, error)
	GetByIDs(ctx context.Context, kbID uuid.UUID, ids []uuid.UUID) ([]wdata.KBChunkHit, error)
	Dim() int
}

type AccessChecker interface {
	IsActorWorkspaceMember(ctx context.Context, accountID, kbID uuid.UUID) (bool, error)
}

type Executor struct {
	chunks    ChunksReader
	embedder  embedding.Embedder
	access    AccessChecker
	provider  ProviderClient
	pool      *pgxpool.Pool
	configErr error
	searchErr error
}

type ProviderClient interface {
	// CallExam invokes an exam endpoint as a specific ArkLoop user. Used by
	// exam-agent personal-data tools. Not used by linked-mode read paths
	// after the admin-credential migration.
	CallExam(ctx context.Context, userID uuid.UUID, scopes []string, method, path string, body any, out any) error
	// CallExamAsAdmin invokes an exam endpoint with a cached admin bearer
	// token. Used by linked-mode read paths so that ArkLoop teachers can
	// see exam's platform-administrator question bank (e.g. 国考医学题库)
	// even though their own accounts lack visibility.
	CallExamAsAdmin(ctx context.Context, method, path string, body any, out any) error
}

func NewExecutor(chunks ChunksReader, embedder embedding.Embedder, access AccessChecker) *Executor {
	return &Executor{chunks: chunks, embedder: embedder, access: access}
}

func NewToolExecutor(pool *pgxpool.Pool) *Executor {
	return NewToolExecutorWithProvider(pool, nil)
}

func NewToolExecutorWithProvider(pool *pgxpool.Pool, provider ProviderClient) *Executor {
	if pool == nil {
		return &Executor{configErr: errors.New("database pool not configured")}
	}
	chunks, err := wdata.NewKBChunksRepository(pool)
	if err != nil {
		return &Executor{configErr: err}
	}
	apiKey := firstNonEmpty(os.Getenv("ARK_EMBED_API_KEY"), os.Getenv("ARK_API_KEY"))
	exec := NewExecutor(chunks, nil, DBAccessChecker{Pool: pool})
	exec.pool = pool
	exec.provider = provider
	if apiKey == "" {
		exec.searchErr = errors.New("ARK_EMBED_API_KEY/ARK_API_KEY not configured")
		return exec
	}
	exec.embedder = embedding.NewDoubao(embedding.DoubaoConfig{
		BaseURL:    firstNonEmpty(os.Getenv("ARK_EMBED_BASE_URL"), "https://ark.cn-beijing.volces.com/api/v3"),
		APIKey:     apiKey,
		Model:      firstNonEmpty(os.Getenv("ARK_EMBED_MODEL"), "doubao-embedding-text-240715"),
		BatchSize:  embedBatchSizeFromEnv(),
		MaxRetries: 3,
		Dim:        chunks.Dim(),
	})
	return exec
}

func (e *Executor) IsNotConfigured() bool {
	return e == nil || e.configErr != nil || e.chunks == nil || e.access == nil
}

func (e *Executor) Execute(ctx context.Context, toolName string, args map[string]any, execCtx tools.ExecutionContext, toolCallID string) tools.ExecutionResult {
	switch toolName {
	case ToolNameSearch:
		return e.executeSearch(ctx, args, execCtx)
	case ToolNameExtractTOC:
		return e.executeExtractTOC(ctx, args, execCtx)
	case ToolNameListKnowledgeBases:
		return e.executeListKnowledgeBases(ctx, args, execCtx)
	case ToolNameListKnowledgePoints:
		return e.executeListKnowledgePoints(ctx, args, execCtx)
	case ToolNameDraftQuestions:
		return e.executeDraftQuestions(ctx, args, execCtx)
	case ToolNameSaveQuestions:
		return e.executeSaveQuestions(ctx, args, execCtx)
	case ToolNameComposePaper:
		return e.executeComposePaper(ctx, args, execCtx)
	default:
		return errResult(errorNotConfigured, "unknown kb tool: "+toolName)
	}
}

func (e *Executor) executeSearch(ctx context.Context, args map[string]any, execCtx tools.ExecutionContext) tools.ExecutionResult {
	if e.IsNotConfigured() {
		return errResult(errorNotConfigured, "kb_search is not configured")
	}
	if e.embedder == nil {
		msg := "kb_search embedder is not configured"
		if e.searchErr != nil {
			msg = msg + ": " + e.searchErr.Error()
		}
		return errResult(errorNotConfigured, msg)
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

func (e *Executor) searchHits(ctx context.Context, kbID uuid.UUID, query string, k int) ([]wdata.KBChunkHit, error) {
	if e.embedder == nil {
		if e.searchErr != nil {
			return nil, e.searchErr
		}
		return nil, errors.New("embedder not configured")
	}
	vecs, err := e.embedder.Embed(ctx, []string{query})
	if err != nil || len(vecs) != 1 {
		if err == nil {
			err = fmt.Errorf("expected 1 embedding vector, got %d", len(vecs))
		}
		return nil, err
	}
	return e.chunks.Search(ctx, kbID, vecs[0], k)
}

func hitToMap(hit wdata.KBChunkHit) map[string]any {
	return map[string]any{
		"id":           hit.ID.String(),
		"document_ref": hit.DocumentRef,
		"ordinal":      hit.Ordinal,
		"heading_path": hit.HeadingPath,
		"chunk_type":   hit.ChunkType,
		"text":         hit.Text,
		"score":        hit.Score,
		"metadata":     hit.Metadata,
	}
}

func jsonBytes(v any, fallback string) []byte {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return []byte(fallback)
	}
	return b
}

func (e *Executor) prepareQuestionSources(ctx context.Context, kbID uuid.UUID, q map[string]any) (string, string, error) {
	if q == nil {
		return "validation_error", "question must be an object", errors.New("question must be an object")
	}
	if snippets := normalizeSourceSnippets(q["source_snippets"]); len(snippets) > 0 {
		q["source_snippets"] = snippets
		return "", "", nil
	}
	chunkIDs, err := parseUUIDSlice(q["source_chunk_ids"])
	if err != nil {
		return "source_chunk_ids_invalid", "source_chunk_ids must be UUID strings", err
	}
	if len(chunkIDs) == 0 {
		return "source_required", "source_snippets or source_chunk_ids are required", errors.New("missing source")
	}
	if e == nil || e.chunks == nil {
		return "source_unavailable", "source chunks are not available", errors.New("chunks reader not configured")
	}
	chunks, err := e.chunks.GetByIDs(ctx, kbID, chunkIDs)
	if err != nil {
		return "source_lookup_failed", "failed to read source chunks", err
	}
	if len(chunks) == 0 {
		return "source_not_found", "source_chunk_ids did not match this knowledge base", errors.New("source chunks not found")
	}
	snippets := make([]map[string]any, 0, len(chunks))
	matchedIDs := make([]string, 0, len(chunks))
	for _, hit := range chunks {
		text := trimRunes(strings.TrimSpace(hit.Text), 500)
		if text == "" {
			continue
		}
		item := map[string]any{
			"chunk_id":     hit.ID.String(),
			"document_ref": hit.DocumentRef,
			"ordinal":      hit.Ordinal,
			"snippet":      text,
		}
		if len(hit.HeadingPath) > 0 {
			item["heading_path"] = hit.HeadingPath
		}
		snippets = append(snippets, item)
		matchedIDs = append(matchedIDs, hit.ID.String())
	}
	if len(snippets) == 0 {
		return "source_not_found", "source chunks have no readable text", errors.New("empty source chunks")
	}
	q["source_snippets"] = snippets
	q["source_chunk_ids"] = matchedIDs
	return "", "", nil
}

func normalizeSourceSnippets(v any) []map[string]any {
	switch items := v.(type) {
	case []map[string]any:
		return normalizeSourceSnippetMaps(items)
	case []any:
		maps := make([]map[string]any, 0, len(items))
		for _, raw := range items {
			if item, ok := raw.(map[string]any); ok {
				maps = append(maps, item)
			}
		}
		return normalizeSourceSnippetMaps(maps)
	default:
		return nil
	}
}

func normalizeSourceSnippetMaps(items []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(firstNonEmpty(asString(item["snippet"]), asString(item["text"]), asString(item["source_text"])))
		if text == "" {
			continue
		}
		cleaned := make(map[string]any, len(item)+1)
		for k, v := range item {
			cleaned[k] = v
		}
		cleaned["snippet"] = trimRunes(text, 500)
		delete(cleaned, "text")
		delete(cleaned, "source_text")
		out = append(out, cleaned)
	}
	return out
}

func trimRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max]))
}

func (e *Executor) executeExtractTOC(ctx context.Context, args map[string]any, execCtx tools.ExecutionContext) tools.ExecutionResult {
	if e.IsNotConfigured() {
		return errResult(errorNotConfigured, "kb_extract_toc is not configured")
	}
	accountID := uuid.Nil
	if execCtx.AccountID != nil {
		accountID = *execCtx.AccountID
	}
	if accountID == uuid.Nil {
		return errResult(errorPermissionDenied, "kb_extract_toc requires an account context")
	}
	kbID, err := uuid.Parse(strings.TrimSpace(asString(args["kb_id"])))
	if err != nil || kbID == uuid.Nil {
		return errResult(errorArgsInvalid, "kb_id is required and must be a UUID")
	}
	docID, err := uuid.Parse(strings.TrimSpace(asString(args["document_id"])))
	if err != nil || docID == uuid.Nil {
		return errResult(errorArgsInvalid, "document_id is required and must be a UUID")
	}
	ok, err := e.access.IsActorWorkspaceMember(ctx, accountID, kbID)
	if err != nil {
		return errResult(errorSearchFailed, "access check failed: "+err.Error())
	}
	if !ok {
		return errResult(errorPermissionDenied, "caller is not a member of this KB workspace")
	}
	// Extract TOC from document's parse_meta_json via direct query
	toc, nodeCount, err := e.extractTOCFromDB(ctx, kbID, docID)
	if err != nil {
		return errResult(errorSearchFailed, "extract toc: "+err.Error())
	}
	return tools.ExecutionResult{ResultJSON: map[string]any{"tree": toc, "node_count": nodeCount}}
}

func (e *Executor) extractTOCFromDB(ctx context.Context, kbID, docID uuid.UUID) (any, int, error) {
	if e.chunks == nil {
		return nil, 0, errors.New("chunks reader not configured")
	}
	// Query heading chunks for this document, ordered by ordinal
	hits, err := e.chunks.ListHeadings(ctx, kbID, docID)
	if err != nil {
		return nil, 0, err
	}
	if len(hits) < 5 {
		return nil, len(hits), nil
	}
	// Build tree from heading_path arrays
	type tocNode struct {
		Name     string     `json:"name"`
		Children []*tocNode `json:"children,omitempty"`
	}
	root := &tocNode{Name: "root"}
	for _, h := range hits {
		current := root
		for _, segment := range h.HeadingPath {
			found := false
			for _, child := range current.Children {
				if child.Name == segment {
					current = child
					found = true
					break
				}
			}
			if !found {
				node := &tocNode{Name: segment}
				current.Children = append(current.Children, node)
				current = node
			}
		}
	}
	return root.Children, len(hits), nil
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

func parseStringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
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
