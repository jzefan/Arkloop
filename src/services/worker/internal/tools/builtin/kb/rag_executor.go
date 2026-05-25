package kb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math/rand"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	paperBankName                = "组卷题库"
	fallbackExamQuestionBankName = "国考医学"
)

var chapterHeadingRE = regexp.MustCompile(`^第\s*[零〇一二三四五六七八九十百千万0-9]+\s*章`)

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

type knowledgePointItem struct {
	ID        uuid.UUID
	Name      string
	ParentID  *uuid.UUID
	SortOrder int
}

type headingCandidate struct {
	Text    string
	Ordinal int
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
	workspaceRef := listKnowledgeBasesWorkspaceRef(args)
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
		query += "\n  AND kb.kb_kind = 'user'"
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

func listKnowledgeBasesWorkspaceRef(args map[string]any) string {
	return strings.TrimSpace(asString(args["workspace_ref"]))
}

func (e *Executor) listLocalKnowledgePointItems(ctx context.Context, kbID uuid.UUID) ([]knowledgePointItem, error) {
	rows, err := e.pool.Query(ctx, `
SELECT id, name, parent_id, sort_order
FROM   kb_knowledge_points
WHERE  kb_id = $1
ORDER  BY sort_order ASC, created_at ASC`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []knowledgePointItem{}
	for rows.Next() {
		var item knowledgePointItem
		if err := rows.Scan(&item.ID, &item.Name, &item.ParentID, &item.SortOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (e *Executor) ensureFallbackKnowledgePointsFromHeadings(ctx context.Context, kbID uuid.UUID) (int, error) {
	rows, err := e.pool.Query(ctx, `
SELECT btrim(text), ordinal
FROM   kb_chunks
WHERE  kb_id = $1
  AND  chunk_type = 'heading'
  AND  btrim(text) <> ''
ORDER  BY ordinal ASC`, kbID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	headings := []headingCandidate{}
	for rows.Next() {
		var h headingCandidate
		if err := rows.Scan(&h.Text, &h.Ordinal); err != nil {
			return 0, err
		}
		headings = append(headings, h)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	names := deriveChapterKnowledgePointNames(headings)
	for i, name := range names {
		if _, err := e.pool.Exec(ctx, `
INSERT INTO kb_knowledge_points (kb_id, name, sort_order)
VALUES ($1, $2, $3)`, kbID, name, i+1); err != nil {
			return i, err
		}
	}
	return len(names), nil
}

func deriveChapterKnowledgePointNames(headings []headingCandidate) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for i, h := range headings {
		chapter := normalizedChapterHeading(h.Text)
		if chapter == "" {
			continue
		}
		name := chapter
		if i+1 < len(headings) {
			title := normalizedChapterTitle(headings[i+1].Text)
			if title != "" {
				name = chapter + " " + title
			}
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func normalizedChapterHeading(text string) string {
	cleaned := strings.Join(strings.Fields(strings.TrimSpace(text)), "")
	if !chapterHeadingRE.MatchString(cleaned) {
		return ""
	}
	return cleaned
}

func normalizedChapterTitle(text string) string {
	cleaned := strings.Join(strings.Fields(strings.TrimSpace(text)), "")
	if cleaned == "" || chapterHeadingRE.MatchString(cleaned) {
		return ""
	}
	runes := []rune(cleaned)
	if len(runes) < 2 || len(runes) > 80 {
		return ""
	}
	if _, err := strconv.Atoi(cleaned); err == nil {
		return ""
	}
	return cleaned
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
		return e.executeProviderListKnowledgePoints(ctx, kb, execCtx)
	}
	items, err := e.listLocalKnowledgePointItems(ctx, kb.ID)
	if err != nil {
		return errResult(errorSearchFailed, "list knowledge points: "+err.Error())
	}
	if len(items) == 0 {
		inserted, err := e.ensureFallbackKnowledgePointsFromHeadings(ctx, kb.ID)
		if err != nil {
			return errResult(errorSearchFailed, "derive knowledge points: "+err.Error())
		}
		if inserted > 0 {
			items, err = e.listLocalKnowledgePointItems(ctx, kb.ID)
			if err != nil {
				return errResult(errorSearchFailed, "list knowledge points: "+err.Error())
			}
		}
	}
	out := []map[string]any{}
	for _, kp := range items {
		item := map[string]any{"id": kp.ID.String(), "name": kp.Name, "sort_order": kp.SortOrder}
		if kp.ParentID != nil {
			item["parent_id"] = kp.ParentID.String()
		}
		out = append(out, item)
	}
	return tools.ExecutionResult{ResultJSON: map[string]any{"items": out}}
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
		return e.executeProviderDraftQuestions(ctx, kb, args, execCtx)
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
	// All saves go to the user's account-level system paper bank, regardless
	// of the source KB's integration mode. Linked KBs use exam only as a
	// reference data source (admin-read); the bank that holds the teacher's
	// confirmed questions is local. See PRD §组卷题库.
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
		if code, msg, err := e.prepareQuestionSources(ctx, kb.ID, q); err != nil {
			failed = append(failed, failureMap(i, code, msg))
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
	// All composes pull from the local 组卷题库 (account-level system bank)
	// and save the resulting paper there. Linked KBs do not write to exam.
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
	difficultyDist := mapStringInt(args["difficulty_distribution"])
	kpDist := mapStringInt(args["knowledge_point_distribution"])
	seed := int64(intFromAny(args["seed"], 0))
	selected, warnings := selectQuestions(pool, totalCount, typeDist, difficultyDist, kpDist, seed)
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
		"total_count":                  totalCount,
		"type_distribution":            typeDist,
		"difficulty_distribution":      difficultyDist,
		"knowledge_point_distribution": kpDist,
		"seed":                         seed,
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

func (e *Executor) executeProviderListKnowledgePoints(ctx context.Context, kb kbDescriptor, execCtx tools.ExecutionContext) tools.ExecutionResult {
	if e == nil || e.provider == nil || kb.ExamScopeID == nil || strings.TrimSpace(*kb.ExamScopeID) == "" {
		return providerUnavailable("当前课程资料绑定的数据暂时不可用，暂时无法读取知识点。请稍后重试。")
	}
	scopeID := strings.TrimSpace(*kb.ExamScopeID)
	query := url.Values{}
	query.Set("exam_scope_id", scopeID)
	query.Set("limit", "500")
	query.Set("offset", "0")
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	// Use the admin token: linked-mode KBs are bound to scopes inside the
	// platform-administrator question bank (e.g. 国考医学题库) which an
	// ordinary teacher account cannot see.
	if err := e.provider.CallExamAsAdmin(ctx, "GET", "/api/knowledge-points?"+query.Encode(), nil, &resp); err != nil {
		return providerUnavailable("当前课程资料绑定的数据暂时不可用，暂时无法读取知识点。请稍后重试。")
	}
	return tools.ExecutionResult{ResultJSON: map[string]any{"items": resp.Items, "total": resp.Total}}
}

func (e *Executor) executeProviderDraftQuestions(ctx context.Context, kb kbDescriptor, args map[string]any, execCtx tools.ExecutionContext) tools.ExecutionResult {
	if e == nil || e.provider == nil {
		return providerUnavailable("当前课程资料绑定的题库暂时不可用，暂时无法获取参考题。请稍后重试。")
	}
	kpID := strings.TrimSpace(asString(args["knowledge_point_id"]))
	if kpID == "" {
		return errResult(errorArgsInvalid, "knowledge_point_id is required")
	}
	query := strings.TrimSpace(asString(args["retrieval_query"]))
	if query == "" {
		query = kpID
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
	qType := strings.TrimSpace(asString(args["type"]))
	difficulty := strings.TrimSpace(asString(args["difficulty"]))
	// Reference questions come from the platform-administrator question
	// bank (e.g. 国考医学) via admin credentials; ordinary teacher
	// accounts cannot see this bank otherwise.
	bankIDs, err := e.providerQuestionBankIDs(ctx)
	if err != nil {
		return providerUnavailable("当前课程资料绑定的题库暂时不可用，暂时无法获取参考题。请稍后重试。")
	}
	refs, usedBankName, err := e.listProviderReferenceQuestionsWithFallback(ctx, kpID, qType, difficulty, bankIDs, 5)
	if err != nil {
		return providerUnavailable("当前课程资料绑定的题库暂时不可用，暂时无法获取参考题。请稍后重试。")
	}
	kpName := providerQuestionLabel(kpID, refs.Items)
	return tools.ExecutionResult{ResultJSON: map[string]any{
		"action":               "draft_questions",
		"kb_id":                kb.ID.String(),
		"knowledge_point_id":   kpID,
		"knowledge_point_name": kpName,
		"count":                count,
		"type":                 qType,
		"difficulty":           difficulty,
		"question_bank":        usedBankName,
		"retrieval_query":      query,
		"retrieval_hits":       hitMaps,
		"reference_questions":  refs.Items,
		"ui_panel":             questionDraftPanel(kpName, count, qType, difficulty, len(hitMaps), len(refs.Items)),
		"instruction":          "基于 retrieval_hits 中的课程资料和 reference_questions 中的命题风格生成题目草稿。不要保存。每道题必须包含 knowledge_point_id、type、difficulty、stem、options、answer、explanation、source_snippets；选择题至少 3 个选项；source_snippets 应引用 retrieval_hits 的 id/document_ref/ordinal 和 200-500 字依据。",
	}}
}

type providerQuestionListResp struct {
	Items []map[string]any `json:"items"`
	Total int              `json:"total"`
}

// providerQuestionBankIDs reads the exam question-bank list with the
// admin token and returns a name→id map. Linked-mode read paths use this
// to discover the 国考医学 reference bank id (which ordinary teacher
// accounts cannot see).
func (e *Executor) providerQuestionBankIDs(ctx context.Context) (map[string]string, error) {
	var banks []map[string]any
	if err := e.provider.CallExamAsAdmin(ctx, "GET", "/api/question-banks", nil, &banks); err != nil {
		return nil, err
	}
	ids := map[string]string{}
	for _, bank := range banks {
		name := strings.TrimSpace(asString(bank["name"]))
		id := strings.TrimSpace(asString(bank["id"]))
		if name != "" && id != "" {
			ids[name] = id
		}
	}
	return ids, nil
}

// listProviderReferenceQuestionsWithFallback fetches reference questions
// for a knowledge point using admin credentials. After the admin-token
// migration there is no per-account exam bank, so this only ever queries
// the platform-administrator reference bank (e.g. 国考医学) — the loop
// is kept generic to allow alternative reference banks in future
// deployments without changing call sites.
func (e *Executor) listProviderReferenceQuestionsWithFallback(
	ctx context.Context,
	kpID string,
	qType string,
	difficulty string,
	bankIDs map[string]string,
	limit int,
) (providerQuestionListResp, string, error) {
	if fallbackBankID := bankIDs[fallbackExamQuestionBankName]; fallbackBankID != "" {
		refs, err := e.listProviderQuestions(ctx, kpID, qType, difficulty, fallbackBankID, limit)
		return refs, fallbackExamQuestionBankName, err
	}
	return providerQuestionListResp{Items: []map[string]any{}}, "", nil
}

func (e *Executor) listProviderQuestions(
	ctx context.Context,
	kpID string,
	qType string,
	difficulty string,
	questionBankID string,
	limit int,
) (providerQuestionListResp, error) {
	query := url.Values{}
	query.Set("knowledge_point_id", kpID)
	if questionBankID != "" {
		query.Set("question_bank_id", questionBankID)
	}
	if qType != "" {
		query.Set("type", qType)
	}
	if difficulty != "" {
		query.Set("difficulty", difficulty)
	}
	if limit <= 0 {
		limit = 20
	}
	query.Set("limit", strconv.Itoa(limit))
	query.Set("offset", "0")
	var resp providerQuestionListResp
	err := e.provider.CallExamAsAdmin(ctx, "GET", "/api/questions?"+query.Encode(), nil, &resp)
	return resp, err
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

// ensurePaperBankKB returns the (account-level) system paper bank for this
// account, creating it on first use. Lookup ignores workspace_ref so that
// teachers across multiple workspaces in the same Account share one bank
// (PRD §组卷题库 "按 Account 维度存在"). The bank's workspace_ref column is
// still set to the calling workspace for traceability, but it does not
// affect the lookup key.
func (e *Executor) ensurePaperBankKB(ctx context.Context, accountID uuid.UUID, workspaceRef string, userID *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := e.pool.QueryRow(ctx, `
SELECT id
FROM   knowledge_bases
WHERE  account_id = $1 AND kb_kind = 'system_paper_bank'
LIMIT  1`, accountID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	// First-use insert. The partial unique index
	// knowledge_bases_one_system_bank_per_account makes this race-safe:
	// concurrent workers attempting to create the bank will land on the same
	// row via ON CONFLICT and re-read the existing id.
	err = e.pool.QueryRow(ctx, `
INSERT INTO knowledge_bases (workspace_ref, account_id, name, description, visibility, integration_mode, kb_kind, created_by)
VALUES ($1, $2, $3, $4, 'workspace_member', 'standalone', 'system_paper_bank', $5)
ON CONFLICT (account_id) WHERE kb_kind = 'system_paper_bank'
DO UPDATE SET updated_at = knowledge_bases.updated_at
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

func selectQuestions(pool []questionRow, total int, typeDist, difficultyDist, kpDist map[string]int, seed int64) ([]questionRow, []map[string]any) {
	sort.Slice(pool, func(i, j int) bool { return pool[i].ID.String() < pool[j].ID.String() })
	if seed != 0 {
		r := rand.New(rand.NewSource(seed))
		r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	}
	warnings := validateDistributions(total, distributionSet{
		{Key: "type", Message: "题型题量不足", Requested: typeDist, Available: countQuestionRows(pool, func(q questionRow) string { return q.Type })},
		{Key: "difficulty", Message: "难度题量不足", Requested: difficultyDist, Available: countQuestionRows(pool, func(q questionRow) string { return q.Difficulty })},
		{Key: "knowledge_point_id", Message: "知识点题量不足", Requested: kpDist, Available: countQuestionRows(pool, questionRowKnowledgePointID)},
	})
	if len(warnings) > 0 {
		return nil, warnings
	}
	if len(pool) < total {
		return nil, []map[string]any{{"available": len(pool), "requested": total, "message": "题池题量不足"}}
	}
	selected := greedySelectQuestionRows(pool, total, typeDist, difficultyDist, kpDist)
	if warnings := unmetDistributionWarnings(selected, typeDist, difficultyDist, kpDist); len(warnings) > 0 {
		return nil, warnings
	}
	return selected, nil
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

type distributionSpec struct {
	Key       string
	Message   string
	Requested map[string]int
	Available map[string]int
}

type distributionSet []distributionSpec

func validateDistributions(total int, specs distributionSet) []map[string]any {
	var warnings []map[string]any
	for _, spec := range specs {
		if sumPositive(spec.Requested) > total {
			warnings = append(warnings, map[string]any{
				spec.Key:    "all",
				"requested": sumPositive(spec.Requested),
				"available": total,
				"message":   "约束题量超过试卷总题数",
			})
			continue
		}
		for value, requested := range spec.Requested {
			if requested <= 0 {
				continue
			}
			available := spec.Available[value]
			if available < requested {
				warnings = append(warnings, map[string]any{
					spec.Key:    value,
					"available": available,
					"requested": requested,
					"message":   spec.Message,
				})
			}
		}
	}
	return warnings
}

func greedySelectQuestionRows(pool []questionRow, total int, typeDist, difficultyDist, kpDist map[string]int) []questionRow {
	remainingType := copyIntMap(typeDist)
	remainingDifficulty := copyIntMap(difficultyDist)
	remainingKP := copyIntMap(kpDist)
	selected := make([]questionRow, 0, total)
	used := map[string]bool{}
	for len(selected) < total {
		bestIndex := -1
		bestScore := -1
		for i, q := range pool {
			id := q.ID.String()
			if used[id] {
				continue
			}
			score := unmetScore(q.Type, remainingType) + unmetScore(q.Difficulty, remainingDifficulty) + unmetScore(questionRowKnowledgePointID(q), remainingKP)
			if score > bestScore {
				bestScore = score
				bestIndex = i
			}
		}
		if bestIndex < 0 {
			break
		}
		q := pool[bestIndex]
		selected = append(selected, q)
		used[q.ID.String()] = true
		decrementIfNeeded(remainingType, q.Type)
		decrementIfNeeded(remainingDifficulty, q.Difficulty)
		decrementIfNeeded(remainingKP, questionRowKnowledgePointID(q))
	}
	return selected
}

func unmetDistributionWarnings(selected []questionRow, typeDist, difficultyDist, kpDist map[string]int) []map[string]any {
	return validateDistributions(len(selected), distributionSet{
		{Key: "type", Message: "题型约束无法同时满足", Requested: typeDist, Available: countQuestionRows(selected, func(q questionRow) string { return q.Type })},
		{Key: "difficulty", Message: "难度约束无法同时满足", Requested: difficultyDist, Available: countQuestionRows(selected, func(q questionRow) string { return q.Difficulty })},
		{Key: "knowledge_point_id", Message: "知识点约束无法同时满足", Requested: kpDist, Available: countQuestionRows(selected, questionRowKnowledgePointID)},
	})
}

func countQuestionRows(pool []questionRow, attr func(questionRow) string) map[string]int {
	out := map[string]int{}
	for _, q := range pool {
		if value := strings.TrimSpace(attr(q)); value != "" {
			out[value]++
		}
	}
	return out
}

func questionRowKnowledgePointID(q questionRow) string {
	if q.KnowledgePointID == nil || *q.KnowledgePointID == uuid.Nil {
		return ""
	}
	return q.KnowledgePointID.String()
}

func questionDraftPanel(kpName string, count int, qType, difficulty string, hitCount, referenceCount int) map[string]any {
	if strings.TrimSpace(qType) == "" {
		qType = "未指定"
	} else {
		qType = displayQuestionType(qType)
	}
	if strings.TrimSpace(difficulty) == "" {
		difficulty = "未指定"
	} else {
		difficulty = displayDifficulty(difficulty)
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
			label = "题型 " + displayQuestionType(typ)
		}
		if difficulty, ok := w["difficulty"].(string); ok && difficulty != "" {
			label = "难度 " + displayDifficulty(difficulty)
		}
		if kpID, ok := w["knowledge_point_id"].(string); ok && kpID != "" {
			label = "知识点 " + kpID
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
		rows.WriteString(html.EscapeString(strings.Trim(strings.Join([]string{displayQuestionType(q.Type), displayDifficulty(q.Difficulty)}, " / "), " /")))
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

func displayQuestionType(value string) string {
	switch strings.TrimSpace(value) {
	case "single_choice":
		return "单选题"
	case "multi_choice":
		return "多选题"
	case "fill_in":
		return "填空题"
	case "short_answer":
		return "简答题"
	case "essay":
		return "论述题"
	default:
		return strings.TrimSpace(value)
	}
}

func displayDifficulty(value string) string {
	switch strings.TrimSpace(value) {
	case "easy":
		return "简单"
	case "medium":
		return "中等"
	case "hard":
		return "困难"
	default:
		return strings.TrimSpace(value)
	}
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

func providerQuestionLabel(kpID string, refs []map[string]any) string {
	if len(refs) > 0 {
		if name := strings.TrimSpace(asString(refs[0]["knowledge_point_name"])); name != "" {
			return name
		}
		if name := strings.TrimSpace(asString(refs[0]["display_name"])); name != "" {
			return name
		}
	}
	return kpID
}


func copyIntMap(in map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range in {
		if strings.TrimSpace(k) != "" && v > 0 {
			out[k] = v
		}
	}
	return out
}

func sumPositive(in map[string]int) int {
	total := 0
	for _, v := range in {
		if v > 0 {
			total += v
		}
	}
	return total
}

func unmetScore(value string, remaining map[string]int) int {
	if remaining[strings.TrimSpace(value)] > 0 {
		return 1
	}
	return 0
}

func decrementIfNeeded(remaining map[string]int, value string) {
	value = strings.TrimSpace(value)
	if remaining[value] > 0 {
		remaining[value]--
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
