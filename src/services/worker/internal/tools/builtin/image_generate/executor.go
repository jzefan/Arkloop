package imagegenerate

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	sharedconfig "arkloop/services/shared/config"
	"arkloop/services/shared/messagecontent"
	"arkloop/services/shared/objectstore"
	workerdata "arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/routing"
	"arkloop/services/worker/internal/tools"
	"arkloop/services/worker/internal/tools/builtin/internal/toolutil"

	"github.com/google/uuid"
)

const (
	imageGenerateConfigKey    = "image_generative.model"
	defaultGeneratedImageName = "generated-image"
)

type ToolExecutor struct {
	store         objectstore.Store
	db            workerdata.QueryDB
	config        sharedconfig.Resolver
	routingLoader *routing.ConfigLoader
	generate      func(context.Context, llm.ResolvedGatewayConfig, llm.ImageGenerationRequest) (llm.GeneratedImage, error)
}

func NewToolExecutor(
	store objectstore.Store,
	db workerdata.QueryDB,
	config sharedconfig.Resolver,
	routingLoader *routing.ConfigLoader,
) *ToolExecutor {
	return &ToolExecutor{
		store:         store,
		db:            db,
		config:        config,
		routingLoader: routingLoader,
		generate:      llm.GenerateImageWithResolvedConfig,
	}
}

func (e *ToolExecutor) Execute(
	ctx context.Context,
	_ string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()
	if e == nil || e.store == nil {
		return toolutil.ErrResult("tool.not_configured", "image generation storage is not configured", started)
	}
	if execCtx.AccountID == nil {
		return toolutil.ErrResult("tool.execution_failed", "account context is required", started)
	}

	prompt := strings.TrimSpace(toolutil.StringArg(args, "prompt"))
	if prompt == "" {
		return toolutil.ErrResult("tool.args_invalid", "parameter prompt is required", started)
	}
	inputImages, err := e.loadInputImages(ctx, args, *execCtx.AccountID)
	if err != nil {
		return toolutil.ErrResult("tool.args_invalid", err.Error(), started)
	}
	if len(inputImages) == 0 {
		inputImages = tools.ReferenceImagesFromPipelineRC(execCtx.PipelineRC, 5)
	}
	selected, err := e.resolveSelectedRoute(ctx, *execCtx.AccountID, execCtx.RunID)
	if err != nil {
		return toolutil.ErrResult("tool.not_configured", err.Error(), started)
	}
	request := llm.ImageGenerationRequest{
		Prompt:       prompt,
		InputImages:  inputImages,
		Size:         strings.TrimSpace(toolutil.StringArg(args, "size")),
		Quality:      strings.TrimSpace(toolutil.StringArg(args, "quality")),
		Background:   strings.TrimSpace(toolutil.StringArg(args, "background")),
		OutputFormat: strings.TrimSpace(toolutil.StringArg(args, "output_format")),
	}
	caps := routing.SelectedRouteModelCapabilities(selected)
	if selected.Credential.ProviderKind == routing.ProviderKindOpenAI && (caps.ModelType == "image" || (caps.SupportsOutputModality("image") && !caps.SupportsOutputModality("text"))) {
		request.ForceOpenAIImageAPI = true
	}
	resolved, err := pipeline.ResolveGatewayConfigFromSelectedRoute(*selected, false, 0)
	if err != nil {
		return toolutil.ErrResult("tool.execution_failed", fmt.Sprintf("resolve image model failed: %s", err.Error()), started)
	}

	generator := e.generate
	if generator == nil {
		generator = llm.GenerateImageWithResolvedConfig
	}
	image, err := generator(ctx, resolved, request)
	if err != nil {
		return toolutil.ErrResultWithDetails(toolutil.ErrorClassForGenerateError(err), err.Error(), toolutil.ErrorDetailsForGenerateError(err), started)
	}
	if len(image.Bytes) == 0 {
		return toolutil.ErrResult("tool.execution_failed", "image provider returned empty image bytes", started)
	}

	contentType := normalizeImageContentType(image.MimeType, image.Bytes)
	filename := defaultGeneratedImageName + fileExtForContentType(contentType)
	key := toolutil.BuildArtifactKey(execCtx, filename)
	var threadID *string
	if execCtx.ThreadID != nil {
		value := execCtx.ThreadID.String()
		threadID = &value
	}
	metadata := objectstore.ArtifactMetadata(objectstore.ArtifactOwnerKindRun, execCtx.RunID.String(), execCtx.AccountID.String(), threadID)
	if err := e.store.PutObject(ctx, key, image.Bytes, objectstore.PutOptions{
		ContentType: contentType,
		Metadata:    metadata,
	}); err != nil {
		return toolutil.ErrResult("tool.upload_failed", fmt.Sprintf("save generated image failed: %s", err.Error()), started)
	}

	result := map[string]any{
		"provider":  image.ProviderKind,
		"model":     image.Model,
		"mime_type": contentType,
		"bytes":     len(image.Bytes),
		"artifacts": []map[string]any{
			{
				"key":       key,
				"filename":  filename,
				"size":      len(image.Bytes),
				"mime_type": contentType,
				"title":     defaultGeneratedImageName,
				"display":   "inline",
			},
		},
	}
	if strings.TrimSpace(image.RevisedPrompt) != "" {
		result["revised_prompt"] = strings.TrimSpace(image.RevisedPrompt)
	}

	return tools.ExecutionResult{
		ResultJSON: result,
		DurationMs: toolutil.DurationMs(started),
	}
}

func (e *ToolExecutor) IsAvailableForAccount(ctx context.Context, accountID uuid.UUID) bool {
	if accountID == uuid.Nil {
		return false
	}
	_, err := e.resolveSelectedRoute(ctx, accountID, uuid.Nil)
	return err == nil
}

func (e *ToolExecutor) resolveSelectedRoute(ctx context.Context, accountID uuid.UUID, runID uuid.UUID) (*routing.SelectedProviderRoute, error) {
	if e.routingLoader == nil {
		return nil, fmt.Errorf("image generation routing is not configured")
	}
	selector := ""
	if e.db != nil && runID != uuid.Nil {
		selector = strings.TrimSpace(e.runGenerationModelOverride(ctx, runID, "image"))
	}
	if selector == "" && e.db != nil {
		_ = e.db.QueryRow(ctx,
			`SELECT value FROM account_entitlement_overrides
			  WHERE account_id = $1 AND key = $2
			    AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
			  LIMIT 1`,
			accountID, imageGenerateConfigKey,
		).Scan(&selector)
	}
	selector = strings.TrimSpace(selector)
	if selector == "" && e.config != nil {
		if value, err := e.config.Resolve(ctx, imageGenerateConfigKey, sharedconfig.Scope{}); err == nil {
			selector = strings.TrimSpace(value)
		}
	}
	if selector == "" {
		return nil, fmt.Errorf("image generation model is not configured")
	}

	cfg, err := e.routingLoader.Load(ctx, &accountID)
	if err != nil {
		return nil, fmt.Errorf("load image routing config failed: %w", err)
	}
	if len(cfg.Routes) == 0 {
		return nil, fmt.Errorf("image routing config is empty")
	}

	credName, modelName, exact := toolutil.SplitModelSelector(selector)
	if exact {
		if route, cred, ok := cfg.GetHighestPriorityRouteByCredentialAndModel(credName, modelName, map[string]any{}); ok {
			selected := routing.CoerceZenMuxGenerationRoute(routing.SelectedProviderRoute{Route: route, Credential: cred})
			return &selected, nil
		}
		if route, cred, ok := cfg.GetHighestPriorityRouteByCredentialName(credName, map[string]any{}); ok {
			route.Model = modelName
			selected := routing.CoerceZenMuxGenerationRoute(routing.SelectedProviderRoute{Route: route, Credential: cred})
			return &selected, nil
		}
		return nil, fmt.Errorf("image generation route not found for selector: %s", selector)
	}
	if route, cred, ok := cfg.GetHighestPriorityRouteByModel(selector, map[string]any{}); ok {
		selected := routing.CoerceZenMuxGenerationRoute(routing.SelectedProviderRoute{Route: route, Credential: cred})
		return &selected, nil
	}
	return nil, fmt.Errorf("image generation route not found for selector: %s", selector)
}

func (e *ToolExecutor) runGenerationModelOverride(ctx context.Context, runID uuid.UUID, task string) string {
	var raw string
	if err := e.db.QueryRow(ctx,
		`SELECT data_json FROM run_events
		  WHERE run_id = $1 AND type = 'run.started'
		  ORDER BY seq ASC LIMIT 1`,
		runID,
	).Scan(&raw); err != nil {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return ""
	}
	rawTask, _ := data["generation_task"].(string)
	if !strings.EqualFold(strings.TrimSpace(rawTask), task) {
		return ""
	}
	model, _ := data["generation_model"].(string)
	model = strings.TrimSpace(model)
	// Reject credential^model selectors from untrusted run event data to prevent
	// users from escalating to credentials not intended for their account tier.
	if strings.Contains(model, "^") {
		return ""
	}
	return model
}

func (e *ToolExecutor) loadInputImages(ctx context.Context, args map[string]any, accountID uuid.UUID) ([]llm.ContentPart, error) {
	if e == nil || e.store == nil || args == nil {
		return nil, nil
	}
	rawValues, ok := args["input_images"]
	if !ok || rawValues == nil {
		return nil, nil
	}
	items, ok := rawValues.([]any)
	if !ok {
		return nil, fmt.Errorf("parameter input_images must be an array of artifact references")
	}
	parts := make([]llm.ContentPart, 0, len(items))
	for idx, item := range items {
		raw, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("input_images[%d] must be a string", idx)
		}
		key := normalizeArtifactRef(raw)
		if key == "" {
			return nil, fmt.Errorf("input_images[%d] is empty", idx)
		}
		if !artifactKeyMatchesAccount(key, accountID) {
			return nil, fmt.Errorf("input_images[%d] is outside the current account", idx)
		}
		data, contentType, err := e.store.GetWithContentType(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("input_images[%d] not found", idx)
		}
		detectedType := httpDetectContentType(data)
		rawType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
		if !strings.HasPrefix(rawType, "image/") && !strings.HasPrefix(detectedType, "image/") {
			return nil, fmt.Errorf("input_images[%d] is not an image artifact", idx)
		}
		contentType = normalizeImageContentType(contentType, data)
		parts = append(parts, llm.ContentPart{
			Type: "image",
			Attachment: &messagecontent.AttachmentRef{
				Key:      key,
				Filename: filepath.Base(key),
				MimeType: contentType,
				Size:     int64(len(data)),
			},
			Data: data,
		})
	}
	return parts, nil
}

func normalizeArtifactRef(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "artifact:") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "artifact:"))
	}
	return trimmed
}

func artifactKeyMatchesAccount(key string, accountID uuid.UUID) bool {
	key = strings.TrimSpace(key)
	if key == "" || accountID == uuid.Nil {
		return false
	}
	return strings.HasPrefix(key, accountID.String()+"/")
}

func normalizeImageContentType(contentType string, data []byte) string {
	cleaned := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if strings.HasPrefix(cleaned, "image/") {
		return cleaned
	}
	if detected := httpDetectContentType(data); strings.HasPrefix(detected, "image/") {
		return detected
	}
	return "image/png"
}

func httpDetectContentType(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
}

func fileExtForContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		exts, err := mime.ExtensionsByType(contentType)
		if err == nil && len(exts) > 0 {
			return exts[0]
		}
		return ".png"
	}
}
