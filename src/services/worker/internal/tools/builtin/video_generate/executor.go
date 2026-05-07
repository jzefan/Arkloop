package videogenerate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	sharedconfig "arkloop/services/shared/config"
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
	videoGenerateConfigKey    = "video_generative.model"
	defaultGeneratedVideoName = "generated-video"
)

type ToolExecutor struct {
	store         objectstore.Store
	db            workerdata.QueryDB
	config        sharedconfig.Resolver
	routingLoader *routing.ConfigLoader
	generate      func(context.Context, llm.ResolvedGatewayConfig, llm.VideoGenerationRequest) (llm.GeneratedVideo, error)
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
		generate:      llm.GenerateVideoWithResolvedConfig,
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
		return toolutil.ErrResult("tool.not_configured", "video generation storage is not configured", started)
	}
	if execCtx.AccountID == nil {
		return toolutil.ErrResult("tool.execution_failed", "account context is required", started)
	}
	prompt := strings.TrimSpace(toolutil.StringArg(args, "prompt"))
	if prompt == "" {
		return toolutil.ErrResult("tool.args_invalid", "parameter prompt is required", started)
	}
	selected, err := e.resolveSelectedRoute(ctx, *execCtx.AccountID, execCtx.RunID)
	if err != nil {
		return toolutil.ErrResult("tool.not_configured", err.Error(), started)
	}
	resolved, err := pipeline.ResolveGatewayConfigFromSelectedRoute(*selected, false, 0)
	if err != nil {
		return toolutil.ErrResult("tool.execution_failed", fmt.Sprintf("resolve video model failed: %s", err.Error()), started)
	}
	request := llm.VideoGenerationRequest{
		Prompt:          prompt,
		AspectRatio:     strings.TrimSpace(toolutil.StringArg(args, "aspect_ratio")),
		Resolution:      strings.TrimSpace(toolutil.StringArg(args, "resolution")),
		NegativePrompt:  strings.TrimSpace(toolutil.StringArg(args, "negative_prompt")),
		DurationSeconds: int32Arg(args, "duration_seconds"),
		FPS:             int32Arg(args, "fps"),
		GenerateAudio:   boolPtrArg(args, "generate_audio"),
	}
	if inputImages := tools.ReferenceImagesFromPipelineRC(execCtx.PipelineRC, 1); len(inputImages) > 0 {
		request.InputImage = &inputImages[0]
	}
	generator := e.generate
	if generator == nil {
		generator = llm.GenerateVideoWithResolvedConfig
	}
	video, err := generator(ctx, resolved, request)
	if err != nil {
		return toolutil.ErrResultWithDetails(toolutil.ErrorClassForGenerateError(err), err.Error(), toolutil.ErrorDetailsForGenerateError(err), started)
	}
	if len(video.Bytes) == 0 && video.Download == nil {
		return toolutil.ErrResult("tool.execution_failed", "video provider returned empty video bytes", started)
	}

	contentType := normalizeVideoContentType(video.MimeType)
	filename := defaultGeneratedVideoName + fileExtForContentType(contentType)
	key := toolutil.BuildArtifactKey(execCtx, filename)
	var threadID *string
	if execCtx.ThreadID != nil {
		value := execCtx.ThreadID.String()
		threadID = &value
	}
	metadata := objectstore.ArtifactMetadata(objectstore.ArtifactOwnerKindRun, execCtx.RunID.String(), execCtx.AccountID.String(), threadID)
	opts := objectstore.PutOptions{ContentType: contentType, Metadata: metadata}

	byteCount := len(video.Bytes)
	if video.Download != nil {
		rc, httpContentType, err := video.Download(ctx)
		if err != nil {
			return toolutil.ErrResultWithDetails(toolutil.ErrorClassForGenerateError(err), err.Error(), toolutil.ErrorDetailsForGenerateError(err), started)
		}
		defer rc.Close()
		if video.MimeType == "" {
			contentType = normalizeVideoContentType(httpContentType)
			opts.ContentType = contentType
			filename = defaultGeneratedVideoName + fileExtForContentType(contentType)
			key = toolutil.BuildArtifactKey(execCtx, filename)
		}
		if ss, ok := e.store.(objectstore.StreamingStore); ok {
			if err := ss.PutObjectStream(ctx, key, rc, opts); err != nil {
				return toolutil.ErrResult("tool.upload_failed", fmt.Sprintf("save generated video failed: %s", err.Error()), started)
			}
		} else {
			data, err := io.ReadAll(rc)
			if err != nil {
				return toolutil.ErrResult("tool.execution_failed", fmt.Sprintf("read generated video failed: %s", err.Error()), started)
			}
			if err := e.store.PutObject(ctx, key, data, opts); err != nil {
				return toolutil.ErrResult("tool.upload_failed", fmt.Sprintf("save generated video failed: %s", err.Error()), started)
			}
			byteCount = len(data)
		}
	} else {
		if err := e.store.PutObject(ctx, key, video.Bytes, opts); err != nil {
			return toolutil.ErrResult("tool.upload_failed", fmt.Sprintf("save generated video failed: %s", err.Error()), started)
		}
	}

	result := map[string]any{
		"provider":  video.ProviderKind,
		"model":     video.Model,
		"mime_type": contentType,
		"bytes":     byteCount,
		"artifacts": []map[string]any{
			{
				"key":       key,
				"filename":  filename,
				"size":      byteCount,
				"mime_type": contentType,
				"title":     defaultGeneratedVideoName,
				"display":   "inline",
			},
		},
	}
	return tools.ExecutionResult{ResultJSON: result, DurationMs: toolutil.DurationMs(started)}
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
		return nil, fmt.Errorf("video generation routing is not configured")
	}
	selector := ""
	if e.db != nil && runID != uuid.Nil {
		selector = strings.TrimSpace(e.runGenerationModelOverride(ctx, runID, "video"))
	}
	if selector == "" && e.db != nil {
		_ = e.db.QueryRow(ctx,
			`SELECT value FROM account_entitlement_overrides
			  WHERE account_id = $1 AND key = $2
			    AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
			  LIMIT 1`,
			accountID, videoGenerateConfigKey,
		).Scan(&selector)
	}
	selector = strings.TrimSpace(selector)
	if selector == "" && e.config != nil {
		if value, err := e.config.Resolve(ctx, videoGenerateConfigKey, sharedconfig.Scope{}); err == nil {
			selector = strings.TrimSpace(value)
		}
	}
	if selector == "" {
		return nil, fmt.Errorf("video generation model is not configured")
	}
	cfg, err := e.routingLoader.Load(ctx, &accountID)
	if err != nil {
		return nil, fmt.Errorf("load video routing config failed: %w", err)
	}
	if len(cfg.Routes) == 0 {
		return nil, fmt.Errorf("video routing config is empty")
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
		return nil, fmt.Errorf("video generation route not found for selector: %s", selector)
	}
	if route, cred, ok := cfg.GetHighestPriorityRouteByModel(selector, map[string]any{}); ok {
		selected := routing.CoerceZenMuxGenerationRoute(routing.SelectedProviderRoute{Route: route, Credential: cred})
		return &selected, nil
	}
	return nil, fmt.Errorf("video generation route not found for selector: %s", selector)
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

var allowedVideoContentTypes = map[string]bool{
	"video/mp4":       true,
	"video/webm":      true,
	"video/quicktime": true,
	"video/ogg":       true,
}

func normalizeVideoContentType(contentType string) string {
	cleaned := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if allowedVideoContentTypes[cleaned] {
		return cleaned
	}
	return "video/mp4"
}

func fileExtForContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	default:
		exts, err := mime.ExtensionsByType(contentType)
		if err == nil && len(exts) > 0 {
			return exts[0]
		}
		return ".mp4"
	}
}

func int32Arg(args map[string]any, key string) int32 {
	if args == nil {
		return 0
	}
	switch value := args[key].(type) {
	case int:
		return int32(value)
	case int32:
		return value
	case int64:
		return int32(value)
	case float64:
		return int32(value)
	default:
		return 0
	}
}

func boolPtrArg(args map[string]any, key string) *bool {
	if args == nil {
		return nil
	}
	if value, ok := args[key].(bool); ok {
		return &value
	}
	return nil
}

