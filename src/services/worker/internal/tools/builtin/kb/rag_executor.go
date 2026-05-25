package kb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math/rand"
	"sort"
	"strings"
	"time"

	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const paperBankName = "组卷题库"

type kbDescriptor struct {
	ID              uuid.UUID
	AccountID       uuid.UUID
	WorkspaceRef    string
	IntegrationMode string
	ExamScopeID     *string
}

type questionRow struct {
	ID               uuid.UUID
	KnowledgePointID *uuid.UUID
	Type             string
	Difficulty       string
	Stem             string
	OptionsJSON      []byte
	Answer           string
	Explanation      string
	SourceJSON       []byte
	CreatedAt        time.Time
}

func (e *Executor) executeListKnowledgeBases(ctx context.Context, args map[string]any, execCtx tools.ExecutionContext) tools.ExecutionResult {
	if e.IsNotConfigured() || e.pool == nil {
		return errResult(errorNotConfigured, "kb_list_knowledge_bases is not configured")
	}
	accountID := uuid.Nil
	if execCtx.AccountID != nil {
		accountID = *execCtx.AccountID
	}
	if accountID == uuid.Nil {
		return errResult(errorPermissionDenied, "kb_list_knowledge_bases requires an account context")
	}
	userID := uuid.Nil
	if execCtx.UserID != nil {
		userID = *execCtx.UserID
	}
	workspaceRef := strings.TrimSpace(asString(args["workspace_ref"]))
	if workspaceRef == "" {
		workspaceRef = strings.TrimSpace(execCtx.WorkspaceRef)
	}
	readyOnly := true
	if v, ok := args["ready_only"].(bool); ok {
		readyOnly = v
	}
	includeSystemBanks := false
	if v, ok := args["include_system_banks"].(bool); ok {
		includeSystemBanks = v
	}

	query := `
SELECT kb.id, kb.name, kb.description, kb.workspace_ref, kb.visibility, kb.integration_mode, kb.exam_scope_id,
       COALESCE((SELECT COUNT(*) FROM kb_documents d WHERE d.kb_id = kb.id), 0) AS document_count,
       COALESCE((SELECT COUNT(*) FROM kb_documents d WHERE d.kb_id = kb.id AND d.status = 'ready'), 0) AS ready_document_count
FROM   knowledge_bases kb
WHERE  kb.account_id = $1
  AND  (kb.visibility <> 'private' OR kb.created_by = $2)`
	argsSQL := []any{accountID, userID}
	if workspaceRef != "" {
		argsSQL = append(argsSQL, workspaceRef)
		query += fmt.Sprintf("\n  AND kb.workspace_ref = $%d", len(argsSQL))
	}
	if !includeSystemBanks {
		argsSQL = append(argsSQL, paperBankName)
		query += fmt.Sprintf("\n  AND kb.name <> $%d", len(argsSQL))
	}
	if readyOnly {
		query += "\n  AND EXISTS (SELECT 1 FROM kb_documents d WHERE d.kb_id = kb.id AND d.status = 'ready')"
	}
	query += "\nORDER BY kb.created_at DESC, kb.id ASC"

	rows, err := e.pool.Query(ctx, query, argsSQL...)
	if err != nil {
		return errResult(errorSearchFailed, "list knowledge bases: "+err.Error())
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, description, ws, visibility, mode string
		var examScopeID *string
		var documentCount, readyDocumentCount int
		if err := rows.Scan(&id, &name, &description, &ws, &visibility, &mode, &examScopeID, &documentCount, &readyDocumentCount); err != nil {
			return errResult(errorSearchFailed, "scan knowledge base: "+err.Error())
		}
		item := map[string]any{
			"id":                   id.String(),
			"name":                 name,
			"description":          description,
			"workspace_ref":        ws,
			"visibility":           visibility,
			"integration_mode":     mode,
			"document_count":       documentCount,
			"ready_document_count": readyDocumentCount,
		}
		if examScopeID != nil {
			item["scope_id"] = *examScopeID
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return errResult(errorSearchFailed, "list knowledge bases: "+err.Error())
	}
	return tools.ExecutionResult{ResultJSON: map[string]any{"items": items, "ready_only": readyOnly}}
}

func (e *Executor) executeListKnowledgePoints(ctx context.Context, args map[string]any, execCtx tools.ExecutionContext) tools.ExecutionResult {
	kb, ok, err := e.authorizedKB(ctx, args, execCtx)
	if err != nil {
		return errResult(errorSearchFailed, err.Error())
	}
	if !ok {
		return errResult(errorPermissionDenied, "caller is not a member of this KB workspace")
	}
	if kb.IntegrationMode == "exam" {
		return providerUnavailable("课程范围的知识点暂时不可用，请稍后重试或联系管理员检查保存目标。")
	}
	rows, err := e.pool.Query(ctx, `
SELECT id, name, parent_id, sort_order
FROM   kb_knowledge_points
WHERE  kb_id = $1
ORDER  BY sort_order ASC, created_at ASC`, kb.ID)
	if err != nil {
		return errResult(errorSearchFailed, "list knowledge points: "+err.Error())
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name string
		var parent *uuid.UUID
		var sortOrder int
		if err := rows.Scan(&id, &name, &parent, &sortOrder); err != nil {
			return errResult(errorSearchFailed, "scan knowledge point: "+err.Error())
		}
		item := map[string]any{"id": id.String(), "name": name, "sort_order": sortOrder}
		if parent != nil {
			item["parent_id"] = parent.String()
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return errResult(errorSearchFailed, "list knowledge points: "+err.Error())
	}
	return tools.ExecutionResult{ResultJSON: map[string]any{"items": items}}
}

func (e *Executor) executeDraftQuestions(ctx context.Context, args map[string]any, execCtx tools.ExecutionContext) tools.ExecutionResult {
	kb, ok, err := e.authorizedKB(ctx, args, execCtx)
	if err != nil {
		return errResult(errorSearchFailed, err.Error())
	}
	if !ok {
		return errResult(errorPermissionDenied, "caller is not a member of this KB workspace")
	}
	if kb.IntegrationMode == "exam" {
		return providerUnavailable("当前课程资料绑定的保存目标暂时不可用，暂不能基于该范围生成题目草稿。")
	}
	kpID, err := uuid.Parse(strings.TrimSpace(asString(args["knowledge_point_id"])))
	if err != nil || kpID == uuid.Nil {
		return errResult(errorArgsInvalid, "knowledge_point_id is required and must be a UUID")
	}
	kpName, err := e.knowledgePointName(ctx, kb.ID, kpID)
	if err != nil {
		return errResult(errorSearchFailed, "load knowledge point: "+err.Error())
	}
	query := strings.TrimSpace(asString(args["retrieval_query"]))
	if query == "" {
		query = kpName
	}
	if query == "" {
		query = kpID.String()
	}
	count := intFromAny(args["count"], 5)
	if count <= 0 {
		count = 5
	}
	if count > 5 {
		count = 5
	}
	hits, err := e.searchHits(ctx, kb.ID, query, 8)
	if err != nil {
		return errResult(errorSearchFailed, "search course material: "+err.Error())
	}
	hitMaps := make([]map[string]any, 0, len(hits))
	for _, hit := range hits {
		hitMaps = append(hitMaps, hitToMap(hit))
	}
	bankID, err := e.ensurePaperBankKB(ctx, kb.AccountID, kb.WorkspaceRef, execCtx.UserID)
	if err != nil {
		return errResult(errorSearchFailed, "ensure paper bank: "+err.Error())
	}
	refs, err := e.listReferenceQuestions(ctx, bankID, kpID, 5)
	if err != nil {
		return errResult(errorSearchFailed, "list reference questions: "+err.Error())
	}
	qType := strings.TrimSpace(asString(args["type"]))
	difficulty := strings.TrimSpace(asString(args["difficulty"]))
	return tools.ExecutionResult{ResultJSON: map[string]any{
		"action":               "draft_questions",
		"kb_id":                kb.ID.String(),
		"knowledge_point_id":   kpID.String(),
		"knowledge_point_name": kpName,
		"count":                count,
		"type":                 qType,
		"difficulty":           difficulty,
		"retrieval_query":      query,
		"retrieval_hits":       hitMaps,
		"reference_questions":  refs,
		"ui_panel":             questionDraftPanel(kpName, count, qType, difficulty, len(hitMaps), len(refs)),
		"instruction":          "基于 retrieval_hits 中的课程资料和 reference_questions 中的命题风格生成题目草稿。不要保存。每道题必须包含 type、difficulty、stem、options、answer、explanation、source_snippets；选择题至少 3 个选项；source_snippets 应引用 retrieval_hits 的 id/document_ref/ordinal 和 200-500 字依据。",
	}}
}

func (e *Executor) executeSaveQuestions(ctx context.Context, args map[string]any, execCtx tools.ExecutionContext) tools.ExecutionResult {
	if confirmed, _ := args["confirmed"].(bool); !confirmed {
		return errResult(errorConfirmation, "kb_save_questions requires confirmed=true after teacher confirmation")
	}
	kb, ok, err := e.authorizedKB(ctx, args, execCtx)
	if err != nil {
		return errResult(errorSearchFailed, err.Error())
	}
	if !ok {
		return errResult(errorPermissionDenied, "caller is not a member of this KB workspace")
	}
	if kb.IntegrationMode == "exam" {
		return providerUnavailable("当前课程资料绑定的保存目标暂时不可用，题目草稿还没有保存。请稍后重试。")
	}
	questionsRaw, ok := args["questions"].([]any)
	if !ok || len(questionsRaw) == 0 {
		return errResult(errorArgsInvalid, "questions array is required")
	}
	bankID, err := e.ensurePaperBankKB(ctx, kb.AccountID, kb.WorkspaceRef, execCtx.UserID)
	if err != nil {
		return errResult(errorSearchFailed, "ensure paper bank: "+err.Error())
	}
	created := []map[string]any{}
	failed := []map[string]any{}
	for i, raw := range questionsRaw {
		q, ok := raw.(map[string]any)
		if !ok {
			failed = append(failed, failureMap(i, "validation_error", "question must be an object"))
			continue
		}
		id, code, msg, err := e.insertQuestion(ctx, bankID, q, execCtx.UserID)
		if err != nil {
			failed = append(failed, failureMap(i, code, msg))
			continue
		}
		created = append(created, map[string]any{"index": i, "id": id.String()})
	}
	return tools.ExecutionResult{ResultJSON: map[string]any{
		"question_bank": "组卷题库",
		"created":       created,
		"failed":        failed,
		"created_count": len(created),
		"failed_count":  len(failed),
	}}
}

func (e *Executor) executeComposePaper(ctx context.Context, args map[string]any, execCtx tools.ExecutionContext) tools.ExecutionResult {
	kb, ok, err := e.authorizedKB(ctx, args, execCtx)
	if err != nil {
		return errResult(errorSearchFailed, err.Error())
	}
	if !ok {
		return errResult(errorPermissionDenied, "caller is not a member of this KB workspace")
	}
	if kb.IntegrationMode == "exam" {
		return providerUnavailable("当前课程资料绑定的保存目标暂时不可用，暂不能保存试卷。请稍后重试。")
	}
	name := strings.TrimSpace(asString(args["name"]))
	if name == "" {
		return errResult(errorArgsInvalid, "name is required")
	}
	kpIDs, err := parseUUIDSlice(args["knowledge_point_ids"])
	if err != nil || len(kpIDs) == 0 {
		return errResult(errorArgsInvalid, "knowledge_point_ids must be a non-empty array of UUID strings")
	}
	totalCount := intFromAny(args["total_count"], 0)
	if totalCount <= 0 {
		return errResult(errorArgsInvalid, "total_count must be positive")
	}
	bankID, err := e.ensurePaperBankKB(ctx, kb.AccountID, kb.WorkspaceRef, execCtx.UserID)
	if err != nil {
		return errResult(errorSearchFailed, "ensure paper bank: "+err.Error())
	}
	pool, err := e.listPaperPool(ctx, bankID, kpIDs)
	if err != nil {
		return errResult(errorSearchFailed, "list paper pool: "+err.Error())
	}
	typeDist := mapStringInt(args["type_distribution"])
	seed := int64(intFromAny(args["seed"], 0))
	selected, warnings := selectQuestions(pool, totalCount, typeDist, seed)
	if len(warnings) > 0 {
		return tools.ExecutionResult{ResultJSON: map[string]any{
			"shortage_warnings": warnings,
			"pool_size":         len(pool),
			"question_bank":     "组卷题库",
			"ui_panel":          shortagePanel(warnings),
		}}
	}
	ids := make([]string, 0, len(selected))
	for _, q := range selected {
		ids = append(ids, q.ID.String())
	}
	markdown := renderPaperMarkdown(name, selected)
	confirmed, _ := args["confirmed"].(bool)
	out := map[string]any{
		"question_bank":  "组卷题库",
		"name":           name,
		"question_ids":   ids,
		"question_count": len(ids),
		"markdown":       markdown,
		"saved":          false,
		"ui_panel":       paperPreviewPanel(name, selected, markdown, confirmed),
	}
	if !confirmed {
		out["message"] = "已生成试卷预览，老师确认后再次调用并传 confirmed=true 保存。"
		return tools.ExecutionResult{ResultJSON: out}
	}
	spec := map[string]any{
		"total_count":       totalCount,
		"type_distribution": typeDist,
		"seed":              seed,
	}
	specJSON := jsonBytes(spec, "{}")
	idsJSON := jsonBytes(ids, "[]")
	var paperID uuid.UUID
	err = e.pool.QueryRow(ctx, `
INSERT INTO kb_papers (kb_id, name, spec_json, seed, question_ids_json, markdown, created_by)
VALUES ($1, $2, $3::jsonb, $4, $5::jsonb, $6, $7)
RETURNING id`, bankID, name, specJSON, seed, idsJSON, markdown, nullableUUID(execCtx.UserID)).Scan(&paperID)
	if err != nil {
		return errResult(errorSearchFailed, "save paper: "+err.Error())
	}
	out["saved"] = true
	out["paper_id"] = paperID.String()
	return tools.ExecutionResult{ResultJSON: out}
}

func (e *Executor) authorizedKB(ctx context.Context, args map[string]any, execCtx tools.ExecutionContext) (kbDescriptor, bool, error) {
	if e.IsNotConfigured() || e.pool == nil {
		return kbDescriptor{}, false, errors.New("kb tools are not configured")
	}
	accountID := uuid.Nil
	if execCtx.AccountID != nil {
		accountID = *execCtx.AccountID
	}
	if accountID == uuid.Nil {
		return kbDescriptor{}, false, errors.New("kb tools require an account context")
	}
	kbID, err := uuid.Parse(strings.TrimSpace(asString(args["kb_id"])))
	if err != nil || kbID == uuid.Nil {
		return kbDescriptor{}, false, errors.New("kb_id is required and must be a UUID")
	}
	ok, err := e.access.IsActorWorkspaceMember(ctx, accountID, kbID)
	if err != nil || !ok {
		return kbDescriptor{}, ok, err
	}
	kb, err := e.loadKBDescriptor(ctx, accountID, kbID)
	return kb, true, err
}

func (e *Executor) loadKBDescriptor(ctx context.Context, accountID, kbID uuid.UUID) (kbDescriptor, error) {
	var kb kbDescriptor
	err := e.pool.QueryRow(ctx, `
SELECT id, account_id, workspace_ref, integration_mode, exam_scope_id
FROM   knowledge_bases
WHERE  id = $1 AND account_id = $2`, kbID, accountID).Scan(&kb.ID, &kb.AccountID, &kb.WorkspaceRef, &kb.IntegrationMode, &kb.ExamScopeID)
	if err != nil {
		return kbDescriptor{}, err
	}
	return kb, nil
}

func (e *Executor) ensurePaperBankKB(ctx context.Context, accountID uuid.UUID, workspaceRef string, userID *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := e.pool.QueryRow(ctx, `
SELECT id
FROM   knowledge_bases
WHERE  account_id = $1 AND workspace_ref = $2 AND name = $3
LIMIT  1`, accountID, workspaceRef, paperBankName).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	err = e.pool.QueryRow(ctx, `
INSERT INTO knowledge_bases (workspace_ref, account_id, name, description, visibility, integration_mode, created_by)
VALUES ($1, $2, $3, $4, 'workspace_member', 'standalone', $5)
ON CONFLICT (workspace_ref, name) DO UPDATE SET updated_at = knowledge_bases.updated_at
RETURNING id`, workspaceRef, accountID, paperBankName, "ArkLoop 智能组卷生成题目的固定题库", nullableUUID(userID)).Scan(&id)
	return id, err
}

func (e *Executor) knowledgePointName(ctx context.Context, kbID, kpID uuid.UUID) (string, error) {
	var name string
	err := e.pool.QueryRow(ctx, `SELECT name FROM kb_knowledge_points WHERE id = $1 AND kb_id = $2`, kpID, kbID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}

func (e *Executor) listReferenceQuestions(ctx context.Context, bankID, kpID uuid.UUID, limit int) ([]map[string]any, error) {
	rows, err := e.pool.Query(ctx, `
SELECT id, question_type, difficulty, stem, options_json, answer, explanation, source_snippets_json, created_at
FROM   kb_questions
WHERE  kb_id = $1 AND knowledge_point_id = $2
ORDER  BY created_at DESC, id ASC
LIMIT  $3`, bankID, kpID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var q questionRow
		if err := rows.Scan(&q.ID, &q.Type, &q.Difficulty, &q.Stem, &q.OptionsJSON, &q.Answer, &q.Explanation, &q.SourceJSON, &q.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, q.toMap())
	}
	return items, rows.Err()
}

func (e *Executor) insertQuestion(ctx context.Context, bankID uuid.UUID, q map[string]any, userID *uuid.UUID) (uuid.UUID, string, string, error) {
	stem := strings.TrimSpace(asString(q["stem"]))
	qType := strings.TrimSpace(asString(q["type"]))
	difficulty := strings.TrimSpace(asString(q["difficulty"]))
	answer := strings.TrimSpace(asString(q["answer"]))
	if stem == "" || qType == "" || difficulty == "" || answer == "" {
		return uuid.Nil, "validation_error", "stem/type/difficulty/answer are required", errors.New("invalid question")
	}
	kpID, err := uuid.Parse(strings.TrimSpace(asString(q["knowledge_point_id"])))
	if err != nil || kpID == uuid.Nil {
		return uuid.Nil, "knowledge_point_id_invalid", "knowledge_point_id must be a UUID", errors.New("invalid knowledge point")
	}
	explanation := strings.TrimSpace(asString(q["explanation"]))
	optionsJSON := jsonBytes(q["options"], "[]")
	sourceJSON := jsonBytes(q["source_snippets"], "[]")
	chunkIDsJSON := jsonBytes(q["source_chunk_ids"], "[]")
	var id uuid.UUID
	err = e.pool.QueryRow(ctx, `
INSERT INTO kb_questions (
    kb_id, knowledge_point_id, question_type, difficulty, stem,
    options_json, answer, explanation, source_chunk_ids_json, source_snippets_json,
    quality_flag, created_by)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9::jsonb, $10::jsonb, 'accepted', $11)
RETURNING id`, bankID, kpID, qType, difficulty, stem, optionsJSON, answer, explanation, chunkIDsJSON, sourceJSON, nullableUUID(userID)).Scan(&id)
	if err != nil {
		return uuid.Nil, "internal_error", "internal error while saving question", err
	}
	return id, "", "", nil
}

func (e *Executor) listPaperPool(ctx context.Context, bankID uuid.UUID, kpIDs []uuid.UUID) ([]questionRow, error) {
	rows, err := e.pool.Query(ctx, `
SELECT id, knowledge_point_id, question_type, difficulty, stem, options_json, answer, explanation, source_snippets_json, created_at
FROM   kb_questions
WHERE  kb_id = $1 AND knowledge_point_id = ANY($2)
ORDER  BY created_at DESC, id ASC`, bankID, kpIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []questionRow
	for rows.Next() {
		var q questionRow
		if err := rows.Scan(&q.ID, &q.KnowledgePointID, &q.Type, &q.Difficulty, &q.Stem, &q.OptionsJSON, &q.Answer, &q.Explanation, &q.SourceJSON, &q.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

func selectQuestions(pool []questionRow, total int, typeDist map[string]int, seed int64) ([]questionRow, []map[string]any) {
	sort.Slice(pool, func(i, j int) bool { return pool[i].ID.String() < pool[j].ID.String() })
	if seed != 0 {
		r := rand.New(rand.NewSource(seed))
		r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	}
	var selected []questionRow
	var warnings []map[string]any
	if len(typeDist) > 0 {
		for typ, n := range typeDist {
			var bucket []questionRow
			for _, q := range pool {
				if q.Type == typ {
					bucket = append(bucket, q)
				}
			}
			if len(bucket) < n {
				warnings = append(warnings, map[string]any{"type": typ, "available": len(bucket), "requested": n, "message": "题型题量不足"})
				continue
			}
			selected = append(selected, bucket[:n]...)
		}
		return selected, warnings
	}
	if len(pool) < total {
		return nil, []map[string]any{{"available": len(pool), "requested": total, "message": "题池题量不足"}}
	}
	return pool[:total], nil
}

func renderPaperMarkdown(name string, questions []questionRow) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(name)
	b.WriteString("\n\n")
	for i, q := range questions {
		fmt.Fprintf(&b, "%d. **%s**（%s / %s）\n\n", i+1, q.Stem, q.Type, q.Difficulty)
		var opts []map[string]any
		if err := json.Unmarshal(q.OptionsJSON, &opts); err == nil {
			for _, opt := range opts {
				fmt.Fprintf(&b, "   %v. %v\n", opt["key"], opt["text"])
			}
			if len(opts) > 0 {
				b.WriteString("\n")
			}
		}
		fmt.Fprintf(&b, "   **答案：** %s\n\n", q.Answer)
		if strings.TrimSpace(q.Explanation) != "" {
			fmt.Fprintf(&b, "   **解析：** %s\n\n", q.Explanation)
		}
	}
	return b.String()
}

func questionDraftPanel(kpName string, count int, qType, difficulty string, hitCount, referenceCount int) map[string]any {
	if strings.TrimSpace(qType) == "" {
		qType = "未指定"
	}
	if strings.TrimSpace(difficulty) == "" {
		difficulty = "未指定"
	}
	title := "出题要求确认"
	code := `<style>
.paper-panel{font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#18212f;border:1px solid #d8dee8;border-radius:8px;padding:14px;background:#fff;max-width:720px}
.paper-panel h3{font-size:16px;margin:0 0 10px}.paper-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px}
.paper-cell{border:1px solid #edf0f5;border-radius:6px;padding:8px;background:#f8fafc}.paper-label{font-size:12px;color:#667085}.paper-value{font-size:14px;margin-top:2px}
.paper-actions{display:flex;gap:8px;margin-top:12px;flex-wrap:wrap}.paper-actions button{border:0;border-radius:6px;padding:8px 10px;background:#1f6feb;color:white;cursor:pointer}.paper-actions button.secondary{background:#eef2f6;color:#18212f}
</style><div class="paper-panel"><h3>出题要求确认</h3><div class="paper-grid">` +
		panelCell("知识点", kpName) +
		panelCell("题目数量", fmt.Sprintf("%d 道", count)) +
		panelCell("题型", qType) +
		panelCell("难度", difficulty) +
		panelCell("课程资料命中", fmt.Sprintf("%d 条", hitCount)) +
		panelCell("参考题", fmt.Sprintf("%d 道", referenceCount)) +
		`</div><div class="paper-actions"><button onclick="sendPrompt('按这个要求生成题目草稿')">生成题目草稿</button><button class="secondary" onclick="sendPrompt('我想调整出题要求')">调整要求</button></div></div>`
	return map[string]any{
		"kind":                "question_draft_form",
		"title":               title,
		"widget_code":         code,
		"confirmation_prompt": "按这个要求生成题目草稿",
	}
}

func shortagePanel(warnings []map[string]any) map[string]any {
	var rows strings.Builder
	for _, w := range warnings {
		label := "题池"
		if typ, ok := w["type"].(string); ok && typ != "" {
			label = typ
		}
		rows.WriteString(`<div class="shortage-row"><strong>`)
		rows.WriteString(html.EscapeString(label))
		rows.WriteString(`</strong><span>需要 `)
		rows.WriteString(html.EscapeString(fmt.Sprint(w["requested"])))
		rows.WriteString(`，可用 `)
		rows.WriteString(html.EscapeString(fmt.Sprint(w["available"])))
		rows.WriteString(`</span></div>`)
	}
	code := `<style>
.shortage-panel{font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;border:1px solid #f1c27d;border-radius:8px;background:#fff8eb;color:#2a1d0c;padding:14px;max-width:720px}
.shortage-panel h3{margin:0 0 10px;font-size:16px}.shortage-row{display:flex;justify-content:space-between;border-top:1px solid #f3dfbf;padding:8px 0;gap:12px}
.shortage-actions{display:flex;gap:8px;margin-top:10px;flex-wrap:wrap}.shortage-actions button{border:0;border-radius:6px;padding:8px 10px;background:#b45309;color:white;cursor:pointer}.shortage-actions button.secondary{background:#f3e7d2;color:#2a1d0c}
</style><div class="shortage-panel"><h3>题量不足</h3>` + rows.String() + `<div class="shortage-actions"><button onclick="sendPrompt('先补题再组卷')">先补题</button><button class="secondary" onclick="sendPrompt('我想放宽组卷条件')">放宽条件</button><button class="secondary" onclick="sendPrompt('缩小组卷范围')">缩小范围</button></div></div>`
	return map[string]any{
		"kind":        "paper_shortage",
		"title":       "题量不足",
		"widget_code": code,
	}
}

func paperPreviewPanel(name string, questions []questionRow, markdown string, confirmed bool) map[string]any {
	var rows strings.Builder
	limit := len(questions)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		q := questions[i]
		rows.WriteString(`<li><span>`)
		rows.WriteString(html.EscapeString(fmt.Sprintf("%d. %s", i+1, q.Stem)))
		rows.WriteString(`</span><em>`)
		rows.WriteString(html.EscapeString(q.Type + " / " + q.Difficulty))
		rows.WriteString(`</em></li>`)
	}
	if len(questions) > limit {
		rows.WriteString(`<li><span>... 还有 `)
		rows.WriteString(html.EscapeString(fmt.Sprint(len(questions) - limit)))
		rows.WriteString(` 道</span></li>`)
	}
	action := `<button onclick="sendPrompt('确认保存这份试卷')">确认保存试卷</button><button class="secondary" onclick="sendPrompt('我想调整试卷')">调整试卷</button>`
	if confirmed {
		action = `<button onclick="sendPrompt('继续组卷')">继续组卷</button>`
	}
	code := `<style>
.preview-panel{font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#14202f;border:1px solid #d8dee8;border-radius:8px;background:#fff;padding:14px;max-width:760px}
.preview-panel h3{margin:0 0 8px;font-size:16px}.preview-panel ul{list-style:none;padding:0;margin:0;display:grid;gap:6px}.preview-panel li{border:1px solid #edf0f5;border-radius:6px;padding:8px;background:#f8fafc;display:grid;gap:3px}
.preview-panel em{font-size:12px;color:#667085;font-style:normal}.preview-actions{display:flex;gap:8px;margin-top:12px;flex-wrap:wrap}.preview-actions button{border:0;border-radius:6px;padding:8px 10px;background:#1f6feb;color:#fff;cursor:pointer}.preview-actions button.secondary{background:#eef2f6;color:#14202f}
</style><div class="preview-panel"><h3>` + html.EscapeString(name) + `</h3><ul>` + rows.String() + `</ul><div class="preview-actions">` + action + `</div></div>`
	return map[string]any{
		"kind":          "paper_preview",
		"title":         "试卷预览",
		"widget_code":   code,
		"markdown_size": len(markdown),
	}
}

func panelCell(label, value string) string {
	return `<div class="paper-cell"><div class="paper-label">` + html.EscapeString(label) + `</div><div class="paper-value">` + html.EscapeString(value) + `</div></div>`
}

func (q questionRow) toMap() map[string]any {
	item := map[string]any{
		"id":          q.ID.String(),
		"type":        q.Type,
		"difficulty":  q.Difficulty,
		"stem":        q.Stem,
		"answer":      q.Answer,
		"explanation": q.Explanation,
		"created_at":  q.CreatedAt,
	}
	if q.KnowledgePointID != nil {
		item["knowledge_point_id"] = q.KnowledgePointID.String()
	}
	var opts any
	if err := json.Unmarshal(q.OptionsJSON, &opts); err == nil {
		item["options"] = opts
	}
	var snippets any
	if err := json.Unmarshal(q.SourceJSON, &snippets); err == nil {
		item["source_snippets"] = snippets
	}
	return item
}

func nullableUUID(id *uuid.UUID) any {
	if id == nil || *id == uuid.Nil {
		return nil
	}
	return *id
}

func failureMap(index int, code, message string) map[string]any {
	return map[string]any{"index": index, "error_code": code, "error_message": message}
}

func providerUnavailable(message string) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: "kb.provider_unavailable",
			Message:    message,
		},
	}
}

func parseUUIDSlice(v any) ([]uuid.UUID, error) {
	raw := parseStringSlice(v)
	out := make([]uuid.UUID, 0, len(raw))
	for _, item := range raw {
		id, err := uuid.Parse(strings.TrimSpace(item))
		if err != nil || id == uuid.Nil {
			return nil, errors.New("invalid UUID")
		}
		out = append(out, id)
	}
	return out, nil
}

func mapStringInt(v any) map[string]int {
	out := map[string]int{}
	switch m := v.(type) {
	case map[string]any:
		for k, raw := range m {
			out[k] = intFromAny(raw, 0)
		}
	case map[string]int:
		for k, raw := range m {
			out[k] = raw
		}
	}
	for k, n := range out {
		if n <= 0 {
			delete(out, k)
		}
	}
	return out
}
