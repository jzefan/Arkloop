// Package exam implements the worker-side tools that let the exam-agent
// persona drive the exam backend on behalf of the current ArkLoop user.
//
// Auth modes:
//   - Per-user OIDC token: worker holds a long-lived service token. For each
//     tool call it asks ArkLoop's internal endpoint to mint a 60s exam
//     access_token scoped to the current user, then makes the actual exam API
//     call with that token. Used by the exam-agent personal-data tools.
//   - Admin token (CallExamAsAdmin): for read-only access to exam's
//     platform-administrator question bank (e.g. 国考医学题库) the worker
//     logs into exam directly with EXAM_ADMIN_USERNAME / EXAM_ADMIN_PASSWORD
//     and caches the resulting bearer token. This is how the linked-KB
//     paper-builder pulls reference questions and the knowledge-point tree
//     that ordinary teacher accounts cannot see.
//
// Tokens are never persisted to disk.
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
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	envExamBaseURL       = "EXAM_BASE_URL"
	envArkLoopAPI        = "ARKLOOP_API_INTERNAL_URL"
	envServiceToken      = "ARKLOOP_INTERNAL_SERVICE_TOKEN"
	envExamAdminUsername = "EXAM_ADMIN_USERNAME"
	envExamAdminPassword = "EXAM_ADMIN_PASSWORD"
	defaultArkLoopAPI    = "http://api:19001"
	tokenIssueTimeoutMs  = 5000
	examCallTimeoutMs    = 30000
	// adminTokenLifetime is the conservative reuse window for a cached admin
	// token. The exam backend's TokenResponse does not include an expires_in,
	// and the JWTs in dev-stack examples are valid for an hour, so 30 min is
	// safely under the smallest deployments we've seen and avoids logging in
	// before every read.
	adminTokenLifetime = 30 * time.Minute
)

// Client is constructed once at worker startup. Tokens are not persisted but
// the admin token is cached in-memory across calls (see adminTokenCache
// below).
type Client struct {
	examBaseURL    string
	arkLoopBaseURL string
	serviceToken   string
	httpClient     *http.Client
	clientID       string

	adminUser     string
	adminPassword string

	adminMu     sync.Mutex
	adminToken  string
	adminExpiry time.Time
}

// NewClient returns a Client wired from env vars. EXAM_BASE_URL and
// ARKLOOP_INTERNAL_SERVICE_TOKEN are required (per-user flow). Admin
// credentials (EXAM_ADMIN_USERNAME / EXAM_ADMIN_PASSWORD) are optional;
// when missing, CallExamAsAdmin returns ErrAdminNotConfigured.
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
		adminUser:      strings.TrimSpace(os.Getenv(envExamAdminUsername)),
		adminPassword:  os.Getenv(envExamAdminPassword),
		httpClient: &http.Client{
			Timeout: time.Duration(examCallTimeoutMs) * time.Millisecond,
		},
	}, nil
}

// ErrAdminNotConfigured signals the deployment has not provided admin
// credentials. Callers should surface a friendly "exam reference data
// unavailable" message to the teacher rather than expose this internally.
var ErrAdminNotConfigured = errors.New("exam admin credentials (EXAM_ADMIN_USERNAME / EXAM_ADMIN_PASSWORD) not configured")

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

// CallExam invokes an exam-backend endpoint as userID. It is exported for
// provider-neutral worker tools that need to proxy exam-backed operations
// without exposing exam terminology in their own package API.
func (c *Client) CallExam(
	ctx context.Context,
	userID uuid.UUID,
	scopes []string,
	method, path string,
	body any,
	out any,
) error {
	return c.callExam(ctx, userID, scopes, method, path, body, out)
}

// CallExamAsAdmin invokes an exam-backend endpoint with a cached admin
// bearer token, refreshing it as needed. Used by linked-mode read paths
// (e.g. national medical question bank) that ordinary teacher accounts
// cannot see.
//
// On 401 the cached token is invalidated and one retry with a freshly
// minted token is attempted before failing.
func (c *Client) CallExamAsAdmin(
	ctx context.Context,
	method, path string,
	body any,
	out any,
) error {
	if c == nil {
		return errors.New("nil exam client")
	}
	if strings.TrimSpace(c.adminUser) == "" || c.adminPassword == "" {
		return ErrAdminNotConfigured
	}
	doCall := func(token string) (int, error) {
		var reqBody io.Reader
		if body != nil {
			buf, err := json.Marshal(body)
			if err != nil {
				return 0, err
			}
			reqBody = bytes.NewReader(buf)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.examBaseURL+path, reqBody)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, fmt.Errorf("call exam %s %s: %w", method, path, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusUnauthorized {
			return resp.StatusCode, nil
		}
		if resp.StatusCode >= 400 {
			return resp.StatusCode, fmt.Errorf("exam %s %s status=%d body=%s", method, path, resp.StatusCode, truncate(string(raw), 500))
		}
		if out != nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, out); err != nil {
				return resp.StatusCode, fmt.Errorf("decode exam response: %w", err)
			}
		}
		return resp.StatusCode, nil
	}

	token, err := c.getAdminToken(ctx, false)
	if err != nil {
		return err
	}
	status, err := doCall(token)
	if err != nil {
		return err
	}
	if status != http.StatusUnauthorized {
		return nil
	}
	// Token was rejected — discard and retry once with a freshly minted one.
	token, err = c.getAdminToken(ctx, true)
	if err != nil {
		return err
	}
	if status, err := doCall(token); err != nil {
		return err
	} else if status == http.StatusUnauthorized {
		return fmt.Errorf("exam %s %s status=401 even after admin re-login", method, path)
	}
	return nil
}

// getAdminToken returns a non-expired admin bearer token, logging in to
// exam if the cache is empty/stale or force=true.
func (c *Client) getAdminToken(ctx context.Context, force bool) (string, error) {
	c.adminMu.Lock()
	defer c.adminMu.Unlock()
	if !force && c.adminToken != "" && time.Now().Before(c.adminExpiry) {
		return c.adminToken, nil
	}
	body, _ := json.Marshal(map[string]any{
		"username": c.adminUser,
		"password": c.adminPassword,
	})
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(tokenIssueTimeoutMs)*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		c.examBaseURL+"/api/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exam admin login: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("exam admin login status=%d body=%s", resp.StatusCode, truncate(string(raw), 200))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode admin login response: %w", err)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return "", errors.New("exam admin login returned empty access_token")
	}
	c.adminToken = out.AccessToken
	c.adminExpiry = time.Now().Add(adminTokenLifetime)
	return c.adminToken, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
