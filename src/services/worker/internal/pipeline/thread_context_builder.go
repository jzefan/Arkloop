package pipeline

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type canonicalThreadContext struct {
	VisibleMessages          []data.ThreadMessage
	Atoms                    []canonicalAtom
	Chunks                   []canonicalChunk
	Entries                  []canonicalThreadContextEntry
	Messages                 []llm.Message
	ThreadMessageIDs         []uuid.UUID
	HasLeadingCompactSummary bool
	LeadingCompactSummary    string
}

type canonicalThreadContextEntry struct {
	AnchorKey       string
	AtomKey         string
	Message         llm.Message
	ThreadMessageID uuid.UUID
	StartThreadSeq  int64
	EndThreadSeq    int64
	StartContextSeq int64
	EndContextSeq   int64
	IsReplacement   bool
	SummaryText     string
}

func buildCanonicalThreadContext(
	ctx context.Context,
	tx pgx.Tx,
	run data.Run,
	messagesRepo data.MessagesRepository,
	attachmentStore MessageAttachmentStore,
	upperBoundMessageID *uuid.UUID,
	messageLimit int,
) (*canonicalThreadContext, error) {
	fetchLimit := canonicalHistoryFetchLimit(messageLimit)

	var (
		visibleMessages []data.ThreadMessage
		err             error
		upperBoundSeq   *int64
	)
	if upperBoundMessageID != nil && *upperBoundMessageID != uuid.Nil {
		seq, seqErr := messagesRepo.GetThreadSeqByMessageID(ctx, tx, run.AccountID, run.ThreadID, *upperBoundMessageID)
		if seqErr != nil {
			return nil, seqErr
		}
		upperBoundSeq = &seq
		visibleMessages, err = messagesRepo.ListByThreadUpToID(ctx, tx, run.AccountID, run.ThreadID, *upperBoundMessageID, fetchLimit)
	} else {
		visibleMessages, err = messagesRepo.ListByThread(ctx, tx, run.AccountID, run.ThreadID, fetchLimit)
		if len(visibleMessages) > 0 {
			lastSeq := visibleMessages[len(visibleMessages)-1].ThreadSeq
			upperBoundSeq = &lastSeq
		}
	}
	if err != nil {
		return nil, err
	}

	renderableMessages := filterPromptRenderableThreadMessages(visibleMessages)
	atoms, chunks := buildCanonicalAtomGraph(renderableMessages)
	var upperBoundContextSeq *int64
	if len(chunks) > 0 {
		lastContextSeq := chunks[len(chunks)-1].ContextSeq
		upperBoundContextSeq = &lastContextSeq
	}

	replacementsRepo := data.ThreadContextReplacementsRepository{}
	var replacements []data.ThreadContextReplacementRecord
	if upperBoundContextSeq != nil {
		replacements, err = replacementsRepo.ListActiveByThreadUpToContextSeq(
			ctx,
			tx,
			run.AccountID,
			run.ThreadID,
			upperBoundContextSeq,
		)
	} else {
		replacements, err = replacementsRepo.ListActiveByThreadUpToSeq(
			ctx,
			tx,
			run.AccountID,
			run.ThreadID,
			upperBoundSeq,
		)
	}
	if err != nil {
		return nil, err
	}

	lastAtom := (*canonicalAtom)(nil)
	if len(atoms) > 0 {
		lastAtom = &atoms[len(atoms)-1]
	}
	mapped := mapReplacementsToContextSpans(replacements, chunks, upperBoundContextSeq)
	selected := selectRenderableReplacementSpans(mapped, lastAtom)
	entries, leadingSummary, err := renderCanonicalThreadMessagesFromGraph(ctx, attachmentStore, atoms, chunks, selected)
	if err != nil {
		return nil, err
	}
	renderedMessages := make([]llm.Message, 0, len(entries))
	renderedIDs := make([]uuid.UUID, 0, len(entries))
	for _, entry := range entries {
		renderedMessages = append(renderedMessages, entry.Message)
		renderedIDs = append(renderedIDs, entry.ThreadMessageID)
	}
	if messageLimit > 0 {
		entries = trimEntriesToMessageLimit(entries, messageLimit)
		renderedMessages = renderedMessages[:0]
		renderedIDs = renderedIDs[:0]
		for _, entry := range entries {
			renderedMessages = append(renderedMessages, entry.Message)
			renderedIDs = append(renderedIDs, entry.ThreadMessageID)
		}
	}

	return &canonicalThreadContext{
		VisibleMessages:          renderableMessages,
		Atoms:                    atoms,
		Chunks:                   chunks,
		Entries:                  entries,
		Messages:                 renderedMessages,
		ThreadMessageIDs:         renderedIDs,
		HasLeadingCompactSummary: strings.TrimSpace(leadingSummary) != "",
		LeadingCompactSummary:    strings.TrimSpace(leadingSummary),
	}, nil
}

func canonicalHistoryFetchLimit(messageLimit int) int {
	if messageLimit <= 0 {
		messageLimit = 200
	}
	return messageLimit
}

func filterPromptRenderableThreadMessages(messages []data.ThreadMessage) []data.ThreadMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]data.ThreadMessage, 0, len(messages))
	for _, msg := range messages {
		if isPromptExcludedThreadMessage(msg) {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func isPromptExcludedThreadMessage(msg data.ThreadMessage) bool {
	if len(msg.MetadataJSON) == 0 {
		return false
	}
	var metadata map[string]any
	if err := json.Unmarshal(msg.MetadataJSON, &metadata); err != nil {
		return false
	}
	return metadataBool(metadata["exclude_from_prompt"])
}

func metadataBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		return trimmed == "true" || trimmed == "1"
	case float64:
		return typed != 0
	default:
		return false
	}
}

func selectRenderableReplacements(items []data.ThreadContextReplacementRecord) []data.ThreadContextReplacementRecord {
	if len(items) == 0 {
		return nil
	}
	candidates := append([]data.ThreadContextReplacementRecord(nil), items...)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Layer != candidates[j].Layer {
			return candidates[i].Layer > candidates[j].Layer
		}
		if !candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
		}
		return candidates[i].StartContextSeq < candidates[j].StartContextSeq
	})

	selected := make([]data.ThreadContextReplacementRecord, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.SummaryText) == "" {
			continue
		}
		overlaps := false
		for _, existing := range selected {
			if candidate.StartContextSeq <= existing.EndContextSeq && candidate.EndContextSeq >= existing.StartContextSeq {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}
		selected = append(selected, candidate)
	}

	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].StartContextSeq != selected[j].StartContextSeq {
			return selected[i].StartContextSeq < selected[j].StartContextSeq
		}
		if selected[i].EndContextSeq != selected[j].EndContextSeq {
			return selected[i].EndContextSeq < selected[j].EndContextSeq
		}
		if selected[i].Layer != selected[j].Layer {
			return selected[i].Layer > selected[j].Layer
		}
		return selected[i].CreatedAt.Before(selected[j].CreatedAt)
	})
	return selected
}

func renderCanonicalThreadMessages(
	ctx context.Context,
	attachmentStore MessageAttachmentStore,
	messages []data.ThreadMessage,
	replacements []data.ThreadContextReplacementRecord,
) ([]canonicalThreadContextEntry, string, error) {
	atoms, chunks := buildCanonicalAtomGraph(messages)
	mapped := mapReplacementsToContextSpans(replacements, chunks, nil)
	selected := selectRenderableReplacementSpans(mapped, nil)
	return renderCanonicalThreadMessagesFromGraph(ctx, attachmentStore, atoms, chunks, selected)
}

func renderCanonicalThreadMessagesFromGraph(
	ctx context.Context,
	attachmentStore MessageAttachmentStore,
	atoms []canonicalAtom,
	chunks []canonicalChunk,
	replacements []canonicalReplacementSpan,
) ([]canonicalThreadContextEntry, string, error) {
	entries := make([]canonicalThreadContextEntry, 0, len(atoms)+len(replacements))
	leadingSummaries := make([]string, 0, 2)
	atomIndex := 0
	seenRealMessage := false

	appendReplacement := func(replacement canonicalReplacementSpan) {
		summary := strings.TrimSpace(replacement.Record.SummaryText)
		entries = append(entries, canonicalThreadContextEntry{
			AnchorKey:       replacementAnchorKey(replacement.Record.ID),
			Message:         makeCompactSnapshotMessage(summary),
			ThreadMessageID: uuid.Nil,
			StartThreadSeq:  replacement.Record.StartThreadSeq,
			EndThreadSeq:    replacement.Record.EndThreadSeq,
			StartContextSeq: replacement.StartContextSeq,
			EndContextSeq:   replacement.EndContextSeq,
			IsReplacement:   true,
			SummaryText:     summary,
		})
		if !seenRealMessage && summary != "" {
			leadingSummaries = append(leadingSummaries, summary)
		}
	}

	appendThreadMessage := func(atom canonicalAtom, msg data.ThreadMessage) error {
		if strings.TrimSpace(msg.Role) == "" {
			return nil
		}
		parts, err := BuildMessageParts(ctx, attachmentStore, msg)
		if err != nil {
			return err
		}
		if msg.Role == "tool" {
			parts = canonicalizeToolMessageParts(parts)
		}
		lm := llm.Message{
			Role:         msg.Role,
			Content:      parts,
			OutputTokens: msg.OutputTokens,
		}
		if msg.Role == "assistant" && len(msg.ContentJSON) > 0 {
			lm.ToolCalls = parseToolCallsFromContentJSON(msg.ContentJSON)
		}
		var keep bool
		lm, keep = filterLongTermHeartbeatDecision(lm)
		if !keep {
			return nil
		}
		entries = append(entries, canonicalThreadContextEntry{
			AnchorKey:       messageAnchorKey(msg.ID),
			AtomKey:         atom.Key,
			Message:         lm,
			ThreadMessageID: msg.ID,
			StartThreadSeq:  msg.ThreadSeq,
			EndThreadSeq:    msg.ThreadSeq,
			StartContextSeq: atom.StartContextSeq,
			EndContextSeq:   atom.EndContextSeq,
			IsReplacement:   false,
		})
		seenRealMessage = true
		return nil
	}

	appendAtom := func(atom canonicalAtom) error {
		for _, msg := range atom.Messages {
			if err := appendThreadMessage(atom, msg); err != nil {
				return err
			}
		}
		return nil
	}

	for _, replacement := range replacements {
		for atomIndex < len(atoms) && atoms[atomIndex].EndContextSeq < replacement.StartContextSeq {
			if err := appendAtom(atoms[atomIndex]); err != nil {
				return nil, "", err
			}
			atomIndex++
		}
		appendReplacement(replacement)
		for atomIndex < len(atoms) && atoms[atomIndex].StartContextSeq <= replacement.EndContextSeq {
			if tailEntry, ok := renderReplacementTailEntry(ctx, attachmentStore, atoms[atomIndex], chunks, replacement); ok {
				entries = append(entries, tailEntry)
				seenRealMessage = true
			}
			atomIndex++
		}
	}
	for atomIndex < len(atoms) {
		if err := appendAtom(atoms[atomIndex]); err != nil {
			return nil, "", err
		}
		atomIndex++
	}
	leadingSummary := strings.TrimSpace(strings.Join(leadingSummaries, "\n\n"))
	return entries, leadingSummary, nil
}

func trimEntriesToMessageLimit(entries []canonicalThreadContextEntry, messageLimit int) []canonicalThreadContextEntry {
	if messageLimit <= 0 || len(entries) == 0 {
		return entries
	}
	realCount := 0
	for _, entry := range entries {
		if !entry.IsReplacement {
			realCount++
		}
	}
	if realCount <= messageLimit {
		return entries
	}

	keptReal := 0
	cutoff := 0
	for i := len(entries) - 1; i >= 0; i-- {
		if !entries[i].IsReplacement {
			keptReal++
			if keptReal >= messageLimit {
				cutoff = i
				break
			}
		}
	}
	// Never split a protocol atom. If cutoff lands inside an atom, move to the atom head.
	if cutoff > 0 && cutoff < len(entries) && !entries[cutoff].IsReplacement && entries[cutoff].AtomKey != "" {
		atomKey := entries[cutoff].AtomKey
		for cutoff > 0 {
			prev := entries[cutoff-1]
			if prev.IsReplacement || prev.AtomKey != atomKey {
				break
			}
			cutoff--
		}
	}
	tail := entries[cutoff:]
	prefix := make([]canonicalThreadContextEntry, 0, cutoff)
	for i := 0; i < cutoff; i++ {
		if entries[i].IsReplacement {
			prefix = append(prefix, entries[i])
		}
	}
	if len(prefix) == 0 {
		return tail
	}
	return append(prefix, tail...)
}

func compactReplacementMetadata(kind string) json.RawMessage {
	payload, _ := json.Marshal(map[string]string{"kind": kind})
	if len(payload) == 0 {
		return json.RawMessage(`{}`)
	}
	return payload
}

func messageAnchorKey(messageID uuid.UUID) string {
	if messageID == uuid.Nil {
		return ""
	}
	return "message:" + messageID.String()
}

func replacementAnchorKey(replacementID uuid.UUID) string {
	if replacementID == uuid.Nil {
		return ""
	}
	return "replacement:" + replacementID.String()
}

func leadingCompactEntryCount(entries []canonicalThreadContextEntry) int {
	count := 0
	for _, entry := range entries {
		if !entry.IsReplacement {
			break
		}
		count++
	}
	return count
}

func renderedMessageAnchorKey(entries []canonicalThreadContextEntry, messageID uuid.UUID) string {
	for _, entry := range entries {
		if entry.ThreadMessageID == messageID && !entry.IsReplacement {
			return entry.AnchorKey
		}
	}
	return ""
}

func replacementAnchorKeyForThreadSeq(entries []canonicalThreadContextEntry, threadSeq int64) string {
	for _, entry := range entries {
		if entry.IsReplacement && entry.StartThreadSeq <= threadSeq && entry.EndThreadSeq >= threadSeq {
			return entry.AnchorKey
		}
	}
	return ""
}

func isLastRenderedMessage(entries []canonicalThreadContextEntry, messageID uuid.UUID) bool {
	if len(entries) == 0 {
		return false
	}
	last := entries[len(entries)-1]
	return !last.IsReplacement && last.ThreadMessageID == messageID
}

func renderReplacementTailEntry(
	ctx context.Context,
	attachmentStore MessageAttachmentStore,
	atom canonicalAtom,
	chunks []canonicalChunk,
	replacement canonicalReplacementSpan,
) (canonicalThreadContextEntry, bool) {
	if !atomSupportsPartialTail(atom) {
		return canonicalThreadContextEntry{}, false
	}
	if replacement.StartContextSeq > atom.StartContextSeq || replacement.EndContextSeq >= atom.EndContextSeq {
		return canonicalThreadContextEntry{}, false
	}
	role := strings.TrimSpace(atom.Messages[0].Role)
	if role != "user" && role != "assistant" {
		return canonicalThreadContextEntry{}, false
	}
	tailParts := make([]string, 0, atom.EndContextSeq-replacement.EndContextSeq)
	for _, chunk := range chunks {
		if chunk.AtomKey != atom.Key {
			continue
		}
		if chunk.ContextSeq <= replacement.EndContextSeq {
			continue
		}
		tailParts = append(tailParts, strings.TrimSpace(chunk.Content))
	}
	tailText := strings.TrimSpace(strings.Join(tailParts, "\n\n"))
	if tailText == "" {
		return canonicalThreadContextEntry{}, false
	}
	originalParts, err := BuildMessageParts(ctx, attachmentStore, atom.Messages[0])
	if err != nil {
		return canonicalThreadContextEntry{}, false
	}
	tailContent, ok := replaceTextPartsKeepingTail(originalParts, tailText)
	if !ok {
		return canonicalThreadContextEntry{}, false
	}
	return canonicalThreadContextEntry{
		AnchorKey:       messageAnchorKey(atom.Messages[0].ID),
		AtomKey:         atom.Key,
		Message:         llm.Message{Role: role, Content: tailContent, OutputTokens: atom.Messages[0].OutputTokens},
		ThreadMessageID: atom.Messages[0].ID,
		StartThreadSeq:  atom.Messages[0].ThreadSeq,
		EndThreadSeq:    atom.Messages[0].ThreadSeq,
		StartContextSeq: replacement.EndContextSeq + 1,
		EndContextSeq:   atom.EndContextSeq,
		IsReplacement:   false,
	}, true
}

func replaceTextPartsKeepingTail(parts []llm.ContentPart, tailText string) ([]llm.ContentPart, bool) {
	if strings.TrimSpace(tailText) == "" || len(parts) == 0 {
		return nil, false
	}
	out := make([]llm.ContentPart, 0, len(parts))
	inserted := false
	for _, part := range parts {
		if part.Kind() != "text" {
			out = append(out, part)
			continue
		}
		if inserted {
			continue
		}
		replaced := part
		replaced.Type = "text"
		replaced.Text = tailText
		out = append(out, replaced)
		inserted = true
	}
	if !inserted {
		return nil, false
	}
	return out, true
}
