package nowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	sharedoutbound "arkloop/services/shared/outboundurl"
	"arkloop/services/worker/internal/memory"
)

const (
	memoryURIPrefix = "nowledge://memory/"
	threadURIPrefix = "nowledge://thread/"
	defaultSource   = "arkloop"
)

type WorkingMemory struct {
	Content   string
	Available bool
}

type SearchResult struct {
	ID              string
	Title           string
	Content         string
	Score           float64
	Importance      float64
	RelevanceReason string
	Labels          []string
	SourceThreadID  string
}

type ListedMemory struct {
	ID         string
	Title      string
	Content    string
	Rating     float64
	Time       string
	LabelIDs   []string
	IsFavorite bool
	Confidence float64
	Source     string
}

type ThreadMessage struct {
	Role      string
	Content   string
	Timestamp string
	Metadata  map[string]any
}

type ThreadSearchResult struct {
	ThreadID       string
	Title          string
	Source         string
	MessageCount   int
	Score          float64
	Snippets       []string
	MatchedSnippet string
}

type ThreadFetchResult struct {
	ThreadID     string
	Title        string
	Source       string
	MessageCount int
	Messages     []ThreadMessage
}

type TriageResult struct {
	ShouldDistill bool
	Reason        string
}

type DistillResult struct {
	MemoriesCreated int
}

type ThreadAppender interface {
	CreateThread(ctx context.Context, ident memory.MemoryIdentity, threadID, title string, messages []ThreadMessage) (string, error)
	AppendThread(ctx context.Context, ident memory.MemoryIdentity, threadID string, messages []ThreadMessage, idempotencyKey string) (int, error)
}

type ThreadReader interface {
	SearchThreads(ctx context.Context, ident memory.MemoryIdentity, query string, limit int) (map[string]any, error)
	FetchThread(ctx context.Context, ident memory.MemoryIdentity, threadID string, offset, limit int) (map[string]any, error)
}

type ContextReader interface {
	ReadWorkingMemory(ctx context.Context, ident memory.MemoryIdentity) (WorkingMemory, error)
	SearchRich(ctx context.Context, ident memory.MemoryIdentity, query string, limit int) ([]SearchResult, error)
}

type Distiller interface {
	TriageConversation(ctx context.Context, ident memory.MemoryIdentity, content string) (TriageResult, error)
	DistillThread(ctx context.Context, ident memory.MemoryIdentity, threadID, title, content string) (DistillResult, error)
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(cfg Config) *Client {
	baseURL, err := sharedoutbound.DefaultPolicy().NormalizeBaseURL(strings.TrimSpace(cfg.BaseURL))
	if err != nil {
		baseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		http:    sharedoutbound.DefaultPolicy().NewHTTPClient(time.Duration(cfg.resolvedTimeoutMs()) * time.Millisecond),
	}
}

func (c *Client) Find(ctx context.Context, ident memory.MemoryIdentity, _ string, query string, limit int) ([]memory.MemoryHit, error) {
	results, err := c.SearchRich(ctx, ident, query, limit)
	if err != nil {
		return nil, err
	}
	hits := make([]memory.MemoryHit, 0, len(results))
	for _, result := range results {
		hits = append(hits, memory.MemoryHit{
			URI:         memoryURIPrefix + result.ID,
			Abstract:    firstNonEmpty(result.Title, result.Content),
			Score:       result.Score,
			MatchReason: result.RelevanceReason,
			IsLeaf:      true,
		})
	}
	return hits, nil
}

func (c *Client) Content(ctx context.Context, ident memory.MemoryIdentity, uri string, layer memory.MemoryLayer) (string, error) {
	memoryID, err := parseMemoryURI(uri)
	if err != nil {
		return "", err
	}
	var response struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.doJSON(ctx, ident, http.MethodGet, "/memories/"+url.PathEscape(memoryID), nil, &response); err != nil {
		return "", err
	}
	title := strings.TrimSpace(response.Title)
	content := strings.TrimSpace(response.Content)
	if title == "" {
		return content, nil
	}
	switch layer {
	case memory.MemoryLayerAbstract:
		if content == "" {
			return title, nil
		}
		return title + "\n" + content, nil
	default:
		if content == "" {
			return title, nil
		}
		return title + "\n\n" + content, nil
	}
}

func (c *Client) ListDir(context.Context, memory.MemoryIdentity, string) ([]string, error) {
	return nil, nil
}

func (c *Client) ListMemories(ctx context.Context, ident memory.MemoryIdentity, limit int) ([]ListedMemory, error) {
	if limit <= 0 {
		limit = 30
	}
	values := url.Values{}
	values.Set("limit", fmt.Sprintf("%d", limit))

	var response struct {
		Memories []struct {
			ID         string   `json:"id"`
			Title      string   `json:"title"`
			Content    string   `json:"content"`
			Rating     float64  `json:"rating"`
			Time       string   `json:"time"`
			LabelIDs   []string `json:"label_ids"`
			IsFavorite bool     `json:"is_favorite"`
			Confidence float64  `json:"confidence"`
			Source     string   `json:"source"`
		} `json:"memories"`
	}
	if err := c.doJSON(ctx, ident, http.MethodGet, "/memories?"+values.Encode(), nil, &response); err != nil {
		return nil, err
	}

	out := make([]ListedMemory, 0, len(response.Memories))
	for _, item := range response.Memories {
		out = append(out, ListedMemory{
			ID:         strings.TrimSpace(item.ID),
			Title:      strings.TrimSpace(item.Title),
			Content:    strings.TrimSpace(item.Content),
			Rating:     item.Rating,
			Time:       strings.TrimSpace(item.Time),
			LabelIDs:   append([]string(nil), item.LabelIDs...),
			IsFavorite: item.IsFavorite,
			Confidence: item.Confidence,
			Source:     strings.TrimSpace(item.Source),
		})
	}
	return out, nil
}

func (c *Client) ListFragments(ctx context.Context, ident memory.MemoryIdentity, limit int) ([]memory.MemoryFragment, error) {
	listed, err := c.ListMemories(ctx, ident, limit)
	if err != nil {
		return nil, err
	}
	fragments := make([]memory.MemoryFragment, 0, len(listed))
	for _, item := range listed {
		score := item.Confidence
		if score == 0 {
			score = item.Rating
		}
		fragments = append(fragments, memory.MemoryFragment{
			ID:          item.ID,
			URI:         memoryURIPrefix + item.ID,
			Title:       item.Title,
			Content:     item.Content,
			Abstract:    firstNonEmpty(item.Title, compactContent(item.Content, 160)),
			Score:       score,
			Labels:      append([]string(nil), item.LabelIDs...),
			RecordedAt:  item.Time,
			IsEphemeral: false,
		})
	}
	return fragments, nil
}

func (c *Client) AppendSessionMessages(context.Context, memory.MemoryIdentity, string, []memory.MemoryMessage) error {
	return nil
}

func (c *Client) CommitSession(context.Context, memory.MemoryIdentity, string) error {
	return nil
}

func (c *Client) Write(ctx context.Context, ident memory.MemoryIdentity, _ memory.MemoryScope, entry memory.MemoryEntry) error {
	_, err := c.WriteReturningURI(ctx, ident, memory.MemoryScopeUser, entry)
	return err
}

func (c *Client) WriteReturningURI(ctx context.Context, ident memory.MemoryIdentity, _ memory.MemoryScope, entry memory.MemoryEntry) (string, error) {
	content, metadata := parseWritableEntry(entry.Content)
	if content == "" {
		return "", fmt.Errorf("nowledge write: content is empty")
	}
	body := map[string]any{
		"content": content,
	}
	if metadata.Title != "" {
		body["title"] = metadata.Title
	}
	if metadata.UnitType != "" {
		body["unit_type"] = metadata.UnitType
	}
	if len(metadata.Labels) > 0 {
		body["labels"] = metadata.Labels
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, ident, http.MethodPost, "/memories", body, &response); err != nil {
		return "", err
	}
	if response.ID == "" {
		return "", nil
	}
	return memoryURIPrefix + response.ID, nil
}

func (c *Client) Delete(ctx context.Context, ident memory.MemoryIdentity, uri string) error {
	memoryID, err := parseMemoryURI(uri)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, ident, http.MethodDelete, "/memories/"+url.PathEscape(memoryID), nil, nil)
}

func (c *Client) ReadWorkingMemory(ctx context.Context, ident memory.MemoryIdentity) (WorkingMemory, error) {
	var response struct {
		Content string `json:"content"`
		Exists  bool   `json:"exists"`
	}
	if err := c.doJSON(ctx, ident, http.MethodGet, "/agent/working-memory", nil, &response); err != nil {
		return WorkingMemory{}, err
	}
	content := strings.TrimSpace(response.Content)
	return WorkingMemory{
		Content:   content,
		Available: response.Exists || content != "",
	}, nil
}

func (c *Client) SearchRich(ctx context.Context, ident memory.MemoryIdentity, query string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	values := url.Values{}
	values.Set("query", strings.TrimSpace(query))
	values.Set("limit", fmt.Sprintf("%d", limit))

	var response struct {
		Memories []struct {
			ID           string   `json:"id"`
			Title        string   `json:"title"`
			Content      string   `json:"content"`
			Confidence   float64  `json:"confidence"`
			Score        float64  `json:"score"`
			LabelIDs     []string `json:"label_ids"`
			Labels       []string `json:"labels"`
			SourceThread any      `json:"source_thread"`
			Metadata     struct {
				Importance      float64 `json:"importance"`
				SimilarityScore float64 `json:"similarity_score"`
				RelevanceReason string  `json:"relevance_reason"`
				SourceThreadID  string  `json:"source_thread_id"`
			} `json:"metadata"`
			RelevanceReason string `json:"relevance_reason"`
		} `json:"memories"`
	}
	if err := c.doJSON(ctx, ident, http.MethodGet, "/memories/search?"+values.Encode(), nil, &response); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(response.Memories))
	for _, item := range response.Memories {
		sourceThreadID := item.Metadata.SourceThreadID
		if sourceThreadID == "" {
			switch value := item.SourceThread.(type) {
			case string:
				sourceThreadID = strings.TrimSpace(value)
			case map[string]any:
				if rawID, ok := value["id"].(string); ok {
					sourceThreadID = strings.TrimSpace(rawID)
				}
			}
		}
		score := item.Score
		if score == 0 {
			score = item.Confidence
		}
		if score == 0 {
			score = item.Metadata.SimilarityScore
		}
		labels := append([]string(nil), item.Labels...)
		if len(labels) == 0 {
			labels = append(labels, item.LabelIDs...)
		}
		results = append(results, SearchResult{
			ID:              strings.TrimSpace(item.ID),
			Title:           strings.TrimSpace(item.Title),
			Content:         strings.TrimSpace(item.Content),
			Score:           score,
			Importance:      item.Metadata.Importance,
			RelevanceReason: firstNonEmpty(item.RelevanceReason, item.Metadata.RelevanceReason),
			Labels:          labels,
			SourceThreadID:  sourceThreadID,
		})
	}
	return results, nil
}

func (c *Client) CreateThread(ctx context.Context, ident memory.MemoryIdentity, threadID, title string, messages []ThreadMessage) (string, error) {
	body := map[string]any{
		"title":    strings.TrimSpace(title),
		"source":   defaultSource,
		"messages": messages,
	}
	if strings.TrimSpace(threadID) != "" {
		body["thread_id"] = strings.TrimSpace(threadID)
	}
	var response struct {
		ID       string `json:"id"`
		ThreadID string `json:"thread_id"`
		Thread   struct {
			ThreadID string `json:"thread_id"`
		} `json:"thread"`
	}
	if err := c.doJSON(ctx, ident, http.MethodPost, "/threads", body, &response); err != nil {
		return "", err
	}
	return firstNonEmpty(response.ID, response.ThreadID, response.Thread.ThreadID, strings.TrimSpace(threadID)), nil
}

func (c *Client) AppendThread(ctx context.Context, ident memory.MemoryIdentity, threadID string, messages []ThreadMessage, idempotencyKey string) (int, error) {
	body := map[string]any{
		"messages":    messages,
		"deduplicate": true,
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		body["idempotency_key"] = strings.TrimSpace(idempotencyKey)
	}
	var response struct {
		MessagesAdded int `json:"messages_added"`
	}
	if err := c.doJSON(ctx, ident, http.MethodPost, "/threads/"+url.PathEscape(strings.TrimSpace(threadID))+"/append", body, &response); err != nil {
		return 0, err
	}
	return response.MessagesAdded, nil
}

func (c *Client) SearchThreads(ctx context.Context, ident memory.MemoryIdentity, query string, limit int) (map[string]any, error) {
	results, err := c.searchThreadsResult(ctx, ident, query, limit)
	if err != nil {
		return nil, err
	}
	threads := make([]map[string]any, 0, len(results))
	for _, item := range results {
		threads = append(threads, map[string]any{
			"thread_id":       item.ThreadID,
			"title":           item.Title,
			"source":          item.Source,
			"message_count":   item.MessageCount,
			"score":           item.Score,
			"matched_snippet": item.MatchedSnippet,
			"snippets":        item.Snippets,
		})
	}
	return map[string]any{"threads": threads, "total_found": len(threads)}, nil
}

func (c *Client) searchThreadsResult(ctx context.Context, ident memory.MemoryIdentity, query string, limit int) ([]ThreadSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	values := url.Values{}
	values.Set("query", strings.TrimSpace(query))
	values.Set("mode", "full")
	values.Set("limit", fmt.Sprintf("%d", limit))
	var response struct {
		Threads []struct {
			ID           string   `json:"id"`
			ThreadID     string   `json:"thread_id"`
			Title        string   `json:"title"`
			Source       string   `json:"source"`
			MessageCount int      `json:"message_count"`
			Score        float64  `json:"score"`
			Snippets     []string `json:"snippets"`
			Matches      []struct {
				Snippet string `json:"snippet"`
			} `json:"matches"`
		} `json:"threads"`
	}
	if err := c.doJSON(ctx, ident, http.MethodGet, "/threads/search?"+values.Encode(), nil, &response); err != nil {
		return nil, err
	}
	results := make([]ThreadSearchResult, 0, len(response.Threads))
	for _, item := range response.Threads {
		snippets := append([]string(nil), item.Snippets...)
		if len(snippets) == 0 {
			for _, match := range item.Matches {
				if snippet := strings.TrimSpace(match.Snippet); snippet != "" {
					snippets = append(snippets, snippet)
				}
			}
		}
		results = append(results, ThreadSearchResult{
			ThreadID:       firstNonEmpty(item.ThreadID, item.ID),
			Title:          strings.TrimSpace(item.Title),
			Source:         strings.TrimSpace(item.Source),
			MessageCount:   item.MessageCount,
			Score:          item.Score,
			Snippets:       snippets,
			MatchedSnippet: firstNonEmpty(snippets...),
		})
	}
	return results, nil
}

func (c *Client) FetchThread(ctx context.Context, ident memory.MemoryIdentity, threadID string, offset, limit int) (map[string]any, error) {
	result, err := c.fetchThreadResult(ctx, ident, threadID, offset, limit)
	if err != nil {
		return nil, err
	}
	messages := make([]map[string]any, 0, len(result.Messages))
	for _, msg := range result.Messages {
		messages = append(messages, map[string]any{
			"role":      msg.Role,
			"content":   msg.Content,
			"timestamp": msg.Timestamp,
		})
	}
	return map[string]any{
		"thread_id":     result.ThreadID,
		"title":         result.Title,
		"source":        result.Source,
		"message_count": result.MessageCount,
		"messages":      messages,
	}, nil
}

func (c *Client) fetchThreadResult(ctx context.Context, ident memory.MemoryIdentity, threadID string, offset, limit int) (ThreadFetchResult, error) {
	values := url.Values{}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		values.Set("offset", fmt.Sprintf("%d", offset))
	}
	var response struct {
		ID           string `json:"id"`
		ThreadID     string `json:"thread_id"`
		Title        string `json:"title"`
		Source       string `json:"source"`
		MessageCount int    `json:"message_count"`
		Messages     []struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Timestamp string `json:"timestamp"`
			CreatedAt string `json:"created_at"`
		} `json:"messages"`
	}
	path := "/threads/" + url.PathEscape(strings.TrimSpace(threadID))
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if err := c.doJSON(ctx, ident, http.MethodGet, path, nil, &response); err != nil {
		return ThreadFetchResult{}, err
	}
	out := ThreadFetchResult{
		ThreadID:     firstNonEmpty(response.ThreadID, response.ID, strings.TrimSpace(threadID)),
		Title:        strings.TrimSpace(response.Title),
		Source:       strings.TrimSpace(response.Source),
		MessageCount: response.MessageCount,
		Messages:     make([]ThreadMessage, 0, len(response.Messages)),
	}
	for _, msg := range response.Messages {
		out.Messages = append(out.Messages, ThreadMessage{
			Role:      strings.TrimSpace(msg.Role),
			Content:   strings.TrimSpace(msg.Content),
			Timestamp: firstNonEmpty(msg.Timestamp, msg.CreatedAt),
		})
	}
	return out, nil
}

func (c *Client) TriageConversation(ctx context.Context, ident memory.MemoryIdentity, content string) (TriageResult, error) {
	var response struct {
		ShouldDistill bool   `json:"should_distill"`
		Reason        string `json:"reason"`
	}
	if err := c.doJSON(ctx, ident, http.MethodPost, "/memories/distill/triage", map[string]any{
		"thread_content": strings.TrimSpace(content),
	}, &response); err != nil {
		return TriageResult{}, err
	}
	return TriageResult{ShouldDistill: response.ShouldDistill, Reason: strings.TrimSpace(response.Reason)}, nil
}

func (c *Client) DistillThread(ctx context.Context, ident memory.MemoryIdentity, threadID, title, content string) (DistillResult, error) {
	var response struct {
		MemoriesCreated int   `json:"memories_created"`
		CreatedMemories []any `json:"created_memories"`
	}
	if err := c.doJSON(ctx, ident, http.MethodPost, "/memories/distill", map[string]any{
		"thread_id":         strings.TrimSpace(threadID),
		"thread_title":      strings.TrimSpace(title),
		"thread_content":    strings.TrimSpace(content),
		"distillation_type": "simple_llm",
		"extraction_level":  "swift",
	}, &response); err != nil {
		return DistillResult{}, err
	}
	count := response.MemoriesCreated
	if count == 0 {
		count = len(response.CreatedMemories)
	}
	return DistillResult{MemoriesCreated: count}, nil
}

func (c *Client) doJSON(ctx context.Context, ident memory.MemoryIdentity, method, path string, body any, out any) error {
	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("nowledge marshal request: %w", err)
		}
		requestBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, requestBody)
	if err != nil {
		return fmt.Errorf("nowledge build request: %w", err)
	}
	c.setHeaders(req, ident)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("nowledge %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("nowledge %s %s: status=%d body=%s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("nowledge decode response: %w", err)
	}
	return nil
}

func (c *Client) setHeaders(req *http.Request, ident memory.MemoryIdentity) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("x-nmem-api-key", c.apiKey)
	}
	req.Header.Set("X-Arkloop-Account", ident.AccountID.String())
	req.Header.Set("X-Arkloop-User", ident.UserID.String())
	req.Header.Set("X-Arkloop-Agent", sanitizeAgentID(ident.AgentID))
	if strings.TrimSpace(ident.ExternalUserID) != "" {
		req.Header.Set("X-Arkloop-External-User", strings.TrimSpace(ident.ExternalUserID))
	}
}

func parseMemoryURI(uri string) (string, error) {
	value := strings.TrimSpace(uri)
	if !strings.HasPrefix(value, memoryURIPrefix) {
		return "", fmt.Errorf("invalid nowledge memory uri: %q", uri)
	}
	memoryID := strings.TrimSpace(strings.TrimPrefix(value, memoryURIPrefix))
	if memoryID == "" {
		return "", fmt.Errorf("invalid nowledge memory uri: %q", uri)
	}
	return memoryID, nil
}

func MemoryIDFromURI(uri string) (string, error) {
	return parseMemoryURI(uri)
}

type writableMetadata struct {
	Title    string
	UnitType string
	Labels   []string
}

func parseWritableEntry(raw string) (string, writableMetadata) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", writableMetadata{}
	}
	meta := writableMetadata{UnitType: "context"}
	if strings.HasPrefix(text, "[") {
		if end := strings.Index(text, "] "); end > 1 {
			header := strings.TrimSpace(text[1:end])
			parts := strings.Split(header, "/")
			if len(parts) >= 3 {
				category := strings.TrimSpace(parts[1])
				key := strings.TrimSpace(parts[2])
				meta.Title = key
				if category != "" {
					meta.Labels = append(meta.Labels, category)
					meta.UnitType = categoryToUnitType(category)
				}
			}
			text = strings.TrimSpace(text[end+2:])
		}
	}
	if meta.Title == "" {
		meta.Title = firstLine(text)
	}
	return text, meta
}

func categoryToUnitType(category string) string {
	switch strings.TrimSpace(strings.ToLower(category)) {
	case "preferences", "profile":
		return "preference"
	case "events":
		return "event"
	case "cases":
		return "procedure"
	case "patterns":
		return "learning"
	case "entities":
		return "context"
	default:
		return "context"
	}
}

func sanitizeAgentID(value string) string {
	var builder strings.Builder
	for _, ch := range strings.TrimSpace(value) {
		switch {
		case ch >= 'a' && ch <= 'z':
			builder.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			builder.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			builder.WriteRune(ch)
		case ch == '-' || ch == '_':
			builder.WriteRune(ch)
		default:
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	if len(text) > 80 {
		return text[:80]
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func compactContent(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
