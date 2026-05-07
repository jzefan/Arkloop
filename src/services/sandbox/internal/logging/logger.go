package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

type LogFields struct {
	SessionID *string
}

type JSONLogger struct {
	component string
	writer    io.Writer
	now       func() time.Time
	mu        sync.Mutex
}

func NewJSONLogger(component string, writer io.Writer) *JSONLogger {
	if writer == nil {
		writer = io.Discard
	}
	if component == "" {
		component = "sandbox"
	}
	return &JSONLogger{
		component: component,
		writer:    writer,
		now:       time.Now,
	}
}

func (l *JSONLogger) Info(msg string, fields LogFields, extra map[string]any) {
	l.log("info", msg, fields, extra)
}

func (l *JSONLogger) Warn(msg string, fields LogFields, extra map[string]any) {
	l.log("warn", msg, fields, extra)
}

func (l *JSONLogger) Error(msg string, fields LogFields, extra map[string]any) {
	l.log("error", msg, fields, extra)
}

func (l *JSONLogger) log(level, msg string, fields LogFields, extra map[string]any) {
	record := map[string]any{
		"ts":         formatTimestamp(l.now()),
		"level":      level,
		"msg":        msg,
		"component":  l.component,
		"session_id": ptrStr(fields.SessionID),
	}
	for k, v := range extra {
		record[k] = v
	}
	payload, err := json.Marshal(record)
	if err != nil {
		payload = []byte(fmt.Sprintf(`{"level":"error","msg":"marshal failed","component":"%s"}`, l.component))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.writer.Write(payload)
	_, _ = l.writer.Write([]byte("\n"))
}

func formatTimestamp(t time.Time) string {
	s := t.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	if len(s) >= 6 && s[len(s)-6:] == "+00:00" {
		return s[:len(s)-6] + "Z"
	}
	return s
}

func ptrStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
