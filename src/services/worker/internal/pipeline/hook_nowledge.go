package pipeline

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	sharedconfig "arkloop/services/shared/config"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/memory"
	"arkloop/services/worker/internal/memory/nowledge"

	"github.com/google/uuid"
)

const nowledgeProviderName = "nowledge"
const nowledgeThreadSource = "arkloop"

const nowledgeGuidanceTag = "nowledge_mem_guidance"

type externalThreadLinkStore interface {
	Get(ctx context.Context, accountID, threadID uuid.UUID, provider string) (string, bool, error)
	Upsert(ctx context.Context, accountID, threadID uuid.UUID, provider, externalThreadID string) error
}

type NowledgeContextContributor struct {
	provider *nowledge.Client
}

type nowledgePromptState struct {
	segments              PromptSegments
	guidance              string
	workingMemoryInjected bool
	recalledInjected      bool
}

func NewNowledgeContextContributor(provider *nowledge.Client) ContextContributor {
	if provider == nil {
		return nil
	}
	return &NowledgeContextContributor{provider: provider}
}

func (c *NowledgeContextContributor) HookProviderName() string { return nowledgeProviderName }

func (c *NowledgeContextContributor) collectPromptState(ctx context.Context, rc *RunContext) (nowledgePromptState, error) {
	if c == nil || c.provider == nil || rc == nil || rc.UserID == nil {
		return nowledgePromptState{}, nil
	}
	ident := memory.MemoryIdentity{
		AccountID: rc.Run.AccountID,
		UserID:    *rc.UserID,
		AgentID:   StableAgentID(rc),
	}
	state := nowledgePromptState{}
	query := buildNowledgeRecallQuery(rc)
	var workingMemory nowledge.WorkingMemory
	var searchResults []nowledge.SearchResult
	var searchErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		workingMemory, _ = c.provider.ReadWorkingMemory(ctx, ident)
	}()
	if strings.TrimSpace(query) != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			searchResults, searchErr = c.provider.SearchRich(ctx, ident, query, 5)
		}()
	}
	wg.Wait()

	if workingMemory.Available && strings.TrimSpace(workingMemory.Content) != "" {
		state.segments = append(state.segments, PromptSegment{
			Name:          "hook.before.nowledge.working_memory",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          "<working_memory>\n" + strings.TrimSpace(workingMemory.Content) + "\n</working_memory>",
			Stability:     PromptStabilitySessionPrefix,
			CacheEligible: true,
		})
		state.workingMemoryInjected = true
	}
	if strings.TrimSpace(query) == "" {
		state.guidance = buildNowledgeGuidanceText(state.workingMemoryInjected, state.recalledInjected)
		return state, nil
	}
	if searchErr != nil || len(searchResults) == 0 {
		state.guidance = buildNowledgeGuidanceText(state.workingMemoryInjected, state.recalledInjected)
		return state, nil
	}
	var lines []string
	for index, result := range searchResults {
		if result.Score < 0.1 {
			continue
		}
		abstract := compactInline(firstNonEmptyString(result.Content, result.Title), 250)
		if abstract == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %.0f%% %s", index+1, result.Score*100, abstract))
	}
	if len(lines) == 0 {
		state.guidance = buildNowledgeGuidanceText(state.workingMemoryInjected, state.recalledInjected)
		return state, nil
	}
	state.segments = append(state.segments, PromptSegment{
		Name:          "hook.before.nowledge.recalled_memories",
		Target:        PromptTargetRuntimeTail,
		Role:          "user",
		Text:          "<system_recalled_memories>\n以下是从历史对话中检索到的参考片段，仅供上下文参考\n" + strings.Join(lines, "\n") + "\n</system_recalled_memories>",
		Stability:     PromptStabilityVolatileTail,
		CacheEligible: false,
	})
	state.recalledInjected = true
	state.guidance = buildNowledgeGuidanceText(state.workingMemoryInjected, state.recalledInjected)
	return state, nil
}

func (c *NowledgeContextContributor) BeforePromptSegments(ctx context.Context, rc *RunContext) (PromptSegments, error) {
	state, err := c.collectPromptState(ctx, rc)
	if err != nil {
		return nil, err
	}
	segments := append(PromptSegments(nil), state.segments...)
	if strings.TrimSpace(state.guidance) != "" {
		segments = append(segments, PromptSegment{
			Name:          "hook.before.nowledge.guidance",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          strings.TrimSpace(state.guidance),
			Stability:     PromptStabilitySessionPrefix,
			CacheEligible: true,
		})
	}
	return segments, nil
}

func (c *NowledgeContextContributor) AfterPromptSegments(context.Context, *RunContext, string) (PromptSegments, error) {
	return nil, nil
}

type NowledgeCompactionAdvisor struct {
	provider *nowledge.Client
}

func NewNowledgeCompactionAdvisor(provider *nowledge.Client) CompactionAdvisor {
	if provider == nil {
		return nil
	}
	return &NowledgeCompactionAdvisor{provider: provider}
}

func (a *NowledgeCompactionAdvisor) HookProviderName() string { return nowledgeProviderName }

func (a *NowledgeCompactionAdvisor) BeforeCompact(ctx context.Context, rc *RunContext, _ CompactInput) (CompactHints, error) {
	if a == nil || a.provider == nil || rc == nil || rc.UserID == nil {
		return nil, nil
	}
	ident := memory.MemoryIdentity{
		AccountID: rc.Run.AccountID,
		UserID:    *rc.UserID,
		AgentID:   StableAgentID(rc),
	}
	workingMemory, err := a.provider.ReadWorkingMemory(ctx, ident)
	if err != nil || !workingMemory.Available || strings.TrimSpace(workingMemory.Content) == "" {
		return nil, err
	}
	return CompactHints{{
		Content:  "保留 working memory 中仍然有效的目标、决策和未完成事项：" + compactInline(workingMemory.Content, 240),
		Source:   nowledgeProviderName,
		Priority: 100,
	}}, nil
}

func (a *NowledgeCompactionAdvisor) AfterCompact(context.Context, *RunContext, CompactOutput) (PostCompactActions, error) {
	return nil, nil
}

type NowledgeThreadPersistenceProvider struct {
	provider *nowledge.Client
	links    externalThreadLinkStore
}

func NewNowledgeThreadPersistenceProvider(provider *nowledge.Client, links externalThreadLinkStore) ThreadPersistenceProvider {
	if provider == nil || links == nil {
		return nil
	}
	return &NowledgeThreadPersistenceProvider{provider: provider, links: links}
}

func (p *NowledgeThreadPersistenceProvider) HookProviderName() string { return nowledgeProviderName }

func (p *NowledgeThreadPersistenceProvider) PersistThread(ctx context.Context, rc *RunContext, delta ThreadDelta, _ ThreadPersistHints) ThreadPersistResult {
	result := ThreadPersistResult{Handled: false, Provider: nowledgeProviderName}
	if p == nil || p.provider == nil || p.links == nil || rc == nil || rc.UserID == nil {
		return result
	}
	if len(delta.Messages) == 0 && strings.TrimSpace(delta.AssistantOutput) == "" {
		return result
	}
	ident := memory.MemoryIdentity{
		AccountID: delta.AccountID,
		UserID:    delta.UserID,
		AgentID:   delta.AgentID,
	}
	externalThreadID, found, err := p.links.Get(ctx, delta.AccountID, delta.ThreadID, nowledgeProviderName)
	if err != nil {
		result.Err = err
		return result
	}
	payload := buildNowledgeThreadPayload(delta)
	if len(payload) == 0 {
		return result
	}
	if !found {
		externalThreadID, err = p.provider.CreateThread(ctx, ident, delta.ThreadID.String(), buildNowledgeThreadTitle(delta), nowledgeThreadSource, payload)
		if err != nil {
			result.Err = err
			return result
		}
		if strings.TrimSpace(externalThreadID) == "" {
			externalThreadID = delta.ThreadID.String()
		}
		if err := p.links.Upsert(ctx, delta.AccountID, delta.ThreadID, nowledgeProviderName, externalThreadID); err != nil {
			result.Err = err
			return result
		}
		result.Handled = true
		result.ExternalThreadID = externalThreadID
		result.AppendedMessages = len(payload)
		result.Committed = true
		return result
	}
	added, err := p.provider.AppendThread(ctx, ident, externalThreadID, payload, buildNowledgeAppendIdempotencyKey(delta, payload))
	if err != nil {
		result.Err = err
		return result
	}
	result.Handled = true
	result.ExternalThreadID = externalThreadID
	result.AppendedMessages = added
	result.Committed = true
	return result
}

type NowledgeDistillObserver struct {
	provider       *nowledge.Client
	links          externalThreadLinkStore
	configResolver sharedconfig.Resolver
	snap           MemorySnapshotStore
	mdb            data.MemoryMiddlewareDB
	impStore       ImpressionStore
	impRefresh     ImpressionRefreshFunc
}

func NewNowledgeDistillObserver(
	provider *nowledge.Client,
	links externalThreadLinkStore,
	configResolver sharedconfig.Resolver,
	snap MemorySnapshotStore,
	mdb data.MemoryMiddlewareDB,
	impStore ImpressionStore,
	impRefresh ImpressionRefreshFunc,
) AfterThreadPersistHook {
	if provider == nil || links == nil {
		return nil
	}
	return &NowledgeDistillObserver{
		provider:       provider,
		links:          links,
		configResolver: configResolver,
		snap:           snap,
		mdb:            mdb,
		impStore:       impStore,
		impRefresh:     impRefresh,
	}
}

func (o *NowledgeDistillObserver) HookProviderName() string { return nowledgeProviderName }

func (o *NowledgeDistillObserver) AfterThreadPersist(ctx context.Context, rc *RunContext, delta ThreadDelta, result ThreadPersistResult) (PersistObservers, error) {
	if o == nil || o.provider == nil || rc == nil || rc.UserID == nil {
		return nil, nil
	}
	if !resolveDistillEnabled(ctx, o.configResolver) {
		return nil, nil
	}
	if result.Err != nil || !result.Handled || !result.Committed {
		return nil, nil
	}
	ident := memory.MemoryIdentity{
		AccountID: delta.AccountID,
		UserID:    delta.UserID,
		AgentID:   delta.AgentID,
	}
	threadID := strings.TrimSpace(result.ExternalThreadID)
	if threadID == "" {
		linkID, found, err := o.links.Get(ctx, delta.AccountID, delta.ThreadID, nowledgeProviderName)
		if err != nil || !found {
			return nil, err
		}
		threadID = linkID
	}
	conversation := buildNowledgeConversation(delta)
	if strings.TrimSpace(conversation) == "" {
		return nil, nil
	}
	triage, err := o.provider.TriageConversation(ctx, ident, conversation)
	if err != nil || !triage.ShouldDistill {
		return nil, err
	}
	distill, err := o.provider.DistillThread(ctx, ident, threadID, buildNowledgeThreadTitle(delta), conversation)
	if err != nil {
		return nil, err
	}
	if distill.MemoriesCreated <= 0 {
		return nil, nil
	}
	if o.impStore != nil {
		addImpressionScore(ctx, o.impStore, ident, impressionScoreForRun(rc), o.configResolver, o.impRefresh)
	}
	scheduleSnapshotRefresh(
		o.provider,
		o.snap,
		o.mdb,
		rc.Run.ID,
		rc.TraceID,
		ident,
		threadID,
		buildNowledgeSnapshotQueries(delta),
		"memory.distill",
		"distill",
	)
	return nil, nil
}

func buildNowledgeSnapshotQueries(delta ThreadDelta) map[string][]string {
	queries := make([]string, 0, len(delta.Messages))
	for _, msg := range delta.Messages {
		if msg.Role != "user" {
			continue
		}
		if text := strings.TrimSpace(msg.Content); text != "" {
			queries = append(queries, text)
		}
	}
	if len(queries) == 0 {
		if title := strings.TrimSpace(buildNowledgeThreadTitle(delta)); title != "" {
			queries = append(queries, title)
		}
	}
	if len(queries) == 0 {
		return nil
	}
	return map[string][]string{
		string(memory.MemoryScopeUser): queries,
	}
}

func buildNowledgeRecallQuery(rc *RunContext) string {
	if rc == nil {
		return ""
	}
	var latest string
	for index := len(rc.Messages) - 1; index >= 0; index-- {
		message := rc.Messages[index]
		if message.Role != "user" {
			continue
		}
		latest = strings.TrimSpace(nowledgeMessageText(message))
		if latest != "" {
			break
		}
	}
	if len([]rune(latest)) < 3 {
		return ""
	}
	if len([]rune(latest)) >= 40 {
		return latest
	}
	window := make([]string, 0, 3)
	start := len(rc.Messages) - 3
	if start < 0 {
		start = 0
	}
	for _, msg := range rc.Messages[start:] {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		text := strings.TrimSpace(nowledgeMessageText(msg))
		if text == "" {
			continue
		}
		window = append(window, text)
	}
	return strings.TrimSpace(strings.Join(window, "\n"))
}

func nowledgeMessageText(message llm.Message) string {
	parts := make([]string, 0, len(message.Content))
	for _, part := range llm.VisibleContentParts(message.Content) {
		text := strings.TrimSpace(llm.PartPromptText(part))
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func compactInline(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func buildNowledgeThreadPayload(delta ThreadDelta) []nowledge.ThreadMessage {
	out := make([]nowledge.ThreadMessage, 0, len(delta.Messages)+1)
	sessionKey := delta.ThreadID.String()
	sessionID := delta.RunID.String()
	source := nowledgeThreadSource
	for _, message := range delta.Messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		index := len(out)
		out = append(out, nowledge.ThreadMessage{
			Role:     message.Role,
			Content:  content,
			Metadata: nowledge.BuildThreadMessageMetadata(source, sessionKey, sessionID, delta.ThreadID.String(), message.Role, content, index, delta.TraceID),
		})
	}
	if strings.TrimSpace(delta.AssistantOutput) != "" {
		index := len(out)
		out = append(out, nowledge.ThreadMessage{
			Role:     "assistant",
			Content:  strings.TrimSpace(delta.AssistantOutput),
			Metadata: nowledge.BuildThreadMessageMetadata(source, sessionKey, sessionID, delta.ThreadID.String(), "assistant", strings.TrimSpace(delta.AssistantOutput), index, delta.TraceID),
		})
	}
	return out
}

func buildNowledgeThreadTitle(delta ThreadDelta) string {
	for _, message := range delta.Messages {
		if strings.TrimSpace(message.Content) == "" || message.Role != "user" {
			continue
		}
		return compactInline(message.Content, 80)
	}
	if strings.TrimSpace(delta.AssistantOutput) != "" {
		return compactInline(delta.AssistantOutput, 80)
	}
	return "Arkloop " + delta.ThreadID.String()
}

func buildNowledgeAppendIdempotencyKey(delta ThreadDelta, messages []nowledge.ThreadMessage) string {
	externalIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.Metadata == nil {
			continue
		}
		if externalID, ok := message.Metadata["external_id"].(string); ok && strings.TrimSpace(externalID) != "" {
			externalIDs = append(externalIDs, strings.TrimSpace(externalID))
		}
	}
	sum := sha1.Sum([]byte(strings.Join([]string{
		delta.ThreadID.String(),
		delta.RunID.String(),
		strings.Join(externalIDs, "|"),
	}, "::")))
	return "ark-batch:" + hex.EncodeToString(sum[:])
}

func buildNowledgeConversation(delta ThreadDelta) string {
	lines := make([]string, 0, len(delta.Messages)+1)
	for _, message := range delta.Messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		lines = append(lines, message.Role+": "+content)
	}
	if strings.TrimSpace(delta.AssistantOutput) != "" {
		lines = append(lines, "assistant: "+strings.TrimSpace(delta.AssistantOutput))
	}
	return strings.Join(lines, "\n\n")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func buildNowledgeGuidanceText(workingMemoryInjected, recalledInjected bool) string {
	lines := []string{
		"你可以访问用户的个人知识图谱（Nowledge Mem）。",
	}
	if workingMemoryInjected || recalledInjected {
		injected := make([]string, 0, 2)
		if workingMemoryInjected {
			injected = append(injected, "Working Memory")
		}
		if recalledInjected {
			injected = append(injected, "相关记忆")
		}
		lines = append(lines,
			"本轮 prompt 已注入 "+strings.Join(injected, "和")+"；先利用已注入内容回答，只有需要更具体、更新或更广的上下文时再调用 memory_search。",
		)
	} else {
		lines = append(lines,
			"当问题涉及过往工作、决策、日期、人物、偏好、计划或历史上下文时，主动先用 memory_search 做语义检索，不要等用户点名要求。",
		)
	}
	lines = append(lines,
		"当 memory_search 返回 source_thread_id 时，使用 memory_thread_fetch 读取完整来源对话。",
		"当你需要跨主题关系、知识演化、来源文档或图谱邻居时，使用 memory_connections。",
		"当你需要按时间回顾近期活动、决策或知识变化时，使用 memory_timeline。",
		"当对话形成决策、偏好、计划、流程或经验时，主动使用 memory_write 保存，而不是假设这些内容会自动长期保留。",
	)
	return strings.Join(lines, "\n")
}
