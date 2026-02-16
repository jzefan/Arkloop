package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"arkloop/services/worker_go/internal/app"
	"arkloop/services/worker_go/internal/queue"
)

const defaultBridgeRequestTimeout = 30 * time.Minute

type PyBridgeHTTPHandler struct {
	endpoint string
	token    string
	client   *http.Client
	logger   *app.JSONLogger
}

func NewPyBridgeHTTPHandler(bridgeURL string, token string, logger *app.JSONLogger) (*PyBridgeHTTPHandler, error) {
	cleanedURL := strings.TrimRight(strings.TrimSpace(bridgeURL), "/")
	if cleanedURL == "" {
		return nil, fmt.Errorf("bridge_url 不能为空")
	}
	cleanedToken := strings.TrimSpace(token)
	if cleanedToken == "" {
		return nil, fmt.Errorf("bridge_token 不能为空")
	}
	if logger == nil {
		logger = app.NewJSONLogger("worker_go", nil)
	}

	return &PyBridgeHTTPHandler{
		endpoint: cleanedURL + "/internal/bridge/execute-run",
		token:    cleanedToken,
		client:   &http.Client{Timeout: defaultBridgeRequestTimeout},
		logger:   logger,
	}, nil
}

func (h *PyBridgeHTTPHandler) Handle(ctx context.Context, lease queue.JobLease) error {
	fields := fieldsFromLease(lease)
	payload := map[string]any{"payload_json": lease.PayloadJSON}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+h.token)

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		h.logger.Info("bridge 返回不可处理状态，已跳过", fields, map[string]any{"status_code": resp.StatusCode})
		return nil
	}

	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	h.logger.Error(
		"bridge 返回错误状态码",
		fields,
		map[string]any{"status_code": resp.StatusCode, "body": string(snippet)},
	)
	return fmt.Errorf("bridge returned status_code=%d", resp.StatusCode)
}

func fieldsFromLease(lease queue.JobLease) app.LogFields {
	fields := app.LogFields{JobID: stringPtr(lease.JobID.String())}
	if value := stringValue(lease.PayloadJSON, "trace_id"); value != "" {
		fields.TraceID = stringPtr(value)
	}
	if value := stringValue(lease.PayloadJSON, "org_id"); value != "" {
		fields.OrgID = stringPtr(value)
	}
	if value := stringValue(lease.PayloadJSON, "run_id"); value != "" {
		fields.RunID = stringPtr(value)
	}
	return fields
}

func stringValue(values map[string]any, key string) string {
	raw, ok := values[key]
	if !ok {
		return ""
	}
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return text
}

func stringPtr(value string) *string {
	copied := value
	return &copied
}
