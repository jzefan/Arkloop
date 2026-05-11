package markdowntopdf

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/tools"
)

const (
	errorArgsInvalid    = "tool.args_invalid"
	errorUploadFailed   = "tool.upload_failed"
	errorFontNotFound   = "tool.font_not_found"
	errorRenderFailed   = "tool.render_failed"
	pdfMimeType         = "application/pdf"
	pdfDisplayDownload  = "download"
	defaultFontSelector = ""
)

// ToolExecutor implements the markdown_to_pdf tool using signintech/gopdf
// for layout and a system-resident TrueType font for glyph rendering.
type ToolExecutor struct {
	store interface {
		PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
	}
	logger *slog.Logger
}

// NewToolExecutor constructs a ToolExecutor. Extra executors (sandbox etc.)
// passed by the caller are ignored — we no longer shell out to any external
// renderer. They are accepted to preserve the pre-existing constructor
// signature so the caller site (artifact_tools.go) needs no changes.
func NewToolExecutor(store interface {
	PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
}, _ ...tools.Executor) *ToolExecutor {
	return &ToolExecutor{store: store, logger: slog.Default()}
}

// Execute converts the caller-supplied Markdown into a PDF artifact and
// uploads it to the configured object store. See spec.go for schema.
func (e *ToolExecutor) Execute(
	ctx context.Context,
	_ string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()

	if blocked, isBlocked := tools.PlanModeBlocked(execCtx.PipelineRC, started); isBlocked {
		return blocked
	}

	filename, _ := args["filename"].(string)
	filename = normalizePDFFilename(filename)
	if filename == "" {
		return errResult(errorArgsInvalid, "parameter filename is required", started)
	}

	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return errResult(errorArgsInvalid, "parameter content is required", started)
	}

	template, _ := args["template"].(string)
	template = strings.TrimSpace(template)
	if template != "" && template != "formal_report" {
		return errResult(errorArgsInvalid, "parameter template must be formal_report", started)
	}

	title, _ := args["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}

	// Resolve the CJK font.
	fontPath, _ := args["font_path"].(string)
	font, err := ResolveCJKFont(fontPath)
	if err != nil {
		return errResult(errorFontNotFound, err.Error(), started)
	}

	// Local image access is not exposed through this tool by default; only
	// HTTP(S) URLs and data URIs will be accepted. If a caller needs local
	// filesystem access they can supply an explicit "image_roots" list.
	roots := parseStringSlice(args["image_roots"])

	pdfBytes, err := Render(RenderOptions{
		Ctx:               ctx,
		Title:             title,
		Markdown:          content,
		Font:              font,
		AllowedImageRoots: roots,
		Logger:            e.logger,
	})
	if err != nil {
		return errResult(errorRenderFailed, fmt.Sprintf("render pdf failed: %s", err.Error()), started)
	}

	return e.uploadLocalPDF(ctx, execCtx, filename, title, pdfBytes, started)
}

func (e *ToolExecutor) uploadLocalPDF(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	filename string,
	title string,
	pdfBytes []byte,
	started time.Time,
) tools.ExecutionResult {
	accountPrefix := "_anonymous"
	if execCtx.AccountID != nil {
		accountPrefix = execCtx.AccountID.String()
	}
	key := fmt.Sprintf("%s/%s/%s", accountPrefix, execCtx.RunID.String(), filename)

	var threadID *string
	if execCtx.ThreadID != nil {
		value := execCtx.ThreadID.String()
		threadID = &value
	}
	metadata := objectstore.ArtifactMetadata(
		objectstore.ArtifactOwnerKindRun,
		execCtx.RunID.String(),
		accountPrefix,
		threadID,
	)

	if err := e.store.PutObject(ctx, key, pdfBytes, objectstore.PutOptions{
		ContentType: pdfMimeType,
		Metadata:    metadata,
	}); err != nil {
		return errResult(errorUploadFailed, fmt.Sprintf("upload failed: %s", err.Error()), started)
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"artifacts": []map[string]any{
				{
					"key":       key,
					"filename":  filename,
					"size":      len(pdfBytes),
					"mime_type": pdfMimeType,
					"title":     title,
					"display":   pdfDisplayDownload,
				},
			},
		},
		DurationMs: durationMs(started),
	}
}

// normalizePDFFilename ensures the filename ends in ".pdf" (case-insensitive).
func normalizePDFFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return ""
	}
	ext := filepath.Ext(filename)
	if strings.EqualFold(ext, ".pdf") {
		return filename
	}
	if ext == "" {
		return filename + ".pdf"
	}
	return strings.TrimSuffix(filename, ext) + ".pdf"
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

func errResult(errorClass, message string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: errorClass,
			Message:    message,
		},
		DurationMs: durationMs(started),
	}
}

func durationMs(started time.Time) int {
	elapsed := time.Since(started)
	millis := int(elapsed / time.Millisecond)
	if millis < 0 {
		return 0
	}
	return millis
}
