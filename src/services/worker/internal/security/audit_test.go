package security

import (
	"context"
	"testing"

	"arkloop/services/shared/plugin"

	"github.com/google/uuid"
)

type auditSinkStub struct {
	events []plugin.AuditEvent
}

func (s *auditSinkStub) Name() string { return "stub" }

func (s *auditSinkStub) Emit(_ context.Context, event plugin.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestSecurityAuditorEmitInjectionDetectedSemanticOnly(t *testing.T) {
	sink := &auditSinkStub{}
	auditor := NewSecurityAuditor(sink)

	auditor.EmitInjectionDetected(
		context.Background(),
		uuid.New(),
		uuid.New(),
		nil,
		nil,
		&SemanticResult{
			Label:       "JAILBREAK",
			Score:       0.99,
			IsInjection: true,
		},
	)

	if len(sink.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(sink.events))
	}

	detail := sink.events[0].Detail
	if got := detail["detection_count"]; got != 1 {
		t.Fatalf("expected detection_count=1, got %#v", got)
	}
	semantic, ok := detail["semantic"].(map[string]any)
	if !ok {
		t.Fatalf("expected semantic payload, got %#v", detail["semantic"])
	}
	if semantic["label"] != "JAILBREAK" {
		t.Fatalf("expected semantic label JAILBREAK, got %#v", semantic["label"])
	}
}
