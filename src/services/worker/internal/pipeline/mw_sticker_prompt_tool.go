package pipeline

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

const stickerSearchToolName = "sticker_search"

var stickerSearchAgentSpec = tools.AgentToolSpec{
	Name:        stickerSearchToolName,
	Version:     "1",
	Description: "搜索当前账户可发送的 Telegram sticker。",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var stickerSearchLlmSpec = llm.ToolSpec{
	Name:        stickerSearchToolName,
	Description: stickerStringPtr("搜索当前账户可发送的 Telegram sticker。当热 sticker 列表不够用时调用。"),
	JSONSchema: map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"query"},
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "情绪、场景、meme 或想表达的意思。",
			},
		},
	},
}

func NewStickerInjectMiddleware(db data.QueryDB) RunMiddleware {
	repo := data.AccountStickersRepository{}
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if rc == nil || rc.ChannelContext == nil || !strings.EqualFold(strings.TrimSpace(rc.ChannelContext.ChannelType), "telegram") || db == nil {
			return next(ctx, rc)
		}
		items, err := repo.ListHot(ctx, db, rc.Run.AccountID, time.Now().UTC().Add(-7*24*time.Hour), 20)
		if err != nil || len(items) == 0 {
			return next(ctx, rc)
		}
		rc.UpsertPromptSegment(PromptSegment{
			Name:      "telegram.stickers",
			Target:    PromptTargetSystemPrefix,
			Role:      "system",
			Stability: PromptStabilityStablePrefix,
			Text:      renderHotStickerPrompt(items),
		})
		rc.UpsertPromptSegment(PromptSegment{
			Name:      "telegram.sticker_instruction",
			Target:    PromptTargetSystemPrefix,
			Role:      "system",
			Stability: PromptStabilityStablePrefix,
			Text:      "需要发表情时，优先从上面的 sticker 列表选择，直接输出 [sticker:<id>]；如果没有合适的，再调用 sticker_search。",
		})
		return next(ctx, rc)
	}
}

func renderHotStickerPrompt(items []data.AccountSticker) string {
	var sb strings.Builder
	sb.WriteString("<stickers>\n")
	for _, item := range items {
		sb.WriteString(fmt.Sprintf(
			"  <sticker id=\"%s\" short=\"%s\" />\n",
			xmlEscapeAttr(strings.TrimSpace(item.ContentHash)),
			xmlEscapeAttr(strings.TrimSpace(item.ShortTags)),
		))
	}
	sb.WriteString("</stickers>")
	return sb.String()
}

func xmlEscapeAttr(value string) string {
	var sb strings.Builder
	if err := xml.EscapeText(&sb, []byte(value)); err != nil {
		return value
	}
	escaped := sb.String()
	escaped = strings.ReplaceAll(escaped, `"`, "&quot;")
	escaped = strings.ReplaceAll(escaped, `'`, "&apos;")
	return escaped
}

func NewStickerToolMiddleware(db data.QueryDB) RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if rc == nil || rc.ChannelContext == nil || !strings.EqualFold(strings.TrimSpace(rc.ChannelContext.ChannelType), "telegram") || db == nil {
			return next(ctx, rc)
		}
		for _, denied := range rc.ToolDenylist {
			if strings.EqualFold(strings.TrimSpace(denied), stickerSearchToolName) {
				return next(ctx, rc)
			}
		}
		if rc.PersonaDefinition != nil && len(rc.PersonaDefinition.ToolAllowlist) > 0 && !containsStickerToolName(rc.PersonaDefinition.ToolAllowlist, stickerSearchToolName) {
			return next(ctx, rc)
		}
		rc.ToolExecutors[stickerSearchToolName] = &stickerSearchExecutor{db: db, accountID: rc.Run.AccountID}
		rc.AllowlistSet[stickerSearchToolName] = struct{}{}
		rc.ToolSpecs = append(rc.ToolSpecs, stickerSearchLlmSpec)
		rc.ToolRegistry = ForkRegistry(rc.ToolRegistry, []tools.AgentToolSpec{stickerSearchAgentSpec})
		return next(ctx, rc)
	}
}

type stickerSearchExecutor struct {
	db        data.QueryDB
	accountID uuid.UUID
}

func (e *stickerSearchExecutor) Execute(ctx context.Context, toolName string, args map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
	query, _ := args["query"].(string)
	items, err := data.AccountStickersRepository{}.Search(ctx, e.db, e.accountID, query, 10)
	if err != nil {
		return tools.ExecutionResult{Error: &tools.ExecutionError{
			ErrorClass: tools.ErrorClassToolExecutionFailed,
			Message:    err.Error(),
		}}
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		results = append(results, map[string]any{
			"id":          item.ContentHash,
			"short_tags":  item.ShortTags,
			"long_desc":   item.LongDesc,
			"usage_count": item.UsageCount,
		})
	}
	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"query":   strings.TrimSpace(query),
			"results": results,
		},
	}
}

func stickerStringPtr(value string) *string { return &value }

func containsStickerToolName(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
