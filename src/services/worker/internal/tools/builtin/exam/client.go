// Package exam implements the worker-side tools that let the exam-agent
// persona drive the exam backend on behalf of the current ArkLoop user.
//
// Auth flow: worker holds a long-lived service token. For each tool call
// it asks ArkLoop's internal endpoint to mint a 60-second exam access_token
// scoped to the current user, then makes the actual exam API call with
// that token. The token is never persisted.
package exam

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	envExamBaseURL      = "EXAM_BASE_URL"
	envArkLoopAPI       = "ARKLOOP_API_INTERNAL_URL"
	envServiceToken     = "ARKLOOP_INTERNAL_SERVICE_TOKEN"
	defaultArkLoopAPI   = "http://api:19001"
	tokenIssueTimeoutMs = 5000
	examCallTimeoutMs   = 30000
)

// Client is constructed once at worker startup. It does no caching of tokens
// (their TTL is 60s; over-engineering caching for a high-stakes path isn't
// worth it).
type Client struct {
	examBaseURL      string
	arkLoopBaseURL   string
	serviceToken     string
	httpClient       *http.Client
	clientID         string
}

// NewClient returns a Client wired from env vars. Returns (nil, error) when
// any required env var is missing — caller should mark the tool as
// NotConfigured.
func NewClient() (*Client, error) {
	examBase := strings.TrimSpace(os.Getenv(envExamBaseURL))
	serviceTok := strings.TrimSpace(os.Getenv(envServiceToken))
	if examBase == "" {
		return nil, fmt.Errorf("%s not set", envExamBaseURL)
	}
	if serviceTok == "" {
		return nil, fmt.Errorf("%s not set", envServiceToken)
	}
	arkBase := strings.TrimSpace(os.Getenv(envArkLoopAPI))
	if arkBase == "" {
		arkBase = defaultArkLoopAPI
	}
	return &Client{
		examBaseURL:    strings.TrimRight(examBase, "/"),
		arkLoopBaseURL: strings.TrimRight(arkBase, "/"),
		serviceToken:   serviceTok,
		clientID:       "exam-web",
		httpClient: &http.Client{
			Timeout: time.Duration(examCallTimeoutMs) * time.Millisecond,
		},
	}, nil
}

// issueUserToken mints a short-lived (60s) exam access_token for userID.
// Always re-mints — no caching, because TTL is shorter than most tool runs.
func (c *Client) issueUserToken(ctx context.Context, userID uuid.UUID, scopes []string) (string, error) {
	if userID == uuid.Nil {
		return "", errors.New("no current user (tool requires user context)")
	}
	body, _ := json.Marshal(map[string]any{
		"user_id":   userID.String(),
		"client_id": c.clientID,
		"scopes":    scopes,
	})

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(tokenIssueTimeoutMs)*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		c.arkLoopBaseURL+"/internal/oauth/issue", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.serviceToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call internal issue: %w", err)
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("internal issue %d: %s", resp.StatusCode, string(rawBody))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rawBody, &out); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("internal issue returned empty access_token")
	}
	return out.AccessToken, nil
}

// callExam invokes an exam-backend endpoint as userID. method/path/body are
// standard; result is unmarshaled into `out` if non-nil.
func (c *Client) callExam(
	ctx context.Context,
	userID uuid.UUID,
	scopes []string,
	method, path string,
	body any,
	out any,
) error {
	token, err := c.issueUserToken(ctx, userID, scopes)
	if err != nil {
		return err
	}

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.examBaseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call exam %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("exam %s %s status=%d body=%s", method, path, resp.StatusCode, truncate(string(rawBody), 500))
	}
	if out != nil && len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, out); err != nil {
			return fmt.Errorf("decode exam response: %w", err)
		}
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
