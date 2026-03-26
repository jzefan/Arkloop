package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"arkloop/services/shared/messagecontent"
	"arkloop/services/shared/objectstore"
	"arkloop/services/shared/rollout"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/stablejson"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type MessageAttachmentStore interface {
	GetWithContentType(ctx context.Context, key string) ([]byte, string, error)
}

type runFirstEventLoader interface {
	FirstEventData(ctx context.Context, tx pgx.Tx, runID uuid.UUID) (string, map[string]any, error)
}

type runRecordLoader interface {
	GetRun(ctx context.Context, tx pgx.Tx, runID uuid.UUID) (*data.Run, error)
}

type loadedRunInputs struct {
	InputJSON        map[string]any
	Messages         []llm.Message
	ThreadMessageIDs []uuid.UUID
}

type LoadedRunInputs = loadedRunInputs

type resumeUnavailableError struct {
	reason string
}

func (e *resumeUnavailableError) Error() string {
	if e == nil || strings.TrimSpace(e.reason) == "" {
		return "resume context is unavailable"
	}
	return e.reason
}

const (
	resumeUnavailableErrorClass      = "resume.unavailable"
	interruptedToolErrorClass        = "tool.interrupted"
	interruptedToolErrorMessage      = "tool execution interrupted before result was recorded"
	runStartedThreadTailMessageIDKey = "thread_tail_message_id"
)

const ResumeUnavailableErrorClass = resumeUnavailableErrorClass

// NewInputLoaderMiddleware 加载 run 的 inputJSON 和线程历史消息到 RunContext。
func NewInputLoaderMiddleware(
	runsRepo data.RunsRepository,
	eventsRepo data.RunEventsRepository,
	messagesRepo data.MessagesRepository,
	attachmentStore MessageAttachmentStore,
	rolloutStore objectstore.BlobStore,
) RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		messageLimit := rc.ThreadMessageHistoryLimit
		if messageLimit <= 0 {
			messageLimit = 200
		}
		loaded, err := loadRunInputs(ctx, rc.Pool, rc.Run, runsRepo, eventsRepo, messagesRepo, attachmentStore, rolloutStore, messageLimit)
		if err != nil {
			var resumeErr *resumeUnavailableError
			if errors.As(err, &resumeErr) {
				failed := rc.Emitter.Emit("run.failed", map[string]any{
					"error_class": resumeUnavailableErrorClass,
					"message":     "resume context is unavailable",
				}, nil, StringPtr(resumeUnavailableErrorClass))
				return appendAndCommitSingle(ctx, rc.Pool, rc.Run, runsRepo, eventsRepo, failed, rc.ReleaseSlot, rc.BroadcastRDB, rc.EventBus)
			}
			return err
		}

		rc.InputJSON = loaded.InputJSON
		rc.Messages = loaded.Messages
		rc.ThreadMessageIDs = loaded.ThreadMessageIDs

		return next(ctx, rc)
	}
}

func loadRunInputs(
	ctx context.Context,
	pool interface {
		BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	},
	run data.Run,
	runsRepo runRecordLoader,
	eventsRepo runFirstEventLoader,
	messagesRepo data.MessagesRepository,
	attachmentStore MessageAttachmentStore,
	rolloutStore objectstore.BlobStore,
	messageLimit int,
) (*loadedRunInputs, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	_, dataJSON, err := eventsRepo.FirstEventData(ctx, tx, run.ID)
	if err != nil {
		return nil, err
	}

	inputJSON := map[string]any{
		"account_id": run.AccountID.String(),
		"thread_id":  run.ThreadID.String(),
	}
	if dataJSON != nil {
		if rawRouteID, ok := dataJSON["route_id"].(string); ok && strings.TrimSpace(rawRouteID) != "" {
			inputJSON["route_id"] = strings.TrimSpace(rawRouteID)
		}
		if rawPersonaID, ok := dataJSON["persona_id"].(string); ok && strings.TrimSpace(rawPersonaID) != "" {
			inputJSON["persona_id"] = strings.TrimSpace(rawPersonaID)
		}
		if rawRole, ok := dataJSON["role"].(string); ok && strings.TrimSpace(rawRole) != "" {
			inputJSON["role"] = strings.TrimSpace(rawRole)
		}
		if rawOutputRouteID, ok := dataJSON["output_route_id"].(string); ok && strings.TrimSpace(rawOutputRouteID) != "" {
			inputJSON["output_route_id"] = strings.TrimSpace(rawOutputRouteID)
		}
		if rawModel, ok := dataJSON["model"].(string); ok && strings.TrimSpace(rawModel) != "" {
			inputJSON["model"] = strings.TrimSpace(rawModel)
		}
		if rawWorkDir, ok := dataJSON["work_dir"].(string); ok && strings.TrimSpace(rawWorkDir) != "" {
			inputJSON["work_dir"] = strings.TrimSpace(rawWorkDir)
		}
	}

	messages, err := messagesRepo.ListByThread(ctx, tx, run.AccountID, run.ThreadID, messageLimit)
	if err != nil {
		return nil, err
	}

	replayedMessages := []llm.Message(nil)
	tailStart := len(messages)
	if run.ResumeFromRunID != nil {
		replayedMessages, tailStart, err = loadResumedReplay(ctx, tx, run, runsRepo, eventsRepo, rolloutStore, messages)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return nil, err
	}

	llmMessages := make([]llm.Message, 0, len(messages)+len(replayedMessages))
	ids := make([]uuid.UUID, 0, len(messages)+len(replayedMessages))
	for i, msg := range messages {
		if i == tailStart && len(replayedMessages) > 0 {
			llmMessages = append(llmMessages, replayedMessages...)
			for range replayedMessages {
				ids = append(ids, uuid.Nil)
			}
		}
		if strings.TrimSpace(msg.Role) == "" {
			continue
		}
		parts, err := BuildMessageParts(ctx, attachmentStore, msg)
		if err != nil {
			return nil, err
		}
		llmMessages = append(llmMessages, llm.Message{
			Role:         msg.Role,
			Content:      parts,
			OutputTokens: msg.OutputTokens,
		})
		ids = append(ids, msg.ID)
	}
	if tailStart == len(messages) && len(replayedMessages) > 0 {
		llmMessages = append(llmMessages, replayedMessages...)
		for range replayedMessages {
			ids = append(ids, uuid.Nil)
		}
	}

	// 提取最后一条用户消息，供 Lua 脚本通过 context.get("last_user_message") 访问
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			inputJSON["last_user_message"] = strings.TrimSpace(messages[i].Content)
			break
		}
	}

	return &loadedRunInputs{
		InputJSON:        inputJSON,
		Messages:         llmMessages,
		ThreadMessageIDs: ids,
	}, nil
}

func LoadRunInputs(
	ctx context.Context,
	pool interface {
		BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	},
	run data.Run,
	runsRepo runRecordLoader,
	eventsRepo runFirstEventLoader,
	messagesRepo data.MessagesRepository,
	attachmentStore MessageAttachmentStore,
	rolloutStore objectstore.BlobStore,
	messageLimit int,
) (*LoadedRunInputs, error) {
	return loadRunInputs(ctx, pool, run, runsRepo, eventsRepo, messagesRepo, attachmentStore, rolloutStore, messageLimit)
}

func IsResumeUnavailableError(err error) bool {
	var resumeErr *resumeUnavailableError
	return errors.As(err, &resumeErr)
}

func loadResumedReplay(
	ctx context.Context,
	tx pgx.Tx,
	run data.Run,
	runsRepo runRecordLoader,
	eventsRepo runFirstEventLoader,
	rolloutStore objectstore.BlobStore,
	threadMessages []data.ThreadMessage,
) ([]llm.Message, int, error) {
	if run.ResumeFromRunID == nil {
		return nil, len(threadMessages), nil
	}
	if rolloutStore == nil {
		return nil, 0, &resumeUnavailableError{reason: "resume rollout store is unavailable"}
	}
	if runsRepo == nil {
		return nil, 0, &resumeUnavailableError{reason: "resume run repository is unavailable"}
	}
	if eventsRepo == nil {
		return nil, 0, &resumeUnavailableError{reason: "resume event repository is unavailable"}
	}

	parentRun, err := runsRepo.GetRun(ctx, tx, *run.ResumeFromRunID)
	if err != nil {
		return nil, 0, err
	}
	if parentRun == nil {
		return nil, 0, &resumeUnavailableError{reason: "resume source run does not exist"}
	}
	if parentRun.ThreadID != run.ThreadID {
		return nil, 0, &resumeUnavailableError{reason: "resume source run does not belong to the thread"}
	}
	if parentRun.Status != "interrupted" && parentRun.Status != "cancelled" {
		return nil, 0, &resumeUnavailableError{reason: "resume source run is not resumable"}
	}

	_, parentStartedData, err := eventsRepo.FirstEventData(ctx, tx, parentRun.ID)
	if err != nil {
		return nil, 0, err
	}
	anchorMessageID, err := resumeAnchorMessageID(parentStartedData)
	if err != nil {
		return nil, 0, err
	}
	tailStart, ok := trailingResumeUserBlockAfterMessage(threadMessages, anchorMessageID)
	if !ok {
		return nil, 0, &resumeUnavailableError{reason: "resume input block is missing"}
	}

	items, err := rollout.NewReader(rolloutStore).ReadRollout(ctx, *run.ResumeFromRunID)
	if err != nil {
		return nil, 0, &resumeUnavailableError{reason: "resume rollout is unavailable"}
	}
	state := rollout.NewReader(rolloutStore).Reconstruct(items)
	replayedMessages, err := buildReplayMessages(state)
	if err != nil {
		return nil, 0, err
	}
	return replayedMessages, tailStart, nil
}

func resumeAnchorMessageID(dataJSON map[string]any) (uuid.UUID, error) {
	if dataJSON == nil {
		return uuid.Nil, &resumeUnavailableError{reason: "resume source run has no thread tail anchor"}
	}
	raw, _ := dataJSON[runStartedThreadTailMessageIDKey].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, &resumeUnavailableError{reason: "resume source run has no thread tail anchor"}
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, &resumeUnavailableError{reason: "resume source run has invalid thread tail anchor"}
	}
	return id, nil
}

func trailingResumeUserBlockAfterMessage(messages []data.ThreadMessage, anchorMessageID uuid.UUID) (int, bool) {
	if len(messages) == 0 {
		return 0, false
	}
	anchorIndex := -1
	for i, msg := range messages {
		if msg.ID == anchorMessageID {
			anchorIndex = i
			break
		}
	}
	if anchorIndex < 0 || anchorIndex == len(messages)-1 {
		return 0, false
	}
	for i := anchorIndex + 1; i < len(messages); i++ {
		if messages[i].Role != "user" {
			return 0, false
		}
	}
	return anchorIndex + 1, true
}

func buildReplayMessages(state *rollout.ReconstructedState) ([]llm.Message, error) {
	if state == nil {
		return nil, nil
	}
	replayed := make([]llm.Message, 0, len(state.ReplayMessages)+len(state.PendingToolCalls))
	for _, msg := range state.ReplayMessages {
		switch msg.Role {
		case "assistant":
			if msg.Assistant == nil {
				continue
			}
			replayed = append(replayed, replayAssistantMessage(*msg.Assistant))
		case "tool":
			if msg.Tool == nil {
				continue
			}
			replayed = append(replayed, replayToolResultMessage(*msg.Tool))
		}
	}
	for _, call := range state.PendingToolCalls {
		replayed = append(replayed, replayToolResultMessage(rollout.ReplayToolResult{
			CallID:    call.CallID,
			Name:      call.Name,
			Error:     interruptedToolErrorMessage,
			Synthetic: true,
		}))
	}
	return replayed, nil
}

func replayAssistantMessage(msg rollout.AssistantMessage) llm.Message {
	var toolCalls []llm.ToolCall
	if len(msg.ToolCalls) > 0 {
		_ = json.Unmarshal(msg.ToolCalls, &toolCalls)
	}
	content := []llm.ContentPart(nil)
	if strings.TrimSpace(msg.Content) != "" {
		content = []llm.ContentPart{{Type: messagecontent.PartTypeText, Text: msg.Content}}
	}
	return llm.Message{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	}
}

func replayToolResultMessage(result rollout.ReplayToolResult) llm.Message {
	envelope := map[string]any{
		"tool_call_id": result.CallID,
	}
	if strings.TrimSpace(result.Name) != "" {
		envelope["tool_name"] = strings.TrimSpace(result.Name)
	}
	if len(result.Output) > 0 {
		var output any
		if err := json.Unmarshal(result.Output, &output); err == nil {
			envelope["result"] = output
		}
	}
	if strings.TrimSpace(result.Error) != "" {
		errorClass := interruptedToolErrorClass
		if !result.Synthetic {
			errorClass = "tool.error"
		}
		envelope["error"] = map[string]any{
			"error_class": errorClass,
			"message":     strings.TrimSpace(result.Error),
		}
	}
	text, err := stablejson.Encode(envelope)
	if err != nil {
		encoded, _ := json.Marshal(envelope)
		text = string(encoded)
	}
	return llm.Message{
		Role:    "tool",
		Content: []llm.TextPart{{Text: text, TrustSource: "tool"}},
	}
}

func BuildMessageParts(ctx context.Context, store MessageAttachmentStore, msg data.ThreadMessage) ([]llm.ContentPart, error) {
	if len(msg.ContentJSON) == 0 {
		return fallbackTextParts(msg.Content), nil
	}
	parsed, err := messagecontent.Parse(msg.ContentJSON)
	if err != nil {
		return fallbackTextParts(msg.Content), nil
	}
	content, err := messagecontent.Normalize(parsed.Parts)
	if err != nil {
		return fallbackTextParts(msg.Content), nil
	}
	parts := make([]llm.ContentPart, 0, len(content.Parts))
	for _, part := range content.Parts {
		switch part.Type {
		case messagecontent.PartTypeText:
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			parts = append(parts, llm.ContentPart{Type: messagecontent.PartTypeText, Text: part.Text})
		case messagecontent.PartTypeFile:
			parts = append(parts, llm.ContentPart{
				Type:          messagecontent.PartTypeFile,
				Attachment:    part.Attachment,
				ExtractedText: part.ExtractedText,
			})
		case messagecontent.PartTypeImage:
			if part.Attachment == nil {
				return nil, fmt.Errorf("message image attachment is required")
			}
			if store == nil {
				return nil, fmt.Errorf("message attachment store not configured")
			}
			dataBytes, contentType, err := store.GetWithContentType(ctx, part.Attachment.Key)
			if err != nil {
				if objectstore.IsNotFound(err) {
					return nil, fmt.Errorf("message attachment not found")
				}
				return nil, err
			}
			attachment := *part.Attachment
			if strings.TrimSpace(contentType) != "" {
				attachment.MimeType = strings.TrimSpace(contentType)
			}
			parts = append(parts, llm.ContentPart{
				Type:       messagecontent.PartTypeImage,
				Attachment: &attachment,
				Data:       dataBytes,
			})
		}
	}
	if len(parts) == 0 {
		return fallbackTextParts(msg.Content), nil
	}
	return parts, nil
}

func fallbackTextParts(content string) []llm.ContentPart {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	return []llm.ContentPart{{Type: messagecontent.PartTypeText, Text: content}}
}
