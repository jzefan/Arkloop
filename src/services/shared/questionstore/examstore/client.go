// Package examstore implements questionstore.QuestionStore against the exam
// backend's REST API. It calls 4 endpoints (knowledge-points, questions,
// questions/batch, papers) using the teacher's OIDC token.
package examstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is the low-level HTTP caller for exam backend endpoints.
type Client struct {
	baseURL    string
	httpClient *http.Client
	sema       chan struct{}
	retry      RetryPolicy
}

type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		sema:       make(chan struct{}, 4),
		retry:      RetryPolicy{MaxAttempts: 3, BaseDelay: 250 * time.Millisecond, MaxDelay: 5 * time.Second},
	}
}

func (c *Client) BaseURL() string { return c.baseURL }

func (c *Client) doJSON(ctx context.Context, method, path, token string, body any, dst any) error {
	select {
	case c.sema <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.sema }()

	var lastErr error
	for attempt := 1; attempt <= c.retry.MaxAttempts; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("marshal: %w", err)
			}
			bodyReader = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-ArkLoop-API-Version", "1")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < c.retry.MaxAttempts {
				sleepBackoff(ctx, c.retry, attempt)
				continue
			}
			return fmt.Errorf("http: %w", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		switch {
		case resp.StatusCode >= 500:
			lastErr = &ServerError{Status: resp.StatusCode, Body: string(respBody)}
			if attempt < c.retry.MaxAttempts {
				sleepBackoff(ctx, c.retry, attempt)
				continue
			}
			return lastErr
		case resp.StatusCode == 401 || resp.StatusCode == 403:
			return &AuthError{Status: resp.StatusCode, Body: string(respBody)}
		case resp.StatusCode >= 400:
			return &ClientError{Status: resp.StatusCode, Body: string(respBody)}
		}
		if dst != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, dst); err != nil {
				return fmt.Errorf("unmarshal: %w", err)
			}
		}
		return nil
	}
	return lastErr
}

// ListKnowledgePoints calls GET /api/knowledge-points?course_id=...
func (c *Client) ListKnowledgePoints(ctx context.Context, token, courseID string, limit, offset int) (*KPListResp, error) {
	qs := url.Values{}
	qs.Set("course_id", courseID)
	if limit > 0 {
		qs.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		qs.Set("offset", strconv.Itoa(offset))
	}
	var resp KPListResp
	err := c.doJSON(ctx, "GET", "/api/knowledge-points?"+qs.Encode(), token, nil, &resp)
	return &resp, err
}

// ListQuestions calls GET /api/questions?...
func (c *Client) ListQuestions(ctx context.Context, token, kpID string, filter ListFilter) (*QListResp, error) {
	qs := url.Values{}
	qs.Set("knowledge_point_id", kpID)
	if filter.Type != "" {
		qs.Set("type", filter.Type)
	}
	if filter.Difficulty != "" {
		qs.Set("difficulty", filter.Difficulty)
	}
	if filter.PatternTag != "" {
		qs.Set("pattern_tag", filter.PatternTag)
	}
	if filter.Limit > 0 {
		qs.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Offset > 0 {
		qs.Set("offset", strconv.Itoa(filter.Offset))
	}
	var resp QListResp
	err := c.doJSON(ctx, "GET", "/api/questions?"+qs.Encode(), token, nil, &resp)
	return &resp, err
}

// CreateQuestionsBatch calls POST /api/questions/batch
func (c *Client) CreateQuestionsBatch(ctx context.Context, token string, drafts []DraftReq) (*BatchResp, error) {
	var resp BatchResp
	err := c.doJSON(ctx, "POST", "/api/questions/batch", token,
		map[string]any{"questions": drafts}, &resp)
	return &resp, err
}

// CreatePaper calls POST /api/papers
func (c *Client) CreatePaper(ctx context.Context, token string, req PaperReq) (*PaperResp, error) {
	var resp PaperResp
	err := c.doJSON(ctx, "POST", "/api/papers", token, req, &resp)
	return &resp, err
}

// ListCourses calls GET /api/courses
func (c *Client) ListCourses(ctx context.Context, token string) (*CourseListResp, error) {
	var resp CourseListResp
	err := c.doJSON(ctx, "GET", "/api/courses", token, nil, &resp)
	return &resp, err
}

// --- errors ---

type ServerError struct{ Status int; Body string }

func (e *ServerError) Error() string { return fmt.Sprintf("examstore: server %d: %s", e.Status, truncate(e.Body, 200)) }

type ClientError struct{ Status int; Body string }

func (e *ClientError) Error() string { return fmt.Sprintf("examstore: client %d: %s", e.Status, truncate(e.Body, 200)) }

type AuthError struct{ Status int; Body string }

func (e *AuthError) Error() string { return fmt.Sprintf("examstore: auth %d: %s", e.Status, truncate(e.Body, 200)) }

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func sleepBackoff(ctx context.Context, p RetryPolicy, attempt int) {
	d := p.BaseDelay * time.Duration(1<<(attempt-1))
	if d > p.MaxDelay {
		d = p.MaxDelay
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
