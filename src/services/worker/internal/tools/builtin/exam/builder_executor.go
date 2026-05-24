package exam

import (
	"context"
	"fmt"
	"time"

	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

// BuilderExecutor implements the 4 new exam_* tools for M2b.
// It shares the same *Client as ToolExecutor (existing 4 tools).
type BuilderExecutor struct {
	client *Client
}

func NewBuilderExecutor(client *Client) *BuilderExecutor {
	return &BuilderExecutor{client: client}
}

func (e *BuilderExecutor) IsNotConfigured() bool {
	return e == nil || e.client == nil
}

func (e *BuilderExecutor) Execute(
	ctx context.Context,
	toolName string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()
	if e.IsNotConfigured() {
		return errResult("exam.not_configured", "exam builder tools are disabled", started)
	}
	userID := uuid.Nil
	if execCtx.UserID != nil {
		userID = *execCtx.UserID
	}

	switch toolName {
	case ToolNameListSeedQuestions:
		return e.listSeedQuestions(ctx, args, userID, started)
	case ToolNameBuildQuestions:
		return e.buildQuestions(ctx, args, userID, started)
	case ToolNameSaveQuestions:
		return e.saveQuestions(ctx, args, userID, started)
	case ToolNameBuildPaper:
		return e.buildPaper(ctx, args, userID, started)
	default:
		return errResult("exam.unknown_tool", "unrecognized builder tool: "+toolName, started)
	}
}

// ─── exam_list_seed_questions ──────────────────────────────────────────

func (e *BuilderExecutor) listSeedQuestions(
	ctx context.Context, args map[string]any, userID uuid.UUID, started time.Time,
) tools.ExecutionResult {
	kpID, _ := args["knowledge_point_id"].(string)
	if kpID == "" {
		return errResult("exam.args_invalid", "knowledge_point_id is required", started)
	}
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	if limit > 20 {
		limit = 20
	}

	params := map[string]any{
		"knowledge_point_id": kpID,
		"limit":              limit,
	}
	if t, ok := args["type"].(string); ok && t != "" {
		params["type"] = t
	}
	if d, ok := args["difficulty"].(string); ok && d != "" {
		params["difficulty"] = d
	}
	if pt, ok := args["pattern_tag"].(string); ok && pt != "" {
		params["pattern_tag"] = pt
	}

	scopes := []string{"openid", "exam:read"}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	path := "/api/questions?" + buildQuery(params)
	if err := e.client.callExam(ctx, userID, scopes, "GET", path, nil, &resp); err != nil {
		return errResult("exam.upstream_error", err.Error(), started)
	}
	if len(resp.Items) == 0 {
		return ok(map[string]any{
			"items":        []any{},
			"total":        0,
			"seed_required": true,
			"message":      "该知识点下没有可参考的种子题",
		}, started)
	}
	return ok(map[string]any{"items": resp.Items, "total": resp.Total}, started)
}

// ─── exam_build_questions ──────────────────────────────────────────────

func (e *BuilderExecutor) buildQuestions(
	ctx context.Context, args map[string]any, userID uuid.UUID, started time.Time,
) tools.ExecutionResult {
	seedIDs := parseStringSlice(args["seed_question_ids"])
	if len(seedIDs) == 0 {
		return errResult("exam.seed_required", "seed_question_ids must not be empty — no seed, no build", started)
	}
	skillKey, _ := args["skill_key"].(string)
	if skillKey == "" {
		return errResult("exam.args_invalid", "skill_key is required", started)
	}
	count := 5
	if c, ok := args["count"].(float64); ok && c > 0 {
		count = int(c)
	}
	if count > 5 {
		count = 5
	}

	// This tool delegates actual LLM generation to the agent loop —
	// it returns a structured "build_request" that the persona prompt
	// instructs the LLM to fulfill using the loaded skill context.
	// The tool itself does NOT call LLM; it prepares the generation context.
	return ok(map[string]any{
		"action":            "build_questions",
		"seed_question_ids": seedIDs,
		"skill_key":         skillKey,
		"count":             count,
		"instruction":       "Use the loaded skill (SKILL.md) to generate questions. Each question's pattern_tag MUST match the seed questions' pattern_tag. Return questions as JSON array.",
	}, started)
}

// ─── exam_save_questions ───────────────────────────────────────────────

func (e *BuilderExecutor) saveQuestions(
	ctx context.Context, args map[string]any, userID uuid.UUID, started time.Time,
) tools.ExecutionResult {
	questionsRaw, ok2 := args["questions"].([]any)
	if !ok2 || len(questionsRaw) == 0 {
		return errResult("exam.args_invalid", "questions array is required and must not be empty", started)
	}

	// Convert to the shape exam expects
	questions := make([]map[string]any, 0, len(questionsRaw))
	for _, q := range questionsRaw {
		if qm, ok3 := q.(map[string]any); ok3 {
			questions = append(questions, qm)
		}
	}

	scopes := []string{"openid", "exam:write"}
	var resp struct {
		Created []map[string]any `json:"created"`
		Failed  []map[string]any `json:"failed"`
	}
	if err := e.client.callExam(ctx, userID, scopes, "POST", "/api/questions/batch",
		map[string]any{"questions": questions}, &resp); err != nil {
		return errResult("exam.upstream_error", err.Error(), started)
	}
	return ok(map[string]any{
		"created":       resp.Created,
		"failed":        resp.Failed,
		"created_count": len(resp.Created),
		"failed_count":  len(resp.Failed),
	}, started)
}

// ─── exam_build_paper ──────────────────────────────────────────────────

func (e *BuilderExecutor) buildPaper(
	ctx context.Context, args map[string]any, userID uuid.UUID, started time.Time,
) tools.ExecutionResult {
	courseID, _ := args["course_id"].(string)
	if courseID == "" {
		return errResult("exam.args_invalid", "course_id is required", started)
	}
	name, _ := args["name"].(string)
	if name == "" {
		return errResult("exam.args_invalid", "name is required", started)
	}
	kpIDs := parseStringSlice(args["knowledge_point_ids"])
	if len(kpIDs) == 0 {
		return errResult("exam.args_invalid", "knowledge_point_ids must not be empty", started)
	}
	totalCount := 25
	if tc, ok2 := args["total_count"].(float64); ok2 && tc > 0 {
		totalCount = int(tc)
	}

	// Build spec
	spec := map[string]any{
		"total_count":          totalCount,
		"type_distribution":    args["type_distribution"],
		"difficulty_distribution": args["difficulty_distribution"],
	}
	if seed, ok2 := args["seed"].(float64); ok2 {
		spec["seed"] = int64(seed)
	}

	// First: list questions from pool
	scopes := []string{"openid", "exam:read", "exam:write"}
	poolQuestions := make([]map[string]any, 0)
	for _, kpID := range kpIDs {
		var resp struct {
			Items []map[string]any `json:"items"`
		}
		path := "/api/questions?knowledge_point_id=" + kpID + "&limit=200"
		if err := e.client.callExam(ctx, userID, scopes, "GET", path, nil, &resp); err != nil {
			return errResult("exam.upstream_error", "list pool: "+err.Error(), started)
		}
		poolQuestions = append(poolQuestions, resp.Items...)
	}

	if len(poolQuestions) < totalCount {
		return ok(map[string]any{
			"shortage_warnings": []map[string]any{{
				"message":   "题池不足",
				"available": len(poolQuestions),
				"requested": totalCount,
			}},
			"pool_size": len(poolQuestions),
		}, started)
	}

	// Simple selection: take first totalCount questions (real impl would use papercompose)
	questionIDs := make([]string, 0, totalCount)
	for i := 0; i < totalCount && i < len(poolQuestions); i++ {
		if id, ok2 := poolQuestions[i]["id"].(string); ok2 {
			questionIDs = append(questionIDs, id)
		}
	}

	// Save paper
	var paperResp map[string]any
	if err := e.client.callExam(ctx, userID, scopes, "POST", "/api/papers",
		map[string]any{
			"name":         name,
			"course_id":    courseID,
			"spec":         spec,
			"question_ids": questionIDs,
		}, &paperResp); err != nil {
		return errResult("exam.upstream_error", "create paper: "+err.Error(), started)
	}
	return ok(map[string]any{
		"paper":          paperResp,
		"question_count": len(questionIDs),
	}, started)
}

// ─── helpers ───────────────────────────────────────────────────────────

func buildQuery(params map[string]any) string {
	var parts []string
	for k, v := range params {
		switch val := v.(type) {
		case string:
			parts = append(parts, k+"="+val)
		case int:
			parts = append(parts, k+"="+intToStr(val))
		}
	}
	return joinParts(parts)
}

func intToStr(i int) string {
	return fmt.Sprintf("%d", i)
}

func joinParts(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "&"
		}
		result += p
	}
	return result
}
