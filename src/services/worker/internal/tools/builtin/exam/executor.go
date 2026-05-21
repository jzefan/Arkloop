package exam

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// ToolExecutor implements all four exam_* tools. Sharing a single struct
// lets the four tools reuse one *Client instance and one cached HTTP pool.
type ToolExecutor struct {
	client *Client
}

func NewToolExecutor(client *Client) *ToolExecutor {
	return &ToolExecutor{client: client}
}

// IsNotConfigured marks the tool surface as hidden when the worker has no
// EXAM_BASE_URL / service token. Without a client we should not advertise
// the tools to the model at all.
func (e *ToolExecutor) IsNotConfigured() bool {
	return e == nil || e.client == nil
}

func (e *ToolExecutor) Execute(
	ctx context.Context,
	toolName string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()
	if blocked, isBlocked := tools.PlanModeBlocked(execCtx.PipelineRC, started); isBlocked {
		return blocked
	}

	if e.IsNotConfigured() {
		return errResult("exam.not_configured", "exam tools are disabled on this worker", started)
	}
	userID := uuid.Nil
	if execCtx.UserID != nil {
		userID = *execCtx.UserID
	}

	switch toolName {
	case ToolNameRecognizeCatalogImage:
		return e.recognizeCatalogImage(ctx, args, userID, started)
	case ToolNameParseCatalogExcel:
		return e.parseCatalogExcel(args, started)
	case ToolNameCreateCatalogTree:
		return e.createCatalogTree(ctx, args, userID, started)
	case ToolNameGenerateQuestions:
		return e.generateQuestions(ctx, args, userID, started)
	default:
		return errResult("exam.unknown_tool", "unrecognized exam tool: "+toolName, started)
	}
}

// ─── exam_recognize_catalog_image ─────────────────────────────────────

func (e *ToolExecutor) recognizeCatalogImage(
	ctx context.Context, args map[string]any, userID uuid.UUID, started time.Time,
) tools.ExecutionResult {
	urls := parseStringSlice(args["image_urls"])
	if len(urls) == 0 {
		return errResult("exam.args_invalid", "image_urls is required", started)
	}
	var resp map[string]any
	err := e.client.callExam(ctx, userID,
		[]string{"openid", "exam:write"},
		"POST", "/api/learning/catalog-photo/recognize",
		map[string]any{"image_urls": urls}, &resp)
	if err != nil {
		return errResult("exam.upstream_error", err.Error(), started)
	}
	return ok(resp, started)
}

// ─── exam_parse_catalog_excel ──────────────────────────────────────────

func (e *ToolExecutor) parseCatalogExcel(
	args map[string]any, started time.Time,
) tools.ExecutionResult {
	filePath, _ := args["file_path"].(string)
	if strings.TrimSpace(filePath) == "" {
		return errResult("exam.args_invalid", "file_path is required", started)
	}
	sheet, _ := args["sheet"].(string)

	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return errResult("exam.excel_open_failed", err.Error(), started)
	}
	defer f.Close()

	if sheet == "" {
		sheet = f.GetSheetName(0)
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return errResult("exam.excel_read_failed", err.Error(), started)
	}

	// Build the tree: course → chapter[] → section[]
	type chapter struct {
		Name     string   `json:"name"`
		Sections []string `json:"sections"`
	}
	type course struct {
		Name     string    `json:"name"`
		Chapters []chapter `json:"chapters"`
	}
	courses := make(map[string]*course)
	chapterIndex := make(map[string]int) // "course|chapter" → idx in courses[c].Chapters

	for i, row := range rows {
		if i == 0 { // header
			continue
		}
		if len(row) < 3 {
			continue
		}
		courseName := strings.TrimSpace(row[0])
		chapterName := strings.TrimSpace(row[1])
		sectionName := strings.TrimSpace(row[2])
		if courseName == "" || chapterName == "" || sectionName == "" {
			continue
		}
		c, ok := courses[courseName]
		if !ok {
			c = &course{Name: courseName}
			courses[courseName] = c
		}
		key := courseName + "|" + chapterName
		idx, exists := chapterIndex[key]
		if !exists {
			c.Chapters = append(c.Chapters, chapter{Name: chapterName})
			idx = len(c.Chapters) - 1
			chapterIndex[key] = idx
		}
		c.Chapters[idx].Sections = append(c.Chapters[idx].Sections, sectionName)
	}

	out := make([]*course, 0, len(courses))
	for _, c := range courses {
		out = append(out, c)
	}
	return ok(map[string]any{"courses": out}, started)
}

// ─── exam_create_catalog_tree ──────────────────────────────────────────

func (e *ToolExecutor) createCatalogTree(
	ctx context.Context, args map[string]any, userID uuid.UUID, started time.Time,
) tools.ExecutionResult {
	courseName, _ := args["course_name"].(string)
	courseName = strings.TrimSpace(courseName)
	if courseName == "" {
		return errResult("exam.args_invalid", "course_name is required", started)
	}
	chaptersRaw, _ := args["chapters"].([]any)
	if len(chaptersRaw) == 0 {
		return errResult("exam.args_invalid", "chapters must not be empty", started)
	}

	scopes := []string{"openid", "exam:write"}

	// 1) Create or reuse the direction (representing the course).
	var dirResp struct {
		ID uuid.UUID `json:"id"`
	}
	if err := e.client.callExam(ctx, userID, scopes,
		"POST", "/api/learning/directions",
		map[string]any{"name": courseName}, &dirResp); err != nil {
		return errResult("exam.upstream_error", "create direction: "+err.Error(), started)
	}
	directionID := dirResp.ID

	createdKPs := 0
	for _, chRaw := range chaptersRaw {
		ch, ok := chRaw.(map[string]any)
		if !ok {
			continue
		}
		chapterName, _ := ch["name"].(string)
		if strings.TrimSpace(chapterName) == "" {
			continue
		}
		var topKP struct {
			ID uuid.UUID `json:"id"`
		}
		if err := e.client.callExam(ctx, userID, scopes,
			"POST", "/api/learning/knowledge-points",
			map[string]any{
				"direction_id": directionID,
				"name":         chapterName,
				"level":        1,
			}, &topKP); err != nil {
			return errResult("exam.upstream_error", "create chapter: "+err.Error(), started)
		}
		createdKPs++

		sections, _ := ch["sections"].([]any)
		for _, sectRaw := range sections {
			sectName, _ := sectRaw.(string)
			if strings.TrimSpace(sectName) == "" {
				continue
			}
			if err := e.client.callExam(ctx, userID, scopes,
				"POST", "/api/learning/knowledge-points",
				map[string]any{
					"direction_id": directionID,
					"parent_id":    topKP.ID,
					"name":         sectName,
					"level":        2,
				}, nil); err != nil {
				return errResult("exam.upstream_error", "create section: "+err.Error(), started)
			}
			createdKPs++
		}
	}

	return ok(map[string]any{
		"direction_id":            directionID.String(),
		"knowledge_points_created": createdKPs,
	}, started)
}

// ─── exam_generate_questions ───────────────────────────────────────────

func (e *ToolExecutor) generateQuestions(
	ctx context.Context, args map[string]any, userID uuid.UUID, started time.Time,
) tools.ExecutionResult {
	kpIDs := parseStringSlice(args["knowledge_point_ids"])
	if len(kpIDs) == 0 {
		return errResult("exam.args_invalid", "knowledge_point_ids must not be empty", started)
	}
	count := 5
	if c, ok := args["count"].(float64); ok && c > 0 {
		count = int(c)
	}
	if count > 5 {
		count = 5 // hard cap; force the model into incremental display pattern
	}
	difficulty, _ := args["difficulty"].(string)
	if difficulty == "" {
		difficulty = "medium"
	}

	reqBody := map[string]any{
		"knowledge_point_ids": kpIDs,
		"total_count":         count,
		"difficulty":          difficulty,
	}
	if td, ok := args["type_distribution"].(map[string]any); ok {
		reqBody["type_distribution"] = td
	}

	// We read the SSE stream to completion and return all events. The model
	// is expected to call us again with another count to continue.
	scopes := []string{"openid", "exam:read", "exam:write"}
	events, err := e.streamGenerate(ctx, userID, scopes, reqBody)
	if err != nil {
		return errResult("exam.upstream_error", err.Error(), started)
	}
	return ok(map[string]any{
		"events":     events,
		"event_count": len(events),
	}, started)
}

// streamGenerate consumes the SSE response of POST /api/questions/ai-generate/stream
// and returns each event as a map. Stops on the first "done" or "error" event.
func (e *ToolExecutor) streamGenerate(
	ctx context.Context, userID uuid.UUID, scopes []string, body map[string]any,
) ([]map[string]any, error) {
	if e.client == nil {
		return nil, errors.New("not configured")
	}
	// We delegate to a thin internal helper to keep this file focused.
	return e.sseGenerate(ctx, userID, scopes, body)
}

// ─── helpers ───────────────────────────────────────────────────────────

func errResult(class, msg string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error:      &tools.ExecutionError{ErrorClass: class, Message: msg},
		DurationMs: durationMs(started),
	}
}

func ok(result map[string]any, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{ResultJSON: result, DurationMs: durationMs(started)}
}

func parseStringSlice(v any) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func durationMs(started time.Time) int {
	d := int(time.Since(started) / time.Millisecond)
	if d < 0 {
		return 0
	}
	return d
}

var _ = fmt.Sprintf // appease unused-import linter when fmt isn't otherwise touched
