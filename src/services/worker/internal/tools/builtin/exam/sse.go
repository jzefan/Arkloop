package exam

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const sseReadTimeoutSeconds = 90

// sseGenerate consumes the SSE response of /api/questions/ai-generate/stream.
// Returns each event as a map[string]any (parsed from the data: line).
// Stops on `event: done` or `event: error` lines, or when the server closes.
func (e *ToolExecutor) sseGenerate(
	ctx context.Context,
	userID uuid.UUID,
	scopes []string,
	body map[string]any,
) ([]map[string]any, error) {
	token, err := e.client.issueUserToken(ctx, userID, scopes)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, sseReadTimeoutSeconds*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		e.client.examBaseURL+"/api/questions/ai-generate/stream", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	// SSE clients need a longer body-read timeout than the standard client
	// configured on c.httpClient (30s). Use a dedicated transport here.
	sseClient := &http.Client{Timeout: 0} // explicit: rely on the request context
	resp, err := sseClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open sse: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("sse status %d: %s", resp.StatusCode, string(buf))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var events []map[string]any
	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			currentEvent = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				continue
			}
			var ev map[string]any
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				// Non-JSON event; surface as raw payload so caller can debug.
				ev = map[string]any{"raw": data}
			}
			if currentEvent != "" {
				ev["_event"] = currentEvent
			}
			events = append(events, ev)
			if currentEvent == "done" || currentEvent == "error" {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil && !isBenignSSEEOF(err) {
		return events, fmt.Errorf("sse read: %w", err)
	}
	return events, nil
}

func isBenignSSEEOF(err error) bool {
	if err == io.EOF {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "EOF") || strings.Contains(msg, "use of closed network connection")
}
